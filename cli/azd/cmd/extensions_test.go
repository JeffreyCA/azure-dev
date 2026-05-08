// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"slices"
	"testing"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// findChildByName returns the child action descriptor with the given name, or nil if not found.
func findChildByName(parent *actions.ActionDescriptor, name string) *actions.ActionDescriptor {
	idx := slices.IndexFunc(parent.Children(), func(child *actions.ActionDescriptor) bool {
		return child.Name == name
	})
	if idx == -1 {
		return nil
	}
	return parent.Children()[idx]
}

// newTestRoot creates a new root action descriptor for testing.
func newTestRoot() *actions.ActionDescriptor {
	return actions.NewActionDescriptor("azd", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{Use: "azd", Short: "Azure Developer CLI"},
	})
}

func TestBindExtension_SharedNamespacePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                      string
		extensions                []*extensions.Extension
		expectedIntermediateDesc  string
		expectedIntermediateNames []string
	}{
		{
			name: "two extensions share 'ai' prefix",
			extensions: []*extensions.Extension{
				{
					Id:          "azure.ai.agents",
					Namespace:   "ai.agents",
					DisplayName: "AI Agents Extension",
					Description: "Extension for the Foundry Agent Service. (Preview)",
				},
				{
					Id:          "azure.ai.finetune",
					Namespace:   "ai.finetune",
					DisplayName: "AI Fine Tune Extension",
					Description: "Extension for Foundry Fine Tuning. (Preview)",
				},
			},
			expectedIntermediateDesc:  "Commands for the ai extension namespace.",
			expectedIntermediateNames: []string{"ai"},
		},
		{
			name: "single extension with nested namespace",
			extensions: []*extensions.Extension{
				{
					Id:          "azure.ai.agents",
					Namespace:   "ai.agents",
					DisplayName: "AI Agents Extension",
					Description: "Extension for the Foundry Agent Service. (Preview)",
				},
			},
			expectedIntermediateDesc:  "Commands for the ai extension namespace.",
			expectedIntermediateNames: []string{"ai"},
		},
		{
			name: "extension with simple namespace (no intermediate)",
			extensions: []*extensions.Extension{
				{
					Id:          "microsoft.azd.demo",
					Namespace:   "demo",
					DisplayName: "Demo Extension",
					Description: "This extension provides examples of the AZD extension framework.",
				},
			},
			expectedIntermediateDesc:  "",
			expectedIntermediateNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRoot()

			for _, ext := range tt.extensions {
				require.NoError(t, bindExtension(root, ext))
			}

			for _, intermediateName := range tt.expectedIntermediateNames {
				intermediateCmd := findChildByName(root, intermediateName)
				require.NotNil(t, intermediateCmd, "intermediate command %s should exist", intermediateName)
				require.Equal(t, tt.expectedIntermediateDesc, intermediateCmd.Options.Command.Short,
					"intermediate namespace command should have generic description")
			}
		})
	}
}

func TestBindExtension_DeterministicOrder(t *testing.T) {
	t.Parallel()
	ext1 := &extensions.Extension{
		Id:          "azure.ai.agents",
		Namespace:   "ai.agents",
		DisplayName: "AI Agents Extension",
		Description: "Extension for the Foundry Agent Service. (Preview)",
	}

	ext2 := &extensions.Extension{
		Id:          "azure.ai.finetune",
		Namespace:   "ai.finetune",
		DisplayName: "AI Fine Tune Extension",
		Description: "Extension for Foundry Fine Tuning. (Preview)",
	}

	// Test order 1: agents first
	root1 := newTestRoot()
	require.NoError(t, bindExtension(root1, ext1))
	require.NoError(t, bindExtension(root1, ext2))

	// Test order 2: finetune first
	root2 := newTestRoot()
	require.NoError(t, bindExtension(root2, ext2))
	require.NoError(t, bindExtension(root2, ext1))

	aiCmd1 := findChildByName(root1, "ai")
	aiCmd2 := findChildByName(root2, "ai")

	require.NotNil(t, aiCmd1)
	require.NotNil(t, aiCmd2)
	require.Equal(t, aiCmd1.Options.Command.Short, aiCmd2.Options.Command.Short,
		"intermediate namespace description should be consistent regardless of binding order")
	require.Equal(t, "Commands for the ai extension namespace.", aiCmd1.Options.Command.Short)
}

