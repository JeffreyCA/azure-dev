// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/internal/grpcserver"
	"github.com/azure/azure-dev/cli/azd/internal/tracing"
	"github.com/azure/azure-dev/cli/azd/internal/tracing/fields"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	kv "github.com/azure/azure-dev/cli/azd/pkg/keyvault"
	"github.com/azure/azure-dev/cli/azd/pkg/lazy"
	"github.com/azure/azure-dev/cli/azd/pkg/output/ux"
	pkgux "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// isJsonOutputFromArgs checks if --output json or -o json was passed in args
func isJsonOutputFromArgs(args []string) bool {
	for i, arg := range args {
		if arg == "--output" || arg == "-o" {
			if i+1 < len(args) && args[i+1] == "json" {
				return true
			}
		}
		if arg == "--output=json" || arg == "-o=json" {
			return true
		}
	}
	return false
}

// Cobra command annotation keys used by the extension binding system.
const (
	// extensionIDAnnotation identifies the extension that owns a leaf cobra command.
	extensionIDAnnotation = "extension.id"
	// extensionNamespaceAnnotation records the full extension namespace on a leaf cobra command.
	extensionNamespaceAnnotation = "extension.namespace"
	// extensionNamespaceOwnerAnnotation marks a cobra command as a node that was created
	// (or may be reused) by the extension binding system. It distinguishes extension-owned
	// namespace nodes from built-in azd commands so subsequent extensions can safely merge
	// into the existing tree without clobbering core azd commands.
	extensionNamespaceOwnerAnnotation = "extension.namespace_owner"
)

// bindExtension binds the extension to the root command.
//
// Multiple extensions may share namespace prefixes (for example "ai" and "ai.finetune").
// In that case an existing extension-owned namespace node is reused, so the cobra
// command tree contains a single descriptor per namespace segment. A node may end up
// being both a leaf (with action+annotations from one extension) and a parent (with
// children from other extensions); cobra natively supports this hybrid shape because
// subcommand resolution in cmd.Find runs before action invocation.
//
// If a namespace segment collides with a non-extension cobra command (a built-in
// azd command such as "auth" or "init"), bindExtension returns an error rather than
// attaching the extension to the built-in's tree.
func bindExtension(
	root *actions.ActionDescriptor,
	extension *extensions.Extension,
) error {
	namespaceParts := strings.Split(extension.Namespace, ".")

	current := root

	// Walk the parent segments, finding or creating a namespace node for each.
	for i := 0; i < len(namespaceParts)-1; i++ {
		part := namespaceParts[i]

		next, err := findOrCreateExtensionNamespaceNode(
			current,
			part,
			strings.Join(namespaceParts[:i+1], "."),
			extension.Namespace,
		)
		if err != nil {
			return err
		}
		current = next
	}

	lastPart := namespaceParts[len(namespaceParts)-1]

	// Attach the extension as a leaf at the final segment. Reuse an existing
	// extension-owned namespace node if present so the node becomes hybrid
	// (both leaf and parent). Reject reuse of built-in azd commands.
	if existing := findExtensionChild(current, lastPart); existing != nil {
		if !isExtensionNamespaceNode(existing) {
			return fmt.Errorf(
				"extension namespace '%s' collides with the built-in azd command '%s'",
				extension.Namespace,
				existing.Name,
			)
		}
		upgradeNamespaceNodeToLeaf(existing, extension)
		return nil
	}

	cmd := newExtensionLeafCommand(lastPart, extension)
	current.Add(lastPart, &actions.ActionDescriptorOptions{
		Command:                cmd,
		ActionResolver:         newExtensionAction,
		DisableTroubleshooting: true,
		GroupingOptions: actions.CommandGroupOptions{
			RootLevelHelp: actions.CmdGroupExtensions,
		},
	})

	return nil
}

