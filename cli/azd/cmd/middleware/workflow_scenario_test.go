// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package middleware

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/pkg/tool"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestScenarioForCommandPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		commandPath  string
		wantScenario string
		wantOk       bool
	}{
		{
			name:         "WorkflowCommandResolvesCore",
			commandPath:  "azd up",
			wantScenario: tool.ScenarioCore,
			wantOk:       true,
		},
		{
			name:         "InitResolvesCore",
			commandPath:  "azd init",
			wantScenario: tool.ScenarioCore,
			wantOk:       true,
		},
		{
			name: "NestedWorkflowCommandUsesTopLevelSegment",
			// A deeper command under a workflow root still resolves via its
			// top-level segment, matching the pre-refactor parent-walk
			// behavior.
			commandPath:  "azd package list",
			wantScenario: tool.ScenarioCore,
			wantOk:       true,
		},
		{
			name:        "ExtensionNamespaceIsNotAWorkflowCommand",
			commandPath: "azd ai agent",
			wantOk:      false,
		},
		{
			name:        "UtilityCommandIsNotAWorkflowCommand",
			commandPath: "azd extension list",
			wantOk:      false,
		},
		{
			name:        "RootOnlyPathIsNotAWorkflowCommand",
			commandPath: "azd",
			wantOk:      false,
		},
		{
			name:        "EmptyPathIsNotAWorkflowCommand",
			commandPath: "",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scenario, ok := ScenarioForCommandPath(tt.commandPath)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantScenario, scenario)
			} else {
				assert.Empty(t, scenario)
			}
		})
	}
}

func TestIsWorkflowCommand(t *testing.T) {
	t.Parallel()

	// newDescriptor builds an ActionDescriptor whose cobra command is
	// parented under a root "azd" command, so CommandPath() returns the
	// full path the predicate parses (e.g. "azd up").
	newDescriptor := func(use string) *actions.ActionDescriptor {
		root := &cobra.Command{Use: "azd"}
		child := &cobra.Command{Use: use}
		root.AddCommand(child)
		return actions.NewActionDescriptor(use, &actions.ActionDescriptorOptions{Command: child})
	}

	t.Run("WorkflowCommandMatches", func(t *testing.T) {
		t.Parallel()
		assert.True(t, IsWorkflowCommand(newDescriptor("up")))
	})

	t.Run("UtilityCommandDoesNotMatch", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsWorkflowCommand(newDescriptor("version")))
	})

	t.Run("NilDescriptorIsSafe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsWorkflowCommand(nil))
	})

	t.Run("NilOptionsIsSafe", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsWorkflowCommand(&actions.ActionDescriptor{}))
	})
}
