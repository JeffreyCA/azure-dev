// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:generate goversioninfo -arm -64

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	azcorelog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"
	"github.com/azure/azure-dev/cli/azd/cmd"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/internal/telemetry"
	"github.com/azure/azure-dev/cli/azd/internal/tracing"
	"github.com/azure/azure-dev/cli/azd/pkg/config"
	"github.com/azure/azure-dev/cli/azd/pkg/installer"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/azure/azure-dev/cli/azd/pkg/oneauth"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/azure/azure-dev/cli/azd/pkg/update"
	"github.com/mattn/go-colorable"
	"github.com/spf13/pflag"
)

func main() {
	ctx := context.Background()

	restoreColorMode := colorable.EnableColorsStdout(nil)
	defer restoreColorMode()

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	debugEnabled := isDebugEnabled()
	logFileCleanup := setupLogging(debugEnabled)
	defer logFileCleanup()

	if debugEnabled {
		azcorelog.SetListener(func(event azcorelog.Event, msg string) {
			log.Printf("%s: %s\n", event, msg)
		})
	}

	log.Printf("azd version: %s", internal.Version)

	ts := telemetry.GetTelemetrySystem()
	if ts != nil {
		ctx = tracing.ContextFromEnv(ctx)
	}

	showedElevationWarning := false

	latest := make(chan *update.VersionInfo)
	bgCtx, bgCancel := context.WithTimeout(context.Background(), 60*time.Second)
	go fetchLatestVersion(bgCtx, latest)

	rootContainer := ioc.NewNestedContainer(nil)

	ctx = context.WithoutCancel(ctx)
	ctx = tools.WithInstalledCheckCache(ctx)

	// Register the context for singleton resolution
	ioc.RegisterInstance(rootContainer, ctx)

	// Execute command with auto-installation support for extensions
	cmdErr := cmd.ExecuteWithAutoInstall(ctx, rootContainer)

	oneauth.Shutdown()

	if !isJsonOutput() {
		if firstNotice := telemetry.FirstNotice(); firstNotice != "" {
			fmt.Fprintln(os.Stderr, output.WithWarningFormat(firstNotice))
		}
	}

	versionInfo, ok := <-latest
	bgCancel()

	// If we were able to fetch a latest version, check to see if we are up to date and
	// print a warning if we are not. Non-production builds (dev builds via `go install` and
	// PR builds with "-pr." prerelease tags) are excluded since they should not suggest updates.
	//
	// Don't write this message when JSON output is enabled, since in that case we use stderr to return structured
	// information about command progress.
	if !isJsonOutput() && ok && !suppressUpdateBanner() && !showedElevationWarning {
		if internal.IsNonProdVersion() {
			log.Printf("eliding update message for non-production build")
		} else if versionInfo.HasUpdate {
			currentVersionStr := internal.VersionInfo().Version.String()
			latestVersionStr := versionInfo.Version

			// Determine the update hint to show.
			updateHint := update.RunUpdateHint("azd update")
			configMgr := config.NewUserConfigManager(config.NewFileConfigManager(config.NewManager()))
			userCfg, cfgErr := configMgr.Load()
			if cfgErr != nil {
				userCfg = config.NewEmptyConfig()
			}
			if !update.HasUpdateConfig(userCfg) {
				updateHint = platformUpgradeHint()
			}

			banner := update.RenderUpdateBanner(update.BannerParams{
				CurrentVersion: currentVersionStr,
				LatestVersion:  latestVersionStr,
				Channel:        versionInfo.Channel,
				UpdateHint:     updateHint,
			})
			fmt.Fprintln(os.Stderr, banner)
		}
	}

	if ts != nil {
		err := ts.Shutdown(ctx)
		if err != nil {
			log.Printf("non-graceful telemetry shutdown: %v\n", err)
		}

		if ts.EmittedAnyTelemetry() {
			err := startBackgroundUploadProcess()
			if err != nil {
				log.Printf("failed to start background telemetry upload: %v\n", err)
			}
		}
	}

	if cmdErr != nil {
		os.Exit(1)
	}
}

