// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// CommandSpec represents the command tree specification for an extension.
type CommandSpec struct {
	Commands []CommandEntry `json:"commands"`
}

// CommandEntry represents a command or subcommand.
type CommandEntry struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Subcommands []CommandEntry `json:"subcommands,omitempty"`
	Flags       []FlagEntry    `json:"flags,omitempty"`
}

// FlagEntry represents a command flag.
type FlagEntry struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description,omitempty"`
	TakesValue  bool   `json:"takesValue,omitempty"`
}

// LoadCommandSpec loads a CommandSpec from a commands.json file.
// Returns nil if the file doesn't exist or is invalid.
// extensionPath is expected to be the path to the extension binary.
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
//
// The matching algorithm:
// 1. Build a map of flags that take values from the current command level
// 2. Skip flags and their values appropriately
// 3. Match the first non-flag arg against known command names
// 4. Recurse into subcommands if matched
func (s *CommandSpec) MatchCommandPath(args []string) string {
	if s == nil || len(args) == 0 {
		return "unknown"
	}

	return matchCommandsWithFlags(s.Commands, args)
}

// matchCommandsWithFlags matches args against commands, properly handling flags.
func matchCommandsWithFlags(commands []CommandEntry, args []string) string {
	if len(commands) == 0 || len(args) == 0 {
		return "unknown"
	}

	// Build set of flag names for all commands at this level
	// This is a simplification - we check flags from all sibling commands
	flagsWithValues := buildFlagsWithValues(commands)

	// Find the first non-flag argument (the command name)
	cmdName, remainingArgs := findCommand(args, flagsWithValues)
	if cmdName == "" {
		return "unknown"
	}

	// Try to find matching command
	for _, cmd := range commands {
		if cmd.Name == cmdName {
			// If command has subcommands, try to match deeper
			if len(cmd.Subcommands) > 0 && len(remainingArgs) > 0 {
				subPath := matchCommandsWithFlags(cmd.Subcommands, remainingArgs)
				if subPath != "unknown" {
					return cmd.Name + " " + subPath
				}
			}
			// Return current match
			return cmd.Name
		}
	}

	return "unknown"
}

// buildFlagsWithValues builds a map of flags that take values from command entries.
func buildFlagsWithValues(commands []CommandEntry) map[string]bool {
	flagsWithValues := make(map[string]bool)

	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			if flag.TakesValue {
				flagsWithValues["--"+flag.Name] = true
				if flag.Shorthand != "" {
					flagsWithValues["-"+flag.Shorthand] = true
				}
			}
		}
	}

	return flagsWithValues
}

// findCommand finds the first non-flag argument and returns remaining args.
// This properly handles flags that take values.
func findCommand(args []string, flagsWithValues map[string]bool) (command string, remaining []string) {
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// If it doesn't start with '-', it's a command
		if !strings.HasPrefix(arg, "-") {
			return arg, args[i+1:]
		}

		// Handle --flag=value syntax
		if strings.Contains(arg, "=") {
			continue
		}

		// Check if this flag takes a value
		if flagsWithValues[arg] {
			skipNext = true
			continue
		}

		// Unknown flag without known value - assume it's a boolean flag
		// This is safe because if it did take a value, the next arg won't
		// match any command name anyway
	}

	return "", nil
}
