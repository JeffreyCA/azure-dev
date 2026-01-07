// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchCommandPath(t *testing.T) {
	spec := &CommandSpec{
		Commands: []CommandEntry{
			{Name: "context", Description: "Show context"},
			{Name: "prompt", Description: "Show prompt"},
			{
				Name:        "mcp",
				Description: "MCP commands",
				Subcommands: []CommandEntry{
					{Name: "serve", Description: "Start server"},
					{Name: "stop", Description: "Stop server"},
				},
			},
			{
				Name:        "config",
				Description: "Configuration commands",
				Subcommands: []CommandEntry{
					{
						Name:        "set",
						Description: "Set configuration",
						Subcommands: []CommandEntry{
							{Name: "key", Description: "Set a key"},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "simple command",
			args:     []string{"context"},
			expected: "context",
		},
		{
			name:     "another simple command",
			args:     []string{"prompt"},
			expected: "prompt",
		},
		{
			name:     "subcommand",
			args:     []string{"mcp", "serve"},
			expected: "mcp serve",
		},
		{
			name:     "subcommand with extra args",
			args:     []string{"mcp", "serve", "--port", "8080"},
			expected: "mcp serve",
		},
		{
			name:     "deeply nested subcommand",
			args:     []string{"config", "set", "key"},
			expected: "config set key",
		},
		{
			name:     "command with positional args",
			args:     []string{"context", "myenv"},
			expected: "context",
		},
		{
			name:     "flags before command",
			args:     []string{"--debug", "context"},
			expected: "context",
		},
		{
			name:     "unknown command",
			args:     []string{"unknown"},
			expected: "unknown",
		},
		{
			name:     "empty args",
			args:     []string{},
			expected: "unknown",
		},
		{
			name:     "parent command only",
			args:     []string{"mcp"},
			expected: "mcp",
		},
		{
			name:     "parent with unknown subcommand",
			args:     []string{"mcp", "unknown"},
			expected: "mcp",
		},
		{
			name:     "only flags",
			args:     []string{"--help", "--debug"},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spec.MatchCommandPath(tt.args)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchCommandPath_NilSpec(t *testing.T) {
	var spec *CommandSpec
	result := spec.MatchCommandPath([]string{"context"})
	require.Equal(t, "unknown", result)
}

func TestLoadCommandSpec(t *testing.T) {
	// Create a temporary directory with commands.json
	tempDir := t.TempDir()

	spec := &CommandSpec{
		Commands: []CommandEntry{
			{Name: "test", Description: "Test command"},
		},
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, "commands.json"), data, 0600)
	require.NoError(t, err)

	// Create a fake binary path (doesn't need to exist)
	binaryPath := filepath.Join(tempDir, "extension")

	// Load the spec
	loaded := LoadCommandSpec(binaryPath)
	require.NotNil(t, loaded)
	require.Len(t, loaded.Commands, 1)
	require.Equal(t, "test", loaded.Commands[0].Name)
}

func TestLoadCommandSpec_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	binaryPath := filepath.Join(tempDir, "extension")

	loaded := LoadCommandSpec(binaryPath)
	require.Nil(t, loaded)
}

func TestLoadCommandSpec_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tempDir, "commands.json"), []byte("not valid json"), 0600)
	require.NoError(t, err)

	binaryPath := filepath.Join(tempDir, "extension")

	loaded := LoadCommandSpec(binaryPath)
	require.Nil(t, loaded)
}

func TestMatchCommandPath_WithFlagsThatTakeValues(t *testing.T) {
	spec := &CommandSpec{
		Commands: []CommandEntry{
			{
				Name:        "serve",
				Description: "Start server",
				Flags: []FlagEntry{
					{Name: "port", Shorthand: "p", TakesValue: true},
					{Name: "host", Shorthand: "h", TakesValue: true},
					{Name: "verbose", Shorthand: "v", TakesValue: false},
				},
			},
			{
				Name:        "deploy",
				Description: "Deploy app",
				Subcommands: []CommandEntry{
					{
						Name: "prod",
						Flags: []FlagEntry{
							{Name: "env", Shorthand: "e", TakesValue: true},
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "flag with value before command",
			args:     []string{"--port", "8080", "serve"},
			expected: "serve",
		},
		{
			name:     "short flag with value before command",
			args:     []string{"-p", "8080", "serve"},
			expected: "serve",
		},
		{
			name:     "flag=value syntax before command",
			args:     []string{"--port=8080", "serve"},
			expected: "serve",
		},
		{
			name:     "boolean flag before command",
			args:     []string{"--verbose", "serve"},
			expected: "serve",
		},
		{
			name:     "mixed flags before command",
			args:     []string{"--verbose", "--port", "8080", "serve"},
			expected: "serve",
		},
		{
			name:     "subcommand with flag value",
			args:     []string{"deploy", "--env", "production", "prod"},
			expected: "deploy prod",
		},
		{
			name:     "command then flags",
			args:     []string{"serve", "--port", "8080", "--verbose"},
			expected: "serve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spec.MatchCommandPath(tt.args)
			require.Equal(t, tt.expected, result)
		})
	}
}
