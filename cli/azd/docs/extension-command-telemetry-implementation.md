# Extension Command Spec (Telemetry + Fig completions): Implementation Plan

This document details the implementation plan for capturing extension command paths in telemetry and enriching the azd Fig completion spec for extension commands, as outlined in [extension-command-telemetry-design.md](./extension-command-telemetry-design.md).

## Overview

We will implement **Approach 4: Bundled Command Spec** with first-class Go support via the `azdext` package.

Extensions will expose a hidden `__commands` subcommand that outputs their command tree as JSON. During `azd x pack`, this command is invoked to generate `commands.json`, which is bundled in the extension archive.

This single `commands.json` artifact is then used for two scenarios:

1. **Telemetry**: azd matches invoked arguments against known commands to safely log command paths.
2. **Fig completions**: azd enriches `azd completion fig` output by injecting the extension's command tree and flags into the generated Fig spec.

### Why this approach works with DisableFlagParsing

Extension commands in azd core are bound as Cobra commands with `DisableFlagParsing: true` (see `cli/azd/cmd/extensions.go`). We do not want to change that.

Fig spec generation (`azd completion fig`) is built by introspecting the Cobra command tree (see `cli/azd/internal/figspec/spec_builder.go`). Because extension commands are bound as thin placeholders (namespace only) and do not register their real subcommands/flags into Cobra, the Fig generator cannot see deeper commands like `azd ai agent init`.

The bundled `commands.json` provides this missing structural metadata without altering runtime parsing behavior.

## Command spec format

### Principles

- The spec is **static** and can be bundled into the extension artifact.
- The spec is **language-agnostic** (Go, .NET, JS, Python extensions can all emit it).
- The spec is **compatible with both telemetry and completions**.
- The spec is **safe to consume** (no need to execute extension code at completion time).

### JSON Schema Design

### Schema Definition

The command tree uses a hierarchical structure mirroring the actual command tree, similar to FigSpec but simplified for telemetry purposes.

