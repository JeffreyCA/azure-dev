// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderUpdateBanner(t *testing.T) {
	t.Parallel()

	params := BannerParams{
		CurrentVersion: "1.11.0",
		LatestVersion:  "1.13.1",
		Channel:        ChannelStable,
		UpdateHint:     RunUpdateHint("azd update"),
	}

	result := RenderUpdateBanner(params)
	require.NotEmpty(t, result)

	for _, s := range []string{
		"Update available:",
		"1.11.0 -> 1.13.1",
		"To update, run:",
		"`azd update`",
		// WithHyperlink falls back to plain URL in non-terminal test environments.
		"github.com/Azure/azure-dev/releases/tag/azure-dev-cli_1.13.1",
	} {
		assert.Contains(t, result, s, "expected banner to contain %q", s)
	}

	assert.NotContains(t, result, "WARNING:")
	assert.NotContains(t, result, "out of date")
}

func TestRenderUpdateBanner_PlatformCommand(t *testing.T) {
	t.Parallel()

	params := BannerParams{
		CurrentVersion: "1.11.0",
		LatestVersion:  "1.13.1",
		Channel:        ChannelStable,
		UpdateHint:     RunUpdateHint("curl -fsSL https://aka.ms/install-azd.sh | bash"),
	}

	t.Run("extracts_first_command", func(t *testing.T) {
		t.Parallel()
		result := RenderUpdateBanner(params)
		assert.Contains(t, result, "`curl -fsSL https://aka.ms/install-azd.sh | bash`")
		assert.NotContains(t, result, "Extra instructions")
	})

	t.Run("handles_visit_url", func(t *testing.T) {
		t.Parallel()
		visitParams := BannerParams{
			CurrentVersion: "1.11.0",
			LatestVersion:  "1.13.1",
			Channel:        ChannelStable,
			UpdateHint:     VisitUpdateHint("https://aka.ms/azd/upgrade/linux"),
		}
		result := RenderUpdateBanner(visitParams)
		assert.Contains(t, result, "To update, visit https://aka.ms/azd/upgrade/linux")
	})
}

func TestRenderUpdateBanner_DailyChannel(t *testing.T) {
	t.Parallel()

	params := BannerParams{
		CurrentVersion: "1.11.0",
		LatestVersion:  "1.13.1 (build 12345)",
		Channel:        ChannelDaily,
		UpdateHint:     RunUpdateHint("azd update"),
	}

	result := RenderUpdateBanner(params)
	assert.Contains(t, result, "Update available:")
	assert.Contains(t, result, "1.13.1 (build 12345)")
	assert.Contains(t, result, "github.com/Azure/azure-dev/commits/main/")
	assert.NotContains(t, result, "1.11.0 ->")
}

func TestFormatUpdateHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    UpdateHint
		contains []string
	}{
		{
			name:     "simple_command",
			input:    RunUpdateHint("azd update"),
			contains: []string{"To update, run:", "`azd update`"},
		},
		{
			name:     "shell_command",
			input:    RunUpdateHint("curl -fsSL https://aka.ms/install-azd.sh | bash"),
			contains: []string{"To update, run:", "`curl -fsSL https://aka.ms/install-azd.sh | bash`"},
		},
		{
			name:     "visit_url",
			input:    VisitUpdateHint("https://aka.ms/azd/upgrade/linux"),
			contains: []string{"To update, visit https://aka.ms/azd/upgrade/linux"},
		},
		{
			name:     "empty_hint",
			input:    UpdateHint{},
			contains: []string{""},
		},
		{
			name:     "winget_command",
			input:    RunUpdateHint("winget upgrade Microsoft.Azd"),
			contains: []string{"To update, run:", "`winget upgrade Microsoft.Azd`"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatUpdateHint(tt.input)
			for _, s := range tt.contains {
				assert.Contains(t, got, s, "expected hint to contain %q", s)
			}
		})
	}
}

func TestReleaseNotesLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   BannerParams
		expected releaseNotesLink
	}{
		{
			name: "stable_links_to_release_tag",
			params: BannerParams{
				LatestVersion: "1.13.1",
				Channel:       ChannelStable,
			},
			expected: releaseNotesLink{
				label: "Release Notes",
				url:   "https://github.com/Azure/azure-dev/releases/tag/azure-dev-cli_1.13.1",
			},
		},
		{
			name: "stable_strips_build_suffix",
			params: BannerParams{
				LatestVersion: "1.13.1 (build 999)",
				Channel:       ChannelStable,
			},
			expected: releaseNotesLink{
				label: "Release Notes",
				url:   "https://github.com/Azure/azure-dev/releases/tag/azure-dev-cli_1.13.1",
			},
		},
		{
			name: "daily_links_to_commits",
			params: BannerParams{
				LatestVersion: "1.14.0-daily.12345",
				Channel:       ChannelDaily,
			},
			expected: releaseNotesLink{
				label: "Recent Changes",
				url:   "https://github.com/Azure/azure-dev/commits/main/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.params.releaseNotesLink()
			assert.Equal(t, tt.expected, got)
		})
	}
}