// findOrCreateExtensionNamespaceNode returns an existing extension-owned namespace
// child with the given name, or creates a new namespace group node when none exists.
// Returns an error when the existing child is a built-in azd command rather than an
// extension namespace node.
//
// If the existing child is itself an extension leaf, it is becoming hybrid because
// we are about to attach a child under it. ensureNamespaceHeaderOnLeaf rewrites its
// Short/Long to the shared-namespace header so the node no longer presents itself
// as one extension's command tree.
func findOrCreateExtensionNamespaceNode(
	parent *actions.ActionDescriptor,
	name string,
	namespacePath string,
	fullNamespace string,
) (*actions.ActionDescriptor, error) {
	if existing := findExtensionChild(parent, name); existing != nil {
		if !isExtensionNamespaceNode(existing) {
			// Use the existing descriptor's name (whatever case the built-in
			// was registered with) so the error doesn't echo the user-typed
			// namespace as if it were the built-in's name.
			return nil, fmt.Errorf(
				"extension namespace '%s' collides with the built-in azd command '%s'",
				fullNamespace,
				existing.Name,
			)
		}
		ensureNamespaceHeaderOnLeaf(existing, namespacePath)
		return existing, nil
	}

	cmd := &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Commands for the %s extension namespace.", namespacePath),
		Annotations: map[string]string{
			extensionNamespaceOwnerAnnotation: "true",
		},
	}

	return parent.Add(name, &actions.ActionDescriptorOptions{
		Command: cmd,
		GroupingOptions: actions.CommandGroupOptions{
			RootLevelHelp: actions.CmdGroupExtensions,
		},
	}), nil
}

// findExtensionChild returns the child action descriptor whose name matches the
// given segment (case-insensitive), or nil. The case-insensitive match is what
// catches built-in command collisions when an extension namespace uses a
// different case (e.g. "Init" or "AUTH").
func findExtensionChild(parent *actions.ActionDescriptor, name string) *actions.ActionDescriptor {
	for _, child := range parent.Children() {
		if strings.EqualFold(child.Name, name) {
			return child
		}
	}
	return nil
}

// isExtensionNamespaceNode reports whether the descriptor represents a namespace
// node created by the extension binding system (and is therefore safe to reuse).
func isExtensionNamespaceNode(descriptor *actions.ActionDescriptor) bool {
	if descriptor.Options == nil || descriptor.Options.Command == nil {
		return false
	}
	return descriptor.Options.Command.Annotations[extensionNamespaceOwnerAnnotation] == "true"
}

// newExtensionLeafCommand constructs the cobra command for an extension leaf, including
// the annotations CobraBuilder relies on to dispatch the extension action and the
// ownership marker that allows other extensions to nest under the same namespace later.
func newExtensionLeafCommand(name string, extension *extensions.Extension) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              extension.Description,
		Long:               extension.Description,
		DisableFlagParsing: true,
		Annotations: map[string]string{
			extensionIDAnnotation:             extension.Id,
			extensionNamespaceAnnotation:      extension.Namespace,
			extensionNamespaceOwnerAnnotation: "true",
		},
	}
}

// upgradeNamespaceNodeToLeaf attaches a leaf extension's action and annotations
// to an existing extension-owned namespace descriptor, preserving its children.
//
// This only runs when a single-segment extension is being attached to a namespace
// node already created by another extension's parent walk, so the node is always
// becoming hybrid (parent + leaf). Hybrid nodes use the shared-namespace header
// for Short and Long; the leaf extension's description is reachable from the
// binary's internal subcommand help.
func upgradeNamespaceNodeToLeaf(descriptor *actions.ActionDescriptor, extension *extensions.Extension) {
	cmd := descriptor.Options.Command
	header := cmd.Short
	cmd.Long = header
	cmd.DisableFlagParsing = true
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[extensionIDAnnotation] = extension.Id
	cmd.Annotations[extensionNamespaceAnnotation] = extension.Namespace
	cmd.Annotations[extensionNamespaceOwnerAnnotation] = "true"

	descriptor.Options.ActionResolver = newExtensionAction
	descriptor.Options.DisableTroubleshooting = true
}