// fetchLatestVersion checks for a newer version of the CLI using the user's
// configured channel and sends the result across the channel, which it then closes.
// If the latest version can not be determined, the channel is closed without writing a value.
func fetchLatestVersion(ctx context.Context, result chan<- *update.VersionInfo) {
	defer close(result)

	// Allow the user to skip the update check if they wish, by setting AZD_SKIP_UPDATE_CHECK to
	// a truthy value.
	if value, has := os.LookupEnv("AZD_SKIP_UPDATE_CHECK"); has {
		if setting, err := strconv.ParseBool(value); err == nil && setting {
			log.Print("skipping update check since AZD_SKIP_UPDATE_CHECK is true")
			return
		} else if err != nil {
			log.Printf("could not parse value for AZD_SKIP_UPDATE_CHECK a boolean "+
				"(it was: %s), proceeding with update check", value)
		}
	}

	// Load user config to determine channel
	configMgr := config.NewUserConfigManager(config.NewFileConfigManager(config.NewManager()))
	userConfig, err := configMgr.Load()
	if err != nil {
		userConfig = config.NewEmptyConfig()
	}

	cfg := update.LoadUpdateConfig(userConfig)

	mgr := update.NewManager(nil, nil)
	versionInfo, err := mgr.CheckForUpdate(ctx, cfg, false)
	if err != nil {
		log.Printf("failed to check for updates: %v, skipping update check", err)
		return
	}

	result <- versionInfo
}

// isDebugEnabled checks to see if `--debug` was passed with a truthy
// value.
func isDebugEnabled() bool {
	debug := false
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)

	// Since we are running this parse logic on the full command line, there may be additional flags
	// which we have not defined in our flag set (but would be defined by whatever command we end up
	// running). Setting UnknownFlags instructs `flags.Parse` to continue parsing the command line
	// even if a flag is not in the flag set (instead of just returning an error saying the flag was not
	// found).
	flags.ParseErrorsAllowlist.UnknownFlags = true
	flags.BoolVar(&debug, "debug", false, "")

	// if flag `-h` of `--help` is within the command, the usage is automatically shown.
	// Setting `Usage` to a no-op will hide this extra unwanted output.
	flags.Usage = func() {}

	_ = flags.Parse(os.Args[1:])
	return debug
}

// isJsonOutput checks to see if `--output` was passed with the value `json`
// suppressUpdateBanner returns true for commands where the "out of date" banner
// adds no value: azd update (stale version in-process), azd config (managing settings).
func suppressUpdateBanner() bool {
	if len(os.Args) < 2 {
		return false
	}
	return os.Args[1] == "update" || os.Args[1] == "config"
}

func isJsonOutput() bool {
	output := ""
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)

	// Since we are running this parse logic on the full command line, there may be additional flags
	// which we have not defined in our flag set (but would be defined by whatever command we end up
	// running). Setting UnknownFlags instructs `flags.Parse` to continue parsing the command line
	// even if a flag is not in the flag set (instead of just returning an error saying the flag was not
	// found).
	flags.ParseErrorsAllowlist.UnknownFlags = true
	flags.StringVarP(&output, "output", "o", "", "")

	// if flag `-h` of `--help` is within the command, the usage is automatically shown.
	// Setting `Usage` to a no-op will hide this extra unwanted output.
	flags.Usage = func() {}

	_ = flags.Parse(os.Args[1:])

	return output == "json"
}

