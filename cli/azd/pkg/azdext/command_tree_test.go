// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandSpec(t *testing.T) {
	tests := []struct {
		name      string
		setupRoot func() *cobra.Command
		expected  *CommandSpec
	}{
		{
			name: "simple commands",
			setupRoot: func() *cobra.Command {
				root := &cobra.Command{Use: "root"}
				root.AddCommand(&cobra.Command{Use: "cmd1", Short: "First command"})
				root.AddCommand(&cobra.Command{Use: "cmd2", Short: "Second command"})
				return root
			},
			expected: &CommandSpec{
				Commands: []Command{
					{Name: "cmd1", Description: "First command"},
					{Name: "cmd2", Description: "Second command"},
				},
			},
		},
		{
			name: "nested subcommands",
			setupRoot: func() *cobra.Command {
				root := &cobra.Command{Use: "root"}
				parent := &cobra.Command{Use: "parent", Short: "Parent command"}
				child := &cobra.Command{Use: "child", Short: "Child command"}
				parent.AddCommand(child)
				root.AddCommand(parent)
				return root
			},
			expected: &CommandSpec{
				Commands: []Command{
					{
						Name:        "parent",
						Description: "Parent command",
						Subcommands: []Command{
							{Name: "child", Description: "Child command"},
						},
					},
				},
			},
		},
		{
			name: "skips hidden commands",
			setupRoot: func() *cobra.Command {
				root := &cobra.Command{Use: "root"}
				root.AddCommand(&cobra.Command{Use: "visible", Short: "Visible"})
				root.AddCommand(&cobra.Command{Use: "hidden", Short: "Hidden", Hidden: true})
				root.AddCommand(&cobra.Command{Use: "__commands", Short: "Internal"})
				return root
			},
			expected: &CommandSpec{
				Commands: []Command{
					{Name: "visible", Description: "Visible"},
				},
			},
		},
		{
			name: "command with flags",
			setupRoot: func() *cobra.Command {
				root := &cobra.Command{Use: "root"}
				cmd := &cobra.Command{Use: "cmd", Short: "Command with flags"}
				cmd.Flags().StringP("output", "o", "", "Output format")
				cmd.Flags().BoolP("verbose", "v", false, "Verbose output")
				root.AddCommand(cmd)
				return root
			},
			expected: &CommandSpec{
				Commands: []Command{
					{
						Name:        "cmd",
						Description: "Command with flags",
						Flags: []Flag{
							{Name: "output", Shorthand: "o", Description: "Output format", TakesValue: true},
							{Name: "verbose", Shorthand: "v", Description: "Verbose output"},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.setupRoot()
			result := BuildCommandSpec(root)

			require.Equal(t, len(tt.expected.Commands), len(result.Commands))

			for i, expectedCmd := range tt.expected.Commands {
				assertCommandEqual(t, expectedCmd, result.Commands[i])
			}
		})
	}
}

func assertCommandEqual(t *testing.T, expected, actual Command) {
	t.Helper()
	require.Equal(t, expected.Name, actual.Name, "command name mismatch")
	require.Equal(t, expected.Description, actual.Description, "description mismatch for %s", expected.Name)
	require.Equal(t, len(expected.Subcommands), len(actual.Subcommands), "subcommand count mismatch for %s", expected.Name)
	require.Equal(t, len(expected.Flags), len(actual.Flags), "flag count mismatch for %s", expected.Name)

	for i, expectedSub := range expected.Subcommands {
		assertCommandEqual(t, expectedSub, actual.Subcommands[i])
	}

	for i, expectedFlag := range expected.Flags {
		require.Equal(t, expectedFlag.Name, actual.Flags[i].Name)
		require.Equal(t, expectedFlag.Shorthand, actual.Flags[i].Shorthand)
		require.Equal(t, expectedFlag.Description, actual.Flags[i].Description)
		require.Equal(t, expectedFlag.TakesValue, actual.Flags[i].TakesValue)
	}
}

func TestNewCommandTreeCommand(t *testing.T) {
	root := &cobra.Command{
		Use: "root",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.AddCommand(&cobra.Command{Use: "test", Short: "Test command"})

	cmdTreeCmd := NewCommandTreeCommand(root)
	root.AddCommand(cmdTreeCmd)

	// Verify the command is hidden
	require.True(t, cmdTreeCmd.Hidden)
	require.Equal(t, "__commands", cmdTreeCmd.Name())

	// Execute the __commands command by setting args and running root
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"__commands"})
	err := root.Execute()
	require.NoError(t, err)

	var spec CommandSpec
	err = json.Unmarshal(buf.Bytes(), &spec)
	require.NoError(t, err)
	require.Len(t, spec.Commands, 1)
	require.Equal(t, "test", spec.Commands[0].Name)
}