// ensureNamespaceHeaderOnLeaf rewrites the existing extension leaf's Short and
// Long to the shared-namespace header when the leaf is about to become hybrid
// (i.e., we are attaching a child under it). No-op for pure namespace nodes,
// which already carry the header as their Short.
func ensureNamespaceHeaderOnLeaf(descriptor *actions.ActionDescriptor, namespacePath string) {
	if descriptor.Options.ActionResolver == nil {
		return
	}
	cmd := descriptor.Options.Command
	header := fmt.Sprintf("Commands for the %s extension namespace.", namespacePath)
	cmd.Short = header
	cmd.Long = header
}

// extensionSiblingChildren returns the cobra children of cmd that were contributed
// by other extensions (i.e. are themselves extension-owned namespace nodes or leaves).
// It is used to surface sibling-extension contributions in help output for hybrid
// leaf commands.
func extensionSiblingChildren(cmd *cobra.Command) []*cobra.Command {
	if cmd == nil {
		return nil
	}
	var siblings []*cobra.Command
	for _, child := range cmd.Commands() {
		if child.Annotations == nil {
			continue
		}
		if child.Annotations[extensionNamespaceOwnerAnnotation] != "true" {
			continue
		}
		siblings = append(siblings, child)
	}
	return siblings
}

// isHelpInvocation reports whether the args slice represents a help-style invocation
// of the parent command itself: no args at all, or only help flags with no positional
// args. A positional arg means the user is targeting a subcommand and the request
// must be forwarded to the extension binary so it can render that subcommand's help.
//
// Global flags (--debug, --cwd, etc.) are stripped before args reach the extension
// action, so this scan only ever sees extension-bound tokens.
func isHelpInvocation(args []string) bool {
	if len(args) == 0 {
		return true
	}
	sawHelp := false
	for _, a := range args {
		switch a {
		case "--help", "-h", "-help":
			sawHelp = true
		default:
			// Any non-help token (positional or other flag) means the user
			// is targeting something deeper than the parent command itself.
			return false
		}
	}
	return sawHelp
}

// injectMetadataChildren attaches synthetic, hidden-from-execution cobra child
// commands derived from the leaf extension's metadata. This is used during
// help rendering for hybrid leaves so that cobra's standard Available Commands
// block lists both the leaf's own subcommands (sourced from metadata) and the
// sibling extension contributions (already registered as cobra children).
//
// The synthetic commands are returned so callers can remove them after help is
// rendered, leaving the cobra tree in its original state.
func injectMetadataChildren(
	cmd *cobra.Command,
	meta *extensions.ExtensionCommandMetadata,
) []*cobra.Command {
	if cmd == nil || meta == nil {
		return nil
	}

	existing := map[string]bool{}
	for _, c := range cmd.Commands() {
		existing[c.Name()] = true
	}

	var added []*cobra.Command
	for _, c := range meta.Commands {
		if c.Hidden || len(c.Name) == 0 {
			continue
		}
		name := c.Name[len(c.Name)-1]
		if existing[name] {
			continue
		}
		synth := &cobra.Command{
			Use:   name,
			Short: c.Short,
			// Cobra's default help template skips commands where IsAvailableCommand()
			// is false, which requires either Runnable or available subcommands. The
			// no-op Run satisfies that; it's never invoked because the synthetic
			// children are removed via RemoveCommand right after cmd.Help() returns.
			Run: func(*cobra.Command, []string) {},
		}
		cmd.AddCommand(synth)
		added = append(added, synth)
	}
	return added
}

// formatSiblingExtensionsFallbackNotice returns a short notice listing sibling
// extension subcommands. It is used only when the leaf's metadata is not
// available, so we cannot render a merged help and instead must invoke the
// binary; the notice ensures siblings remain at least discoverable.
func formatSiblingExtensionsFallbackNotice(parentPath string, siblings []*cobra.Command) string {
	if len(siblings) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Note: this namespace is shared with other extensions. ")
	b.WriteString("Sibling subcommands available under '")
	b.WriteString(parentPath)
	b.WriteString("':\n")
	for _, s := range siblings {
		extId := s.Annotations[extensionIDAnnotation]
		short := s.Short
		if short == "" {
			short = "(no description)"
		}
		if extId != "" {
			fmt.Fprintf(&b, "  %s — %s (via extension %s)\n", s.Name(), short, extId)
		} else {
			fmt.Fprintf(&b, "  %s — %s\n", s.Name(), short)
		}
	}
	return b.String()
}