**File**: `cli/azd/extensions/commands.schema.json`

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "ExtensionCommandSpec",
  "description": "Schema for extension command tree specification used for telemetry.",
  "type": "object",
  "properties": {
    "extensionId": {
      "type": "string",
      "description": "The unique identifier of the extension."
    },
    "version": {
      "type": "string",
      "description": "The version of the extension."
    },
		"commands": {
      "type": "array",
      "description": "Top-level commands exposed by the extension.",
      "items": {
        "$ref": "#/definitions/Command"
      }
    }
  },
  "required": ["extensionId", "version", "commands"],
  "definitions": {
    "Command": {
      "type": "object",
      "description": "A command or subcommand in the extension.",
      "properties": {
        "name": {
          "type": "string",
          "description": "The command name (e.g., 'context', 'mcp')."
        },
        "description": {
          "type": "string",
          "description": "Brief description of the command."
        },
        "subcommands": {
          "type": "array",
          "description": "Nested subcommands.",
          "items": {
            "$ref": "#/definitions/Command"
          }
        },
				"flags": {
					"type": "array",
					"description": "Flags/options accepted by this command.",
					"items": {
						"$ref": "#/definitions/Flag"
					}
				},
				"args": {
					"type": "array",
					"description": "Positional arguments accepted by this command.",
					"items": {
						"$ref": "#/definitions/Arg"
					}
				}
      },
      "required": ["name"]
    },
    "Flag": {
      "type": "object",
			"description": "A flag/option for a command.",
      "properties": {
        "name": {
          "type": "string",
          "description": "The long flag name without dashes (e.g., 'output')."
        },
        "shorthand": {
          "type": "string",
          "description": "Single character shorthand (e.g., 'o')."
        },
        "description": {
          "type": "string",
          "description": "Brief description of the flag."
				},
				"takesValue": {
					"type": "boolean",
					"description": "Whether the flag expects an argument value.",
					"default": false
				},
				"valueName": {
					"type": "string",
					"description": "Display name for the flag value in completion UIs (e.g., 'path', 'name', 'environment')."
        }
      },
      "required": ["name"]
    },
    "Arg": {
      "type": "object",
			"description": "A positional argument for a command.",
      "properties": {
        "name": {
          "type": "string",
          "description": "The argument name (e.g., 'service', 'environment')."
        },
        "description": {
          "type": "string",
          "description": "Brief description of the argument."
        },
        "optional": {
          "type": "boolean",
          "description": "Whether this argument is optional.",
          "default": false
				},
				"variadic": {
					"type": "boolean",
					"description": "Whether this argument may repeat (variadic).",
					"default": false
        }
      },
      "required": ["name"]
    }
  }
}
```

### Example Output

For the `microsoft.azd.demo` extension:

```json
{
  "extensionId": "microsoft.azd.demo",
  "version": "0.4.0",
  "commands": [
    {
      "name": "context",
      "description": "Displays the current azd project & environment context."
    },
    {
      "name": "prompt",
      "description": "Display prompt capabilities."
    },
    {
      "name": "colors",
      "description": "Display color palette."
    },
    {
      "name": "version",
      "description": "Display extension version."
    },
    {
      "name": "mcp",
      "description": "MCP server commands.",
      "subcommands": [
        {
          "name": "serve",
          "description": "Start MCP server.",
          "flags": [
            {"name": "transport", "shorthand": "t", "description": "Transport type"}
          ]
        }
      ]
    },
    {
      "name": "listen",
      "description": "Starts the extension and listens for events."
    }
  ]
}
```

## How `commands.json` is used for Fig completions

### Goal

Generate a complete Fig spec for extension commands such that when extensions define (for example) `ai.agent` as a namespace and expose subcommands like `init`, the generated spec includes:

- `azd ai agent init`
- the flags/options for `init`
- positional args (best-effort)

### High-level behavior

When generating the Fig spec, azd will:

1. Build the base Fig spec from the Cobra root command as today.
2. Identify extension placeholder commands in the Cobra tree via annotations (e.g., `extension.id`).
3. Load the corresponding `commands.json` from the installed extension.
4. Inject the extension command tree and flags into the Fig spec at that placeholder node.

### Mapping rules (commands.json → Fig spec)

- Each `Command` becomes a Fig `Subcommand` with `name` and `description`.
- Each `Flag` becomes a Fig `Option`.
	- `--<name>` is always included.
	- `-<shorthand>` is included when present.
	- If `takesValue` is true, the option receives a single Fig `args` entry:
		- use `valueName` when present, otherwise default to the flag name.
- Each `Arg` becomes a Fig `Arg`.

> Note: dynamic generators (e.g., suggesting environments/templates) are out of scope for the initial integration. They can be layered later either via extension-provided generator metadata or by azd core heuristics.

## Namespace merging and collision behavior

This section clarifies how multiple extensions can co-exist in the final Fig spec.

### 1) Shared namespace *prefixes* (supported)

Multiple extensions may share a namespace prefix as long as their full namespaces are different.

This is a common and important scenario.

Example (extension IDs shown for clarity; namespaces are what azd binds into the command tree):

- Extension A: `id: azure.ai.agents`, `namespace: ai.agents`  → supports commands like `azd ai agents init`
- Extension B: `id: azure.ai.finetuning`, `namespace: ai.finetuning` → supports commands like `azd ai finetuning start`

In this case, `azd ai` is a shared grouping node in the Cobra tree (already supported by `bindExtension`). Fig spec generation will show both `agents` and `finetuning` under `ai`, each enriched from their own `commands.json`.

This is the recommended and simplest coexistence model.

### 2) Shared exact namespace (not supported)

If two extensions declare the same exact namespace (e.g., both declare `namespace: ai`), azd does not have an unambiguous way to decide which extension should be invoked for `azd ai <...>`.

To keep runtime behavior and generated completions consistent and predictable, we do **not** support exact-namespace duplication. Installation (or enablement) should fail with a clear error message directing the user to uninstall one of the extensions or choose extensions with distinct namespaces.

## Implementation Tasks

### Task 1: Add `azdext.NewCommandTreeCommand()` Function

**File**: `cli/azd/pkg/azdext/command_tree.go` (new file)

This function creates a hidden Cobra command that extension authors add to their root command. When invoked, it traverses the command tree and outputs JSON.

```go
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// CommandSpec represents the full command tree specification for an extension.
type CommandSpec struct {
	ExtensionID string    `json:"extensionId"`
	Version     string    `json:"version"`
	Commands    []Command `json:"commands"`
}

