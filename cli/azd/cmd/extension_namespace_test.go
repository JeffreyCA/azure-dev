// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/stretchr/testify/require"
)

func TestNamespacesConflict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ns1      string
		ns2      string
		conflict bool
		msg      string
	}{
		// Exact match — the only install-time block today.
		{
			name:     "same namespace",
			ns1:      "ai",
			ns2:      "ai",
			conflict: true,
			msg:      "the same namespace",
		},
		{
			name:     "case insensitive - same namespace",
			ns1:      "AI",
			ns2:      "ai",
			conflict: true,
			msg:      "the same namespace",
		},
		{
			name:     "case insensitive - mixed case exact match",
			ns1:      "Ai",
			ns2:      "AI",
			conflict: true,
			msg:      "the same namespace",
		},
		// Prefix-overlapping namespaces are NOT a conflict — bindExtension
		// merges them into a single hybrid cobra node.
		{name: "ns1 is prefix of ns2", ns1: "ai", ns2: "ai.agent", conflict: false},
		{name: "ns2 is prefix of ns1", ns1: "ai.agent", ns2: "ai", conflict: false},
		{name: "deeply nested overlap", ns1: "ai.models", ns2: "ai.models.finetune", conflict: false},
		{name: "case insensitive prefix overlap", ns1: "AI", ns2: "ai.agent", conflict: false},
		// Disjoint namespaces.
		{name: "no conflict - different namespaces", ns1: "ai", ns2: "demo", conflict: false},
		{name: "no conflict - sibling namespaces", ns1: "ai.agent", ns2: "ai.finetune", conflict: false},
		{name: "no conflict - partial match not at boundary", ns1: "ai", ns2: "air", conflict: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conflict, msg := namespacesConflict(tt.ns1, tt.ns2)
			require.Equal(t, tt.conflict, conflict)
			if tt.conflict {
				require.Equal(t, tt.msg, msg)
			}
		})
	}
}

func TestCheckNamespaceCommandOverlap(t *testing.T) {
	t.Parallel()

	t.Run("empty namespace allowed", func(t *testing.T) {
		t.Parallel()
		err := checkNamespaceCommandOverlap("new.ext", "", map[string]*extensions.Extension{
			"existing.ext": {Id: "existing.ext", Namespace: "ai"},
		})
		require.NoError(t, err)
	})

	t.Run("disjoint namespaces allowed", func(t *testing.T) {
		t.Parallel()
		installed := map[string]*extensions.Extension{
			"existing.ext": {Id: "existing.ext", Namespace: "ai"},
		}
		require.NoError(t, checkNamespaceCommandOverlap("new.ext", "deploy", installed))
	})

	t.Run("prefix-overlapping namespaces allowed", func(t *testing.T) {
		t.Parallel()
		// Coexistence is enabled: bindExtension produces a single hybrid
		// `ai` node, cobra resolves `ai finetune` to the nested extension's
		// child subcommand, and the merged help renderer surfaces both
		// extensions' contributions. No install-time block.
		installed := map[string]*extensions.Extension{
			"leaf.ext": {Id: "leaf.ext", Namespace: "ai"},
		}
		require.NoError(t, checkNamespaceCommandOverlap("nested.ext", "ai.finetune", installed))
		require.NoError(t, checkNamespaceCommandOverlap("nested.ext", "ai.models.eval", installed))

		installed = map[string]*extensions.Extension{
			"nested.ext": {Id: "nested.ext", Namespace: "ai.finetune"},
		}
		require.NoError(t, checkNamespaceCommandOverlap("leaf.ext", "ai", installed))
	})

	t.Run("exact match always blocked", func(t *testing.T) {
		t.Parallel()
		installed := map[string]*extensions.Extension{
			"existing.ext": {Id: "existing.ext", Namespace: "ai"},
		}
		err := checkNamespaceCommandOverlap("new.ext", "ai", installed)
		require.Error(t, err)

		var errWithSuggestion *internal.ErrorWithSuggestion
		require.ErrorAs(t, err, &errWithSuggestion)
		require.Contains(t, errWithSuggestion.Err.Error(), "namespace 'ai' conflicts")
		require.Contains(t, errWithSuggestion.Err.Error(), "existing.ext")
	})

	t.Run("exact match is case insensitive", func(t *testing.T) {
		t.Parallel()
		installed := map[string]*extensions.Extension{
			"existing.ext": {Id: "existing.ext", Namespace: "AI"},
		}
		err := checkNamespaceCommandOverlap("new.ext", "ai", installed)
		require.Error(t, err)
	})

	t.Run("ignores self for upgrades", func(t *testing.T) {
		t.Parallel()
		installed := map[string]*extensions.Extension{
			"my.ext": {Id: "my.ext", Namespace: "ai"},
		}
		require.NoError(t, checkNamespaceCommandOverlap("my.ext", "ai", installed))
	})

	t.Run("ignores installed extensions with no namespace", func(t *testing.T) {
		t.Parallel()
		installed := map[string]*extensions.Extension{
			"no-ns.ext": {Id: "no-ns.ext", Namespace: ""},
		}
		require.NoError(t, checkNamespaceCommandOverlap("new.ext", "ai", installed))
	})
}