type extensionAction struct {
	console          input.Console
	extensionRunner  *extensions.Runner
	lazyEnv          *lazy.Lazy[*environment.Environment]
	extensionManager *extensions.Manager
	azdServer        *grpcserver.Server
	globalOptions    *internal.GlobalCommandOptions
	kvService        kv.KeyVaultService
	cmd              *cobra.Command
	args             []string
}

func newExtensionAction(
	console input.Console,
	extensionRunner *extensions.Runner,
	commandRunner exec.CommandRunner,
	lazyEnv *lazy.Lazy[*environment.Environment],
	extensionManager *extensions.Manager,
	cmd *cobra.Command,
	azdServer *grpcserver.Server,
	globalOptions *internal.GlobalCommandOptions,
	kvService kv.KeyVaultService,
	args []string,
) actions.Action {
	return &extensionAction{
		console:          console,
		extensionRunner:  extensionRunner,
		lazyEnv:          lazyEnv,
		extensionManager: extensionManager,
		azdServer:        azdServer,
		globalOptions:    globalOptions,
		kvService:        kvService,
		cmd:              cmd,
		args:             args,
	}
}

func (a *extensionAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	extensionId, has := a.cmd.Annotations[extensionIDAnnotation]
	if !has {
		return nil, internal.ErrExtensionNotFound
	}

	extension, err := a.extensionManager.GetInstalled(extensions.FilterOptions{
		Id: extensionId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get extension %s: %w", extensionId, err)
	}

	// When the cobra command is a hybrid leaf (an extension leaf that is also
	// a parent of sibling-extension subcommands) and the user requested help,
	// render azd's own help instead of shelling out to the binary so the
	// Available Commands block lists both the leaf's own subcommands and the
	// sibling contributions; the binary alone would only list its own.
	if a.cmd.HasSubCommands() && isHelpInvocation(a.args) {
		if meta, mErr := a.extensionManager.LoadMetadata(extensionId); mErr == nil && meta != nil {
			added := injectMetadataChildren(a.cmd, meta)
			defer a.cmd.RemoveCommand(added...)
			// generateCmdHelp is captured at command registration time, so
			// the Available Commands section reflects whatever children were
			// attached then. Re-render with the augmented children before
			// invoking cmd.Help().
			a.cmd.SetHelpTemplate(generateCmdHelp(a.cmd, generateCmdHelpOptions{}))
			return nil, a.cmd.Help()
		}
		// Metadata unavailable — fall through to invoking the binary so it
		// can render its own help. Surface a brief notice first so siblings
		// are at least discoverable.
		if siblings := extensionSiblingChildren(a.cmd); len(siblings) > 0 {
			a.console.Message(ctx, formatSiblingExtensionsFallbackNotice(a.cmd.CommandPath(), siblings))
		}
	}
	// Start update check in background while extension runs
	// By the time extension finishes, we'll have the result ready
	showUpdateWarning := !isJsonOutputFromArgs(os.Args)
	if showUpdateWarning {
		updateResultChan := make(chan *updateCheckOutcome, 1)
		// Create a minimal copy with only the fields needed for update checking.
		// Cannot copy the full Extension due to sync.Once (contains sync.noCopy).
		// The goroutine will re-fetch the full extension from config when saving.
		extForCheck := &extensions.Extension{
			Id:                extension.Id,
			DisplayName:       extension.DisplayName,
			Version:           extension.Version,
			Source:            extension.Source,
			LastUpdateWarning: extension.LastUpdateWarning,
		}
		go a.checkForUpdateAsync(ctx, extForCheck, updateResultChan)
		// Note: This defer runs AFTER the defer for a.azdServer.Stop() registered later,
		// because defers execute in LIFO order. This is intentional - we want to show
		// the warning after the extension completes but the server stop doesn't affect us.
		defer func() {
			// Collect result and show warning if needed (non-blocking read)
			select {
			case result := <-updateResultChan:
				if result != nil && result.shouldShow && result.warning != nil {
					a.console.MessageUxItem(ctx, result.warning)
					a.console.Message(ctx, "")

					// Record cooldown only after warning is actually displayed
					a.recordUpdateWarningShown(result.extensionId, result.extensionSource)
				}
			default:
				// Check didn't complete in time, skip warning (and don't record cooldown)
			}
		}()
	}

	tracing.SetUsageAttributes(
		fields.ExtensionId.String(extension.Id),
		fields.ExtensionVersion.String(extension.Version))

	allEnv := []string{}
	allEnv = append(allEnv, os.Environ()...)

	forceColor := !color.NoColor
	if forceColor {
		allEnv = append(allEnv, "FORCE_COLOR=1")
	}

	// Pass the console width down to the child process
	// COLUMNS is a semi-standard environment variable used by many Unix programs to determine the width of the terminal.
	width := pkgux.ConsoleWidth()
	if width > 0 {
		allEnv = append(allEnv, fmt.Sprintf("COLUMNS=%d", width))
	}

	env, err := a.lazyEnv.GetValue()
	if err == nil && env != nil {
		// Resolve Key Vault secret references only in azd-managed environment
		// variables (akvs:// and @Microsoft.KeyVault formats). System env vars
		// from os.Environ() are NOT processed — only the azd environment's
		// variables may contain KV references.
		azdEnvVars := env.Environ()
		subId := env.Getenv("AZURE_SUBSCRIPTION_ID")
		azdEnvVars, kvErr := kv.ResolveSecretEnvironment(ctx, a.kvService, azdEnvVars, subId)
		if kvErr != nil {
			log.Printf("warning: %v", kvErr)
		}

		allEnv = append(allEnv, azdEnvVars...)
	}

	serverInfo, err := a.azdServer.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start gRPC server: %w", err)
	}

	defer a.azdServer.Stop()

	jwtToken, err := grpcserver.GenerateExtensionToken(extension, serverInfo)
	if err != nil {
		return nil, fmt.Errorf(
			"generating extension token: %w",
			internal.ErrExtensionTokenFailed,
		)
	}

	allEnv = append(allEnv,
		fmt.Sprintf("AZD_SERVER=%s", serverInfo.Address),
		fmt.Sprintf("AZD_ACCESS_TOKEN=%s", jwtToken),
	)

	// Propagate trace context to the extension process
	if traceEnv := tracing.Environ(ctx); len(traceEnv) > 0 {
		allEnv = append(allEnv, traceEnv...)
	}

	// Use globalOptions for flag propagation instead of cmd.Flags().
	// Extension commands use DisableFlagParsing=true, so cobra never parses
	// global flags like --debug, --cwd, or -e. The globalOptions were populated
	// by ParseGlobalFlags() before command tree construction and are the only
	// reliable source for these values.
	options := &extensions.InvokeOptions{
		Args: a.args,
		Env:  allEnv,
		// cmd extensions are always interactive (connected to terminal)
		Interactive: true,
		Debug:       a.globalOptions.EnableDebugLogging,
		// Use globalOptions.NoPrompt which includes agent detection,
		// not just the --no-prompt CLI flag
		NoPrompt:    a.globalOptions.NoPrompt,
		Cwd:         a.globalOptions.Cwd,
		Environment: a.globalOptions.EnvironmentName,
	}

	_, invokeErr := a.extensionRunner.Invoke(ctx, extension, options)

	// Update warning is shown via defer above (runs after invoke completes)

	if invokeErr != nil {
		// Check if the extension reported a structured error via gRPC.
		// This gives us a typed LocalError/ServiceError for telemetry classification
		// instead of just a generic exit-code error.
		if reportedErr := extension.GetReportedError(); reportedErr != nil {
			// Wrap both errors so the chain contains both:
			// - reportedErr (LocalError/ServiceError) for telemetry classification
			// - invokeErr (ExtensionRunError) for UX middleware handling
			return nil, fmt.Errorf("%w: %w", reportedErr, invokeErr)
		}

		return nil, invokeErr
	}

	return nil, nil
}

