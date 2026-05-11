// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"strings"
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/stretchr/testify/assert"
)

func TestCheckForMatchingExtensionsLogic(t *testing.T) {
	t.Parallel()
	// Test the core logic without needing to mock the extension manager
	// We'll create a simple function that mimics the matching logic

	testExtensions := []*extensions.ExtensionMetadata{
		{
			Id:          "extension1",
			Namespace:   "demo",
			DisplayName: "Demo Extension",
			Description: "Simple demo extension",
		},
		{
			Id:          "extension2",
			Namespace:   "vhvb.demo",
			DisplayName: "VHVB Demo Extension",
			Description: "VHVB namespace demo extension",
		},
		{
			Id:          "extension3",
			Namespace:   "vhvb.demo.advanced",
			DisplayName: "Advanced VHVB Demo",
			Description: "Advanced demo with longer namespace",
		},
		{
			Id:          "extension4",
			Namespace:   "other.namespace",
			DisplayName: "Other Extension",
			Description: "Different namespace pattern",
		},
		// Fixture pair for the broad-leaf + nested-extension scenarios.
		{
			Id:          "agents-leaf",
			Namespace:   "ai",
			DisplayName: "AI Agents (leaf)",
			Description: "Owns the bare `ai` namespace",
		},
		{
			Id:          "finetune",
			Namespace:   "ai.finetuning",
			DisplayName: "AI Fine-tuning",
			Description: "Nested under ai.finetuning",
		},
	}

	// Helper function that mimics checkForMatchingExtensions logic.
	// Mirrors the production behavior: only matches at the deepest non-empty
	// candidate prefix are returned (longest-match-wins). Shorter-prefix
	// matches are dropped when a deeper prefix also has matches, so a more
	// specific extension is always preferred over a broader one.
	checkMatches := func(
		args []string, availableExtensions []*extensions.ExtensionMetadata) []*extensions.ExtensionMetadata {
		if len(args) == 0 {
			return nil
		}

		var matchingExtensions []*extensions.ExtensionMetadata

		for i := 1; i <= len(args); i++ {
			candidateNamespace := strings.Join(args[:i], ".")

			var levelMatches []*extensions.ExtensionMetadata
			for _, ext := range availableExtensions {
				if ext.Namespace == candidateNamespace {
					levelMatches = append(levelMatches, ext)
				}
			}
			if len(levelMatches) > 0 {
				matchingExtensions = levelMatches
			}
		}

		return matchingExtensions
	}

	tests := []struct {
		name            string
		args            []string
		expectedMatches []string // Extension IDs that should match
	}{
		{
			name:            "single word matches single extension",
			args:            []string{"demo"},
			expectedMatches: []string{"extension1"},
		},
		{
			name:            "two words matches nested namespace",
			args:            []string{"vhvb", "demo"},
			expectedMatches: []string{"extension2"},
		},
		{
			name: "three words returns only the deepest match",
			args: []string{"vhvb", "demo", "advanced"},
			// vhvb.demo and vhvb.demo.advanced both have matches, but only
			// the deeper one is returned so the user isn't asked to choose
			// between a broad and a specific extension that overlap.
			expectedMatches: []string{"extension3"},
		},
		{
			name: "multiple words returns only the deepest match",
			args: []string{"vhvb", "demo", "advanced", "extra"},
			// No extension has namespace vhvb.demo.advanced.extra; the
			// deepest match is still vhvb.demo.advanced, so we return only
			// that one and drop the shorter vhvb.demo match.
			expectedMatches: []string{"extension3"},
		},
		{
			name:            "no matches for unknown namespace",
			args:            []string{"unknown", "command"},
			expectedMatches: []string{},
		},
		{
			name:            "empty args returns no matches",
			args:            []string{},
			expectedMatches: []string{},
		},
		{
			name:            "partial namespace without full match",
			args:            []string{"vhvb"},
			expectedMatches: []string{}, // No extension with namespace "vhvb" exists
		},
		{
			name: "broad leaf shown when user typed an unknown subcommand",
			// User has azure.ai.agents (namespace "ai") in the registry and
			// types `azd ai blah` — `blah` matches nothing, but the leaf
			// extension at `ai` is the right suggestion because once
			// installed, the leaf binary can decide whether `blah` is a
			// valid subcommand.
			args:            []string{"ai", "blah"},
			expectedMatches: []string{"agents-leaf"},
		},
		{
			name: "specific match wins over broader leaf",
			// User has azure.ai.agents (namespace "ai") AND azure.ai.finetune
			// (namespace "ai.finetuning"). Typing `azd ai finetuning` exactly
			// matches the more specific extension; the broader leaf must NOT
			// be offered because finetune is the only extension that
			// actually contributes a top-level `finetuning` command.
			args:            []string{"ai", "finetuning"},
			expectedMatches: []string{"finetune"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Execute function
			matches := checkMatches(tt.args, testExtensions)

			// Tighten the assertion: the production helper now returns only
			// the deepest matching prefix, so the matched IDs should equal
			// the expected set exactly (not just be a subset).
			matchedIds := make([]string, 0, len(matches))
			for _, match := range matches {
				matchedIds = append(matchedIds, match.Id)
			}
			assert.ElementsMatch(t, tt.expectedMatches, matchedIds)
		})
	}
}