// Command represents a command or subcommand in the extension.
type Command struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Subcommands []Command `json:"subcommands,omitempty"`
	Flags       []Flag    `json:"flags,omitempty"`
	Args        []Arg     `json:"args,omitempty"`
}

// Flag represents a flag/option for a command.
type Flag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description,omitempty"`
}

// Arg represents a positional argument for a command.
type Arg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

// NewCommandTreeCommand creates a hidden command that outputs the command tree as JSON.
// Extension authors should add this to their root command:
//
//	rootCmd.AddCommand(azdext.NewCommandTreeCommand(rootCmd, extensionID, version))
func NewCommandTreeCommand(root *cobra.Command, extensionID, version string) *cobra.Command {
	return &cobra.Command{
		Use:    "__commands",
		Hidden: true,
		Short:  "Output command tree as JSON (internal use)",
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := BuildCommandSpec(root, extensionID, version)
			
			output, err := json.MarshalIndent(spec, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal command spec: %w", err)
			}
			
			fmt.Fprintln(cmd.OutOrStdout(), string(output))
			return nil
		},
	}
}

// BuildCommandSpec builds a CommandSpec from a Cobra command tree.
func BuildCommandSpec(root *cobra.Command, extensionID, version string) *CommandSpec {
	commands := buildCommands(root)
	
	return &CommandSpec{
		ExtensionID: extensionID,
		Version:     version,
		Commands:    commands,
	}
}

// buildCommands recursively builds Command entries from Cobra subcommands.
func buildCommands(cmd *cobra.Command) []Command {
	var commands []Command
	
	for _, sub := range cmd.Commands() {
		// Skip hidden commands and help command
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "__commands" {
			continue
		}
		
		command := Command{
			Name:        sub.Name(),
			Description: sub.Short,
			Subcommands: buildCommands(sub),
			Flags:       buildFlags(sub),
			Args:        buildArgs(sub),
		}
		
		commands = append(commands, command)
	}
	
	return commands
}

// buildFlags extracts flags from a Cobra command.
func buildFlags(cmd *cobra.Command) []Flag {
	var flags []Flag
	
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, Flag{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Description: f.Usage,
		})
	})
	
	return flags
}

// buildArgs extracts positional arguments from a Cobra command's Use field.
func buildArgs(cmd *cobra.Command) []Arg {
	// Parse args from Use field (e.g., "command [optional] <required>")
	useParts := strings.Fields(cmd.Use)
	if len(useParts) <= 1 {
		return nil
	}
	
	var args []Arg
	for _, part := range useParts[1:] {
		if strings.HasPrefix(part, "-") {
			continue // Skip flags
		}
		
		isOptional := strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]")
		argName := strings.Trim(part, "[]<>")
		
		args = append(args, Arg{
			Name:     argName,
			Optional: isOptional,
		})
	}
	
	return args
}
```

### Task 2: Update Demo Extension to Use `NewCommandTreeCommand`

**File**: `cli/azd/extensions/microsoft.azd.demo/internal/cmd/root.go`

```go
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "azd demo <command> [options]",
		Short:         "Demonstrates AZD extension framework capabilities.",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")

	rootCmd.AddCommand(newListenCommand())
	rootCmd.AddCommand(newContextCommand())
	rootCmd.AddCommand(newPromptCommand())
	rootCmd.AddCommand(newColorsCommand())
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newMcpCommand())
	rootCmd.AddCommand(newGhUrlParseCommand())
	
	// Add hidden command tree export for telemetry
	rootCmd.AddCommand(azdext.NewCommandTreeCommand(rootCmd, "microsoft.azd.demo", version.Version))

	return rootCmd
}
```

### Task 3: Update `azd x pack` to Generate `commands.json`

**File**: `cli/azd/extensions/microsoft.azd.extensions/internal/cmd/pack.go`

Add a new task to generate `commands.json` before creating archives:

```go
func runPackageAction(ctx context.Context, flags *packageFlags) error {
	// ... existing code ...

	taskList := ux.NewTaskList(nil).
		AddTask(ux.TaskOptions{
			Title: "Building extension",
			// ... existing build task ...
		}).
		AddTask(ux.TaskOptions{
			Title: "Generating command spec",
			Action: func(spf ux.SetProgressFunc) (ux.TaskState, error) {
				if err := generateCommandSpec(extensionMetadata, absInputPath); err != nil {
					// Log warning but don't fail - commands.json is optional
					log.Printf("Warning: failed to generate command spec: %v", err)
					return ux.Skipped, nil
				}
				return ux.Success, nil
			},
		}).
		AddTask(ux.TaskOptions{
			Title: "Packaging extension",
			// ... existing package task ...
		})

	return taskList.Run()
}