// updateCheckOutcome holds the result of an async update check
type updateCheckOutcome struct {
	shouldShow      bool
	warning         *ux.WarningMessage
	extensionId     string // Used to record cooldown only when warning is actually displayed
	extensionSource string // Source of the extension for precise lookup
}

// checkForUpdateAsync performs the update check in a goroutine and sends the result to the channel.
// This runs in parallel with the extension execution, so by the time the extension finishes,
// we have the result ready with zero added latency.
func (a *extensionAction) checkForUpdateAsync(
	ctx context.Context,
	extension *extensions.Extension,
	resultChan chan<- *updateCheckOutcome,
) {
	defer close(resultChan)

	outcome := &updateCheckOutcome{shouldShow: false}

	// Create cache manager
	cacheManager, err := extensions.NewRegistryCacheManager()
	if err != nil {
		log.Printf("failed to create cache manager: %v", err)
		resultChan <- outcome
		return
	}

	// Check if cache needs refresh - if so, refresh it now (we have time while extension runs)
	if cacheManager.IsExpiredOrMissing(ctx, extension.Source) {
		a.refreshCacheForSource(ctx, cacheManager, extension.Source)
	}

	// Create update checker
	updateChecker := extensions.NewUpdateChecker(cacheManager)

	// Check if we should show a warning (respecting cooldown)
	// Uses extension's LastUpdateWarning field
	if !updateChecker.ShouldShowWarning(extension) {
		resultChan <- outcome
		return
	}

	// Check for updates
	result, err := updateChecker.CheckForUpdate(ctx, extension)
	if err != nil {
		log.Printf("failed to check for extension update: %v", err)
		resultChan <- outcome
		return
	}

	if result.HasUpdate {
		outcome.shouldShow = true
		outcome.warning = extensions.FormatUpdateWarning(result)
		outcome.extensionId = extension.Id
		outcome.extensionSource = extension.Source
		// Note: Cooldown is recorded by caller only when warning is actually displayed
	}

	resultChan <- outcome
}

