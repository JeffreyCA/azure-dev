// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/azure/azure-dev/cli/azd/extensions/microsoft.azd.extensions/internal"
	"github.com/azure/azure-dev/cli/azd/extensions/microsoft.azd.extensions/internal/models"
	"github.com/azure/azure-dev/cli/azd/pkg/common"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/spf13/cobra"
)

type packageFlags struct {
	inputPath  string
	outputPath string
	rebuild    bool
}

func newPackCommand() *cobra.Command {
	flags := &packageFlags{}

	packageCmd := &cobra.Command{
		Use:   "pack",
		Short: "Build and pack extension artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			internal.WriteCommandHeader(
				"Package azd extension (azd x pack)",
				"Packages the azd extension project and updates the registry",
			)

			defaultPackageFlags(flags)
			err := runPackageAction(cmd.Context(), flags)
			if err != nil {
				return err
			}

			internal.WriteCommandSuccess("Extension packaged successfully")
			return nil
		},
	}

	packageCmd.Flags().StringVarP(
		&flags.outputPath,
		"output", "o", "",
		"Path to the artifacts output directory. If not provided, will use local registry artifacts path.",
	)

	packageCmd.Flags().StringVarP(
		&flags.inputPath,
		"input", "i", "./bin",
		"Path to the input directory.",
	)

	packageCmd.Flags().BoolVar(
		&flags.rebuild,
		"rebuild", false,
		"Rebuild the extension before packaging.",
	)

	return packageCmd
}

func runPackageAction(ctx context.Context, flags *packageFlags) error {
	absExtensionPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get absolute path for extension directory: %w", err)
	}

	extensionMetadata, err := models.LoadExtension(absExtensionPath)
	if err != nil {
		return fmt.Errorf("failed to load extension metadata: %w", err)
	}

	if flags.outputPath == "" {
		localRegistryArtifactsPath, err := internal.LocalRegistryArtifactsPath()
		if err != nil {
			return err
		}

		flags.outputPath = filepath.Join(localRegistryArtifactsPath, extensionMetadata.Id, extensionMetadata.Version)
	}

	absInputPath := filepath.Join(extensionMetadata.Path, flags.inputPath)
	absOutputPath, err := filepath.Abs(flags.outputPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for output directory: %w", err)
	}

	fmt.Println()
	fmt.Printf("%s: %s\n", output.WithBold("Input Path"), output.WithHyperlink(absInputPath, absInputPath))
	fmt.Printf("%s: %s\n", output.WithBold("Output Path"), output.WithHyperlink(absOutputPath, absOutputPath))

	taskList := ux.NewTaskList(nil).
		AddTask(ux.TaskOptions{
			Title: "Building extension",
			Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
				// Verify if we have any existing binaries
				if !flags.rebuild {
					entires, err := os.ReadDir(absInputPath)
					if err == nil {
						binaries := []string{}

						for _, entry := range entires {
							if entry.IsDir() {
								continue
							}

							// Only process files that match the extension ID
							artifactName := entry.Name()
							if !strings.HasPrefix(artifactName, extensionMetadata.SafeDashId()) {
								continue
							}

							ext := filepath.Ext(artifactName)
							if ext != ".exe" && ext != "" {
								continue
							}

							binaries = append(binaries, entry.Name())
						}

						if len(binaries) > 0 {
							return ux.Skipped, nil
						}
					}
				}

				buildCmd := exec.Command("azd", "x", "build", "--all")
				buildCmd.Dir = extensionMetadata.Path

				resultBytes, err := buildCmd.CombinedOutput()
				if err != nil {
					return ux.Error, common.NewDetailedError(
						"Build failed",
						fmt.Errorf("failed to run command: %w, Command output: %s", err, string(resultBytes)),
					)
				}

				return ux.Success, nil
			},
		}).
		AddTask(ux.TaskOptions{
			Title: "Generating command spec",
			Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
				commandsJsonPath := filepath.Join(extensionMetadata.Path, "commands.json")

				// Find a binary for the current platform to invoke
				binaryPath, err := findCurrentPlatformBinary(extensionMetadata, absInputPath)
				if err != nil {
					// No binary for current platform, skip
					return ux.Skipped, nil
				}

				// Invoke binary with __commands
				// Set NO_COLOR to ensure clean output without ANSI escape codes
				cmd := exec.CommandContext(ctx, binaryPath, "__commands") //nolint:gosec
				cmd.Env = append(os.Environ(), "NO_COLOR=1")
				output, err := cmd.Output()
				if err != nil {
					// Binary might not support __commands (non-Go or old extension)
					return ux.Skipped, nil
				}

				// Validate JSON
				var spec map[string]any
				if err := json.Unmarshal(output, &spec); err != nil {
					return ux.Skipped, nil
				}

				// Write commands.json (always overwrite to ensure it's up to date)
				if err := os.WriteFile(commandsJsonPath, output, osutil.PermissionFile); err != nil {
					return ux.Error, common.NewDetailedError(
						"Failed to write commands.json",
						fmt.Errorf("failed to write command spec: %w", err),
					)
				}

				return ux.Success, nil
			},
		}).
		AddTask(ux.TaskOptions{
			Title: "Packaging extension",
			Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
				if err := packExtensionBinaries(extensionMetadata, flags.outputPath); err != nil {
					return ux.Error, common.NewDetailedError(
						"Packaging failed",
						fmt.Errorf("failed to package extension: %w", err),
					)
				}

				return ux.Success, nil
			},
		})

	return taskList.Run()
}