func TestBindExtension_DeeplyNestedNamespace(t *testing.T) {
	t.Parallel()
	ext1 := &extensions.Extension{
		Id:          "azure.ai.models.finetune",
		Namespace:   "ai.models.finetune",
		DisplayName: "AI Models Fine Tune",
		Description: "Extension for fine tuning AI models.",
	}

	ext2 := &extensions.Extension{
		Id:          "azure.ai.models.eval",
		Namespace:   "ai.models.eval",
		DisplayName: "AI Models Eval",
		Description: "Extension for evaluating AI models.",
	}

	root := newTestRoot()
	require.NoError(t, bindExtension(root, ext1))
	require.NoError(t, bindExtension(root, ext2))

	// Verify "ai" intermediate command
	aiCmd := findChildByName(root, "ai")
	require.NotNil(t, aiCmd)
	require.Equal(t, "Commands for the ai extension namespace.", aiCmd.Options.Command.Short)

	// Verify "models" intermediate command under "ai"
	modelsCmd := findChildByName(aiCmd, "models")
	require.NotNil(t, modelsCmd)
	require.Equal(t, "Commands for the ai.models extension namespace.", modelsCmd.Options.Command.Short)

	// Verify leaf commands exist and have correct descriptions
	finetuneCmd := findChildByName(modelsCmd, "finetune")
	evalCmd := findChildByName(modelsCmd, "eval")

	require.NotNil(t, finetuneCmd)
	require.NotNil(t, evalCmd)
	require.Equal(t, "Extension for fine tuning AI models.", finetuneCmd.Options.Command.Short)
	require.Equal(t, "Extension for evaluating AI models.", evalCmd.Options.Command.Short)
}

// TestBindExtension_HybridLeafAndParent verifies that two extensions where one occupies
// a parent namespace ("ai") and another occupies a nested namespace ("ai.finetune") can
// be bound in either order without producing duplicate cobra children. The shared "ai"
// node ends up as a hybrid: a leaf with the parent extension's action and annotations,
// plus a child subcommand contributed by the nested extension.
func TestBindExtension_HybridLeafAndParent(t *testing.T) {
	t.Parallel()

	leafExt := &extensions.Extension{
		Id:          "microsoft.azd.ai.builder",
		Namespace:   "ai",
		DisplayName: "AI Builder",
		Description: "AI Builder extension.",
	}
	nestedExt := &extensions.Extension{
		Id:          "azure.ai.finetune",
		Namespace:   "ai.finetune",
		DisplayName: "AI Fine Tune",
		Description: "AI Fine Tune extension.",
	}

	cases := []struct {
		name string
		bind []*extensions.Extension
	}{
		{name: "leafFirst", bind: []*extensions.Extension{leafExt, nestedExt}},
		{name: "nestedFirst", bind: []*extensions.Extension{nestedExt, leafExt}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newTestRoot()
			for _, ext := range tc.bind {
				require.NoError(t, bindExtension(root, ext))
			}

			aiChildren := slices.Collect(func(yield func(*actions.ActionDescriptor) bool) {
				for _, child := range root.Children() {
					if child.Name == "ai" && !yield(child) {
						return
					}
				}
			})
			require.Len(t, aiChildren, 1, "exactly one 'ai' descriptor should exist on root")

			ai := aiChildren[0]

			// Hybrid leaf carries the parent extension's action + annotations.
			require.NotNil(t, ai.Options.ActionResolver,
				"hybrid 'ai' node should expose an ActionResolver from the leaf extension")
			require.Equal(t, leafExt.Id, ai.Options.Command.Annotations[extensionIDAnnotation])
			require.Equal(t, leafExt.Namespace, ai.Options.Command.Annotations[extensionNamespaceAnnotation])
			require.True(t, ai.Options.Command.DisableFlagParsing,
				"hybrid 'ai' node must keep DisableFlagParsing so leaf extension owns its arg parsing")
			require.True(t, ai.Options.DisableTroubleshooting)
			require.Equal(t, "true",
				ai.Options.Command.Annotations[extensionNamespaceOwnerAnnotation],
				"hybrid 'ai' node must remain marked as extension-owned")

			// Hybrid leaves carry the shared-namespace header in both Short and
			// Long; the leaf extension's own description is reachable from the
			// binary's subcommand help, not at the shared namespace level.
			expectedHeader := "Commands for the ai extension namespace."
			require.Equal(t, expectedHeader, ai.Options.Command.Short,
				"hybrid 'ai' node must use the shared-namespace header as Short")
			require.Equal(t, expectedHeader, ai.Options.Command.Long,
				"hybrid 'ai' node must use the shared-namespace header as Long")
			require.NotContains(t, ai.Options.Command.Short, leafExt.Description,
				"hybrid 'ai' node must not surface the leaf description at the shared namespace level")
			require.NotContains(t, ai.Options.Command.Long, leafExt.Description,
				"hybrid 'ai' node must not surface the leaf description at the shared namespace level")

			// Nested extension is reachable as a child subcommand.
			finetune := findChildByName(ai, "finetune")
			require.NotNil(t, finetune)
			require.NotNil(t, finetune.Options.ActionResolver)
			require.Equal(t, nestedExt.Id,
				finetune.Options.Command.Annotations[extensionIDAnnotation])
		})
	}
}