// generateCommandSpec attempts to generate commands.json by:
// 1. Checking if commands.json already exists in extension directory
// 2. If not, invoking the binary with __commands flag
func generateCommandSpec(extensionMetadata *models.ExtensionSchema, binPath string) error {
	commandsJsonPath := filepath.Join(extensionMetadata.Path, "commands.json")
	
	// Check if commands.json already exists (manual creation)
	if _, err := os.Stat(commandsJsonPath); err == nil {
		return nil // Already exists, use it
	}
	
	// Find a binary for the current platform to invoke
	binaryPath, err := findCurrentPlatformBinary(extensionMetadata, binPath)
	if err != nil {
		return fmt.Errorf("no binary found for current platform: %w", err)
	}
	
	// Invoke binary with __commands
	cmd := exec.Command(binaryPath, "__commands")
	output, err := cmd.Output()
	if err != nil {
		// Binary might not support __commands (non-Go or old extension)
		return fmt.Errorf("binary does not support __commands: %w", err)
	}
	
	// Validate JSON
	var spec map[string]interface{}
	if err := json.Unmarshal(output, &spec); err != nil {
		return fmt.Errorf("invalid JSON from __commands: %w", err)
	}
	
	// Write commands.json
	if err := os.WriteFile(commandsJsonPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write commands.json: %w", err)
	}
	
	return nil
}

func findCurrentPlatformBinary(extensionMetadata *models.ExtensionSchema, binPath string) (string, error) {
	// Determine current OS/arch
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	
	// Look for matching binary
	pattern := fmt.Sprintf("%s-%s-%s", extensionMetadata.SafeDashId(), goos, goarch)
	entries, err := os.ReadDir(binPath)
	if err != nil {
		return "", err
	}
	
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), pattern) {
			return filepath.Join(binPath, entry.Name()), nil
		}
	}
	
	return "", fmt.Errorf("no binary matching %s found", pattern)
}
```

Also update `packExtensionBinaries` to include `commands.json` in archives:

```go
func packExtensionBinaries(
	extensionMetadata *models.ExtensionSchema,
	outputPath string,
) error {
	// ... existing code ...

	extensionYamlSourcePath := filepath.Join(extensionMetadata.Path, "extension.yaml")
	commandsJsonSourcePath := filepath.Join(extensionMetadata.Path, "commands.json")

	// Map and copy artifacts
	for _, entry := range entries {
		// ... existing binary filtering ...

		sourceFiles := []string{extensionYamlSourcePath, artifactSourcePath}
		
		// Include commands.json if it exists
		if _, err := os.Stat(commandsJsonSourcePath); err == nil {
			sourceFiles = append(sourceFiles, commandsJsonSourcePath)
		}

		_, err := createArchive(artifactName, fileWithoutExt, outputPath, sourceFiles)
		// ... rest of existing code ...
	}

	return nil
}
```

### Task 4: Add Command Spec Types to Core Extensions Package

**File**: `cli/azd/pkg/extensions/command_spec.go` (new file)

```go
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package extensions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommandSpec represents the command tree specification for an extension.
type CommandSpec struct {
	ExtensionID string    `json:"extensionId"`
	Version     string    `json:"version"`
	Commands    []Command `json:"commands"`
}

// Command represents a command or subcommand.
type Command struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Subcommands []Command `json:"subcommands,omitempty"`
	Flags       []Flag    `json:"flags,omitempty"`
	Args        []Arg     `json:"args,omitempty"`
}

// Flag represents a command flag.
type Flag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description,omitempty"`
}

// Arg represents a positional argument.
type Arg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

// LoadCommandSpec loads a CommandSpec from a commands.json file.
// Returns nil if the file doesn't exist or is invalid.
func LoadCommandSpec(extensionPath string) *CommandSpec {
	commandsPath := filepath.Join(filepath.Dir(extensionPath), "commands.json")
	
	data, err := os.ReadFile(commandsPath)
	if err != nil {
		return nil
	}
	
	var spec CommandSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil
	}
	
	return &spec
}