// recordUpdateWarningShown saves the cooldown timestamp after a warning is displayed
func (a *extensionAction) recordUpdateWarningShown(extensionId, extensionSource string) {
	// Re-fetch the full extension from config to avoid overwriting fields
	fullExtension, err := a.extensionManager.GetInstalled(extensions.FilterOptions{
		Id:     extensionId,
		Source: extensionSource,
	})
	if err != nil {
		log.Printf("failed to get extension for saving warning timestamp: %v", err)
		return
	}

	// Record the warning timestamp
	extensions.RecordUpdateWarningShown(fullExtension)

	// Save the updated extension to config
	if err := a.extensionManager.UpdateInstalled(fullExtension); err != nil {
		log.Printf("failed to save warning timestamp: %v", err)
	}
}

// refreshCacheForSource attempts to refresh the cache for a specific source
func (a *extensionAction) refreshCacheForSource(
	ctx context.Context,
	cacheManager *extensions.RegistryCacheManager,
	sourceName string,
) {
	// Find extensions from this source to get registry data
	sourceExtensions, err := a.extensionManager.FindExtensions(ctx, &extensions.FilterOptions{
		Source: sourceName,
	})
	if err != nil {
		log.Printf("failed to fetch extensions from source %s: %v", sourceName, err)
		return
	}

	// Cache the extensions
	if err := cacheManager.Set(ctx, sourceName, sourceExtensions); err != nil {
		log.Printf("failed to cache extensions for source %s: %v", sourceName, err)
	}
}