// TestBindExtension_RejectsBuiltInCollision verifies that an extension namespace whose
// top segment matches an existing built-in azd command is rejected at bind time, rather
// than silently attaching the extension to the built-in command's tree.
func TestBindExtension_RejectsBuiltInCollision(t *testing.T) {
	t.Parallel()

	root := newTestRoot()
	// Simulate a built-in azd command (no extension-owner annotation).
	root.Add("auth", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{Use: "auth", Short: "Built-in auth command."},
	})

	cases := []struct {
		name      string
		namespace string
	}{
		{name: "topLevelLeaf", namespace: "auth"},
		{name: "nestedUnderBuiltIn", namespace: "auth.foo"},
		// Case-insensitive: extension namespace "Auth" / "AUTH.foo" must still
		// be detected as colliding with the lowercase built-in command.
		{name: "topLevelLeafMixedCase", namespace: "Auth"},
		{name: "nestedUnderBuiltInMixedCase", namespace: "AUTH.foo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := bindExtension(root, &extensions.Extension{
				Id:          "contoso.auth",
				Namespace:   tc.namespace,
				Description: "Conflicting extension.",
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "collides with the built-in azd command")
			require.Contains(t, err.Error(), "auth")
		})
	}
}

// TestBindExtension_PureLeafKeepsLeafDescription verifies that when no other
// extension shares the namespace, the leaf's Long is the leaf description (not
// padded with the namespace-header line). The header should only appear once
// the node becomes hybrid.
func TestBindExtension_PureLeafKeepsLeafDescription(t *testing.T) {
	t.Parallel()
	root := newTestRoot()
	require.NoError(t, bindExtension(root, &extensions.Extension{
		Id:          "demo.ext",
		Namespace:   "demo",
		Description: "Demo extension.",
	}))
	demo := findChildByName(root, "demo")
	require.NotNil(t, demo)
	require.Equal(t, "Demo extension.", demo.Options.Command.Short)
	require.NotContains(t, demo.Options.Command.Long,
		"Commands for the demo extension namespace.",
		"pure leaves must not carry the shared-namespace header")
}

func TestIsHelpInvocation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no args triggers help", args: nil, want: true},
		{name: "empty args triggers help", args: []string{}, want: true},
		{name: "explicit --help only", args: []string{"--help"}, want: true},
		{name: "explicit -h only", args: []string{"-h"}, want: true},
		{name: "subcommand args do not trigger help", args: []string{"agent"}, want: false},
		{name: "subcommand with --help forwards to binary", args: []string{"agent", "--help"}, want: false},
		{name: "subcommand with extra args", args: []string{"agent", "list"}, want: false},
		{name: "other flags only do not trigger help", args: []string{"--debug"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isHelpInvocation(tc.args))
		})
	}
}

func TestExtensionSiblingChildren(t *testing.T) {
	t.Parallel()
	root := newTestRoot()
	require.NoError(t, bindExtension(root, &extensions.Extension{
		Id:          "leaf.ext",
		Namespace:   "ai",
		Description: "Leaf extension",
	}))
	require.NoError(t, bindExtension(root, &extensions.Extension{
		Id:          "nested.ext",
		Namespace:   "ai.finetune",
		Description: "Nested extension that fine-tunes models",
	}))

	leafDesc := findChildByName(root, "ai")
	require.NotNil(t, leafDesc)
	leafCmd := leafDesc.Options.Command
	leafCmd.Use = "ai"
	for _, child := range leafDesc.Children() {
		leafCmd.AddCommand(child.Options.Command)
	}

	siblings := extensionSiblingChildren(leafCmd)
	require.Len(t, siblings, 1)
	require.Equal(t, "finetune", siblings[0].Name())
}