// MatchCommandPath attempts to match the given arguments against known commands.
// Returns the matched command path (e.g., "mcp serve") or "unknown" if no match.
func (s *CommandSpec) MatchCommandPath(args []string) string {
	if s == nil || len(args) == 0 {
		return "unknown"
	}
	
	return matchCommands(s.Commands, args, "")
}

// matchCommands recursively matches args against commands.
func matchCommands(commands []Command, args []string, prefix string) string {
	if len(args) == 0 {
		if prefix == "" {
			return "unknown"
		}
		return strings.TrimSpace(prefix)
	}
	
	arg := args[0]
	
	// Skip flags (start with -)
	if strings.HasPrefix(arg, "-") {
		return matchCommands(commands, args[1:], prefix)
	}
	
	// Try to find matching command
	for _, cmd := range commands {
		if cmd.Name == arg {
			newPrefix := prefix + " " + cmd.Name
			
			// If command has subcommands, try to match deeper
			if len(cmd.Subcommands) > 0 && len(args) > 1 {
				result := matchCommands(cmd.Subcommands, args[1:], newPrefix)
				if result != "unknown" {
					return result
				}
			}
			
			// Return current match
			return strings.TrimSpace(newPrefix)
		}
	}
	
	// No match found - if we have a prefix, return it (partial match)
	if prefix != "" {
		return strings.TrimSpace(prefix)
	}
	
	return "unknown"
}
```

### Task 5: Update Extension Manager to Load Command Spec

**File**: `cli/azd/pkg/extensions/manager.go`

Update the extension loading to also load command specs:

```go
// In the Extension struct, add:
type Extension struct {
	// ... existing fields ...
	CommandSpec *CommandSpec `json:"-"` // Loaded at runtime, not persisted
}