// setupLogging configures log output based on AZD_DEBUG_LOG environment variable
// Returns a cleanup function that should be called when the program exits
func setupLogging(debugEnabled bool) func() {
	debugLogValue := os.Getenv("AZD_DEBUG_LOG")

	var logOutput io.Writer = io.Discard
	var cleanupFunc func() = func() {}

	// Check if debug logging is enabled and valid
	if debugLogValue != "" {
		if isDebugLogEnabled, err := strconv.ParseBool(debugLogValue); err == nil && isDebugLogEnabled {
			// Create daily log files adjacent to azd binary
			if logFile, err := createDailyLogFile(); err == nil {
				if debugEnabled {
					// When --debug is used, write to both stderr and log file
					logOutput = io.MultiWriter(os.Stderr, logFile)
				} else {
					// When only AZD_DEBUG_LOG is set, write only to log file
					logOutput = logFile
				}

				// Set cleanup function to close the log file
				cleanupFunc = func() {
					logFile.Close()
				}
			}
		}
	}

	// If debug is enabled but no log file was created, use stderr
	if debugEnabled && logOutput == io.Discard {
		logOutput = os.Stderr
	}

	log.SetOutput(logOutput)
	return cleanupFunc
}

// createDailyLogFile creates a daily log file adjacent to the azd binary
func createDailyLogFile() (*os.File, error) {
	// Get the path to the current executable
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get current executable path: %w", err)
	}

	// Get the directory containing the executable
	execDir := filepath.Dir(execPath)

	// Create log filename with current date
	currentDate := time.Now().Format("2006-01-02")
	logFileName := fmt.Sprintf("azd-%s.log", currentDate)
	logFilePath := filepath.Join(execDir, logFileName)

	// Open or create the log file (append mode)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create/open log file %s: %w", logFilePath, err)
	}

	return logFile, nil
}

// platformUpgradeHint returns the platform-specific update action for azd.
func platformUpgradeHint() update.UpdateHint {
	installedBy := installer.InstalledBy()

	if runtime.GOOS == "windows" {
		switch installedBy {
		case installer.InstallTypePs:
			//nolint:lll
			return update.RunUpdateHint("powershell -ex AllSigned -c \"Invoke-RestMethod 'https://aka.ms/install-azd.ps1' | Invoke-Expression\"")
		case installer.InstallTypeWinget:
			return update.RunUpdateHint("winget upgrade Microsoft.Azd")
		case installer.InstallTypeChoco:
			return update.RunUpdateHint("choco upgrade azd")
		default:
			return update.VisitUpdateHint("https://aka.ms/azd/upgrade/windows")
		}
	} else if runtime.GOOS == "linux" {
		switch installedBy {
		case installer.InstallTypeSh:
			return update.RunUpdateHint("curl -fsSL https://aka.ms/install-azd.sh | bash")
		default:
			return update.VisitUpdateHint("https://aka.ms/azd/upgrade/linux")
		}
	} else if runtime.GOOS == "darwin" {
		switch installedBy {
		case installer.InstallTypeBrew:
			return update.RunUpdateHint("brew uninstall azd && brew install azure/azd/azd")
		case installer.InstallTypeSh:
			return update.RunUpdateHint("curl -fsSL https://aka.ms/install-azd.sh | bash")
		default:
			return update.VisitUpdateHint("https://aka.ms/azd/upgrade/mac")
		}
	}

	return update.VisitUpdateHint("https://aka.ms/azd/upgrade")
}

func startBackgroundUploadProcess() error {
	// The background upload process executable is ourself
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}

	// #nosec G204 - this is not a security issue, we are executing our own binary
	cmd := exec.Command(execPath, cmd.TelemetryCommandFlag, cmd.TelemetryUploadCommandFlag)

	// Use the location of azd as the cwd for the background uploading process.  On windows, when a process is running
	// the current working directory is considered in use and can not be deleted. If a user runs `azd` in a directory, we
	// do want that directory to be considered in use and locked while the telemetry upload is happening. One example of
	// where we see this problem often is in our CI for end to end tests where we run a copy of `azd` that we built in an
	// ephemeral directory created by (*testing.T).TempDir().  When the test completes, the testing package attempts to
	// clean up the temporary directory, but if the telemetry upload process is still running, the directory can not be
	// deleted.
	cmd.Dir = filepath.Dir(execPath)

	err = cmd.Start()
	return err
}