func packExtensionBinaries(
	extensionMetadata *models.ExtensionSchema,
	outputPath string,
) error {
	// Prepare artifacts for registry
	buildPath := filepath.Join(extensionMetadata.Path, "bin")
	entries, err := os.ReadDir(buildPath)
	if err != nil {
		return fmt.Errorf("failed to read artifacts directory: %w", err)
	}

	extensionYamlSourcePath := filepath.Join(extensionMetadata.Path, "extension.yaml")
	commandsJsonSourcePath := filepath.Join(extensionMetadata.Path, "commands.json")

	// Ensure target directory exists
	if err := os.MkdirAll(outputPath, osutil.PermissionDirectory); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Check if commands.json exists
	hasCommandsJson := false
	if _, err := os.Stat(commandsJsonSourcePath); err == nil {
		hasCommandsJson = true
	}

	// Map and copy artifacts
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process files that match the extension ID
		artifactName := entry.Name()
		if !strings.HasPrefix(artifactName, extensionMetadata.SafeDashId()) {
			continue
		}

		ext := filepath.Ext(artifactName)
		if ext != ".exe" && ext != "" {
			continue
		}

		fileWithoutExt := internal.GetFileNameWithoutExt(artifactName)
		artifactSourcePath := filepath.Join(buildPath, entry.Name())
		sourceFiles := []string{extensionYamlSourcePath, artifactSourcePath}

		// Include commands.json if it exists
		if hasCommandsJson {
			sourceFiles = append(sourceFiles, commandsJsonSourcePath)
		}

		_, err := createArchive(artifactName, fileWithoutExt, outputPath, sourceFiles)
		if err != nil {
			return fmt.Errorf("failed to create archive for %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// findCurrentPlatformBinary finds an extension binary for the current OS/arch
func findCurrentPlatformBinary(extensionMetadata *models.ExtensionSchema, binPath string) (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Look for matching binary with pattern: <id>-<os>-<arch>[.exe]
	pattern := fmt.Sprintf("%s-%s-%s", extensionMetadata.SafeDashId(), goos, goarch)
	entries, err := os.ReadDir(binPath)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check if file starts with the pattern (handles both with and without .exe)
		if strings.HasPrefix(name, pattern) {
			return filepath.Join(binPath, name), nil
		}
	}

	return "", fmt.Errorf("no binary matching %s found in %s", pattern, binPath)
}

func defaultPackageFlags(flags *packageFlags) {
	if flags.inputPath == "" {
		flags.inputPath = "bin"
	}
}

// getArchiveType determines the appropriate archive format based on the artifact name
func getArchiveType(artifactName string) string {
	if strings.Contains(artifactName, "linux") {
		return "tar.gz"
	}
	return "zip"
}

// createArchive creates an archive file using the appropriate format for the given artifact
func createArchive(artifactName, fileWithoutExt, outputPath string, sourceFiles []string) (string, error) {
	archiveType := getArchiveType(artifactName)
	targetFilePath := filepath.Join(outputPath, fmt.Sprintf("%s.%s", fileWithoutExt, archiveType))

	var archiveFunc func([]string, string) error
	switch archiveType {
	case "tar.gz":
		archiveFunc = internal.TarGzSource
	case "zip":
		archiveFunc = internal.ZipSource
	default:
		return "", fmt.Errorf("unsupported archive type: %s", archiveType)
	}

	return targetFilePath, archiveFunc(sourceFiles, targetFilePath)
}