func TestFormatSiblingExtensionsFallbackNotice(t *testing.T) {
	t.Parallel()

	t.Run("no siblings returns empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", formatSiblingExtensionsFallbackNotice("azd ai", nil))
	})

	t.Run("renders sibling list", func(t *testing.T) {
		t.Parallel()
		sibling := &cobra.Command{
			Use:   "finetune",
			Short: "Fine-tune models",
			Annotations: map[string]string{
				extensionIDAnnotation: "nested.ext",
			},
		}
		notice := formatSiblingExtensionsFallbackNotice("azd ai", []*cobra.Command{sibling})
		require.Contains(t, notice, "azd ai")
		require.Contains(t, notice, "finetune")
		require.Contains(t, notice, "nested.ext")
		require.Contains(t, notice, "Fine-tune models")
	})
}

func TestInjectMetadataChildren(t *testing.T) {
	t.Parallel()

	parent := &cobra.Command{Use: "ai"}
	// Pre-existing sibling cobra child contributed by another extension.
	sibling := &cobra.Command{
		Use:   "finetune",
		Short: "Fine-tune models",
		Annotations: map[string]string{
			extensionNamespaceOwnerAnnotation: "true",
			extensionIDAnnotation:             "nested.ext",
		},
	}
	parent.AddCommand(sibling)

	meta := &extensions.ExtensionCommandMetadata{
		Commands: []extensions.Command{
			{Name: []string{"agent"}, Short: "Manage agents"},
			{Name: []string{"toolboxes"}, Short: "Manage toolboxes"},
			{Name: []string{"finetune"}, Short: "Should not collide"},
			{Name: []string{"hidden"}, Short: "Hidden one", Hidden: true},
		},
	}

	added := injectMetadataChildren(parent, meta)
	require.Len(t, added, 2, "expected only non-colliding, non-hidden metadata commands to be injected")

	names := make([]string, 0, len(added))
	for _, c := range added {
		names = append(names, c.Name())
	}
	require.ElementsMatch(t, []string{"agent", "toolboxes"}, names)

	// Sibling must remain present and untouched.
	found := slices.Contains(parent.Commands(), sibling)
	require.True(t, found, "pre-existing sibling cobra command must remain attached")

	// Removing the synthetic children must restore the original tree.
	parent.RemoveCommand(added...)
	require.Len(t, parent.Commands(), 1)
	require.Equal(t, "finetune", parent.Commands()[0].Name())
}

// TestInjectMetadataChildren_RendersInHelp guards against a regression where
// the synthetic injected children failed to appear in azd's rendered help.
// The renderer captures the children list when called, so this test exercises
// it the same way extensionAction.Run does at runtime.
func TestInjectMetadataChildren_RendersInHelp(t *testing.T) {
	t.Parallel()

	parent := &cobra.Command{Use: "ai", Short: "AI commands"}
	sibling := &cobra.Command{
		Use:   "finetune",
		Short: "Fine-tune models",
		Run:   func(*cobra.Command, []string) {},
		Annotations: map[string]string{
			extensionNamespaceOwnerAnnotation: "true",
		},
	}
	parent.AddCommand(sibling)

	meta := &extensions.ExtensionCommandMetadata{
		Commands: []extensions.Command{
			{Name: []string{"agent"}, Short: "Manage agents"},
			{Name: []string{"toolboxes"}, Short: "Manage toolboxes"},
		},
	}
	added := injectMetadataChildren(parent, meta)
	t.Cleanup(func() { parent.RemoveCommand(added...) })

	help := generateCmdHelp(parent, generateCmdHelpOptions{})
	require.Contains(t, help, "agent", "metadata-injected 'agent' must render in help output")
	require.Contains(t, help, "toolboxes", "metadata-injected 'toolboxes' must render in help output")
	require.Contains(t, help, "finetune", "pre-existing sibling must render in help output")
}
