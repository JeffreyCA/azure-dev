// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandSpec represents the command tree specification for an extension.
type CommandSpec struct {
	Commands []Command `json:"commands"`
}

// Command represents a command or subcommand in the extension.
type Command struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Subcommands []Command `json:"subcommands,omitempty"`
	Flags       []Flag    `json:"flags,omitempty"`
}

// Flag represents a flag/option for a command.
type Flag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Description string `json:"description,omitempty"`
	TakesValue  bool   `json:"takesValue,omitempty"`
}

// NewCommandTreeCommand creates a hidden command that outputs the command tree as JSON.
// Extension authors should add this to their root command:
//
//	rootCmd.AddCommand(azdext.NewCommandTreeCommand(rootCmd))
//
// Note: The caller (azd x pack) should set NO_COLOR=1 to ensure clean output.
func NewCommandTreeCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:    "__commands",
		Hidden: true,
		Short:  "Output command tree as JSON (internal use)",
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := BuildCommandSpec(root)

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
func BuildCommandSpec(root *cobra.Command) *CommandSpec {
	return &CommandSpec{
		Commands: buildCommands(root),
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
		flag := Flag{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Description: f.Usage,
			TakesValue:  f.Value.Type() != "bool",
		}
		flags = append(flags, flag)
	})

	return flags
}