// Update GetInstalled or add a method to load command spec
func (m *Manager) GetInstalledWithCommands(options FilterOptions) (*Extension, error) {
	ext, err := m.GetInstalled(options)
	if err != nil {
		return nil, err
	}
	
	// Load command spec if available
	ext.CommandSpec = LoadCommandSpec(ext.Path)
	
	return ext, nil
}
```

### Task 6: Add Telemetry Field for Extension Command

**File**: `cli/azd/internal/tracing/fields/fields.go`

Add new field:

```go
// Extension related fields
var (
	// ... existing fields ...
	
	// The command path within the extension (e.g., "mcp serve").
	ExtensionCommand = AttributeKey{
		Key:            attribute.Key("extension.command"),
		Classification: SystemMetadata,
		Purpose:        FeatureInsight,
	}
)
```

### Task 7: Update Extension Action to Log Command Path

**File**: `cli/azd/cmd/extensions.go`

Update the `Run` method to match and log command path:

```go
func (a *extensionAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	extensionId, has := a.cmd.Annotations["extension.id"]
	if !has {
		return nil, fmt.Errorf("extension id not found")
	}

	extension, err := a.extensionManager.GetInstalled(extensions.FilterOptions{
		Id: extensionId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get extension %s: %w", extensionId, err)
	}

	// Load command spec and match command path
	commandSpec := extensions.LoadCommandSpec(extension.Path)
	commandPath := "unknown"
	if commandSpec != nil {
		commandPath = commandSpec.MatchCommandPath(a.args)
	}

	tracing.SetUsageAttributes(
		fields.ExtensionId.String(extension.Id),
		fields.ExtensionVersion.String(extension.Version),
		fields.ExtensionCommand.String(commandPath),
	)

	// ... rest of existing code ...
}
```

### Task 8: Update Extension Install to Extract commands.json

**File**: `cli/azd/pkg/extensions/manager.go`

The existing install logic extracts files from the archive. Ensure `commands.json` is preserved alongside `extension.yaml` and the binary. No changes may be needed if the archive extraction already extracts all files.

### Task 9: Documentation Updates

**File**: `cli/azd/docs/extension-framework.md`

Add a new section documenting the command spec feature:

```markdown
### Command Telemetry

Extensions can provide a command specification to enable accurate telemetry for command usage.

#### For Go Extensions (Automatic)

Add the command tree export to your root command:

\`\`\`go
import "github.com/azure/azure-dev/cli/azd/pkg/azdext"

func NewRootCommand() *cobra.Command {
    rootCmd := &cobra.Command{...}
    
    // Add subcommands...
    
    // Add command tree export for telemetry (required for Go extensions)
    rootCmd.AddCommand(azdext.NewCommandTreeCommand(rootCmd, "your.extension.id", version))
    
    return rootCmd
}
\`\`\`

When you run `azd x pack`, the command spec is automatically generated.

#### For Non-Go Extensions (Manual)

Create a `commands.json` file in your extension root directory following the schema at `extensions/commands.schema.json`.

Alternatively, implement a `__commands` subcommand that outputs the JSON to stdout.

#### Schema

See `cli/azd/extensions/commands.schema.json` for the full schema. Example:

\`\`\`json
{
  "extensionId": "my.extension",
  "version": "1.0.0",
  "commands": [
    {
      "name": "deploy",
      "description": "Deploy resources"
    },
    {
      "name": "config",
      "subcommands": [
        {"name": "get"},
        {"name": "set"}
      ]
    }
  ]
}
\`\`\`
```

### Task 10: Enrich Fig spec generation using `commands.json`

Update the Fig spec generator to recognize extension placeholder nodes and inject additional command/flag metadata from `commands.json`.

Key behaviors:

- For each Cobra command that has annotation `extension.id`, load the installed extension's `commands.json`.
- Replace/extend the generated Fig `Subcommand` (for that Cobra command) with:
	- nested `subcommands` from `commands.json`
	- `options` from `commands.json` for each command node

If `commands.json` is missing or invalid, fall back to current behavior (namespace-only completions).

Also implement the namespace collision policy described above:

- If using Option 2A: enforce uniqueness at install time and keep spec generation simple.
- If using Option 2B: merge multiple specs under the same namespace node and ensure disjoint subcommand ownership.

## File Summary

### New Files

| File | Purpose |
|------|---------|
| `cli/azd/pkg/azdext/command_tree.go` | `NewCommandTreeCommand()` and types for Go extensions |
| `cli/azd/pkg/extensions/command_spec.go` | `CommandSpec` types and `MatchCommandPath()` logic |
| `cli/azd/extensions/commands.schema.json` | JSON schema for `commands.json` |

### Modified Files

| File | Changes |
|------|---------|
| `cli/azd/extensions/microsoft.azd.extensions/internal/cmd/pack.go` | Generate and bundle `commands.json` |
| `cli/azd/extensions/microsoft.azd.demo/internal/cmd/root.go` | Add `NewCommandTreeCommand()` call |
| `cli/azd/internal/tracing/fields/fields.go` | Add `ExtensionCommand` field |
| `cli/azd/cmd/extensions.go` | Load command spec and log command path |
| `cli/azd/docs/extension-framework.md` | Document the feature |

## Testing Plan

1. **Unit Tests**:
   - `command_tree_test.go`: Test `BuildCommandSpec()` builds correct tree from Cobra commands
   - `command_spec_test.go`: Test `MatchCommandPath()` with various arg patterns

2. **Integration Tests**:
   - Pack demo extension and verify `commands.json` is in archive
   - Install extension and verify command path appears in telemetry

3. **Manual Testing**:
   - Run `azd demo context` and verify telemetry shows `extension.command: "context"`
   - Run `azd demo mcp serve` and verify telemetry shows `extension.command: "mcp serve"`
   - Run extension with unknown args and verify `extension.command: "unknown"`

## Rollout Plan

1. **Phase 1**: Implement core functionality
   - Add `azdext.NewCommandTreeCommand()`
   - Add `CommandSpec` types and matching logic
   - Update demo extension as reference implementation

2. **Phase 2**: Integrate with pack/install
   - Update `azd x pack` to generate `commands.json`
   - Ensure `commands.json` is extracted on install

3. **Phase 3**: Telemetry integration
   - Add `ExtensionCommand` telemetry field
   - Update extension action to log command path

4. **Phase 4**: Documentation and rollout
   - Update extension framework docs
	- Update all official extensions to use `NewCommandTreeCommand()`
	- Update the azd Fig spec snapshot to include enriched extension commands
