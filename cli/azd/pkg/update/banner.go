// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package update

import (
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/output"
)

const (
	// stableReleaseNotesURL is the release notes URL template for stable versions.
	stableReleaseNotesURL = "https://github.com/Azure/azure-dev/releases/tag/azure-dev-cli_%s"
	// dailyReleaseNotesURL points to the main branch commits for daily builds.
	dailyReleaseNotesURL = "https://github.com/Azure/azure-dev/commits/main/"
)

// BannerParams holds the data needed to render an update banner.
type BannerParams struct {
	CurrentVersion string
	LatestVersion  string
	Channel        Channel
	UpdateHint     UpdateHint
}

type updateHintKind int

const (
	updateHintUnknown updateHintKind = iota
	updateHintRun
	updateHintVisit
)

// UpdateHint describes how a user should update azd.
type UpdateHint struct {
	kind  updateHintKind
	value string
}

// RunUpdateHint returns an update hint that renders a shell command.
func RunUpdateHint(command string) UpdateHint {
	return UpdateHint{
		kind:  updateHintRun,
		value: command,
	}
}

// VisitUpdateHint returns an update hint that renders a documentation URL.
func VisitUpdateHint(url string) UpdateHint {
	return UpdateHint{
		kind:  updateHintVisit,
		value: url,
	}
}

// RenderUpdateBanner returns the formatted update notification string.
// The returned string includes
// color/formatting escape codes and is ready to be printed to stderr.
func RenderUpdateBanner(p BannerParams) string {
	var sb strings.Builder
	releaseNotes := p.releaseNotesLink()
	sb.WriteString(output.WithWarningFormat("Update available: ") + p.versionDisplay())
	sb.WriteString(fmt.Sprintf(" (%s)", output.WithHyperlink(releaseNotes.url, releaseNotes.label)))

	if updateHint := formatUpdateHint(p.UpdateHint); updateHint != "" {
		sb.WriteString("\n")
		sb.WriteString(updateHint)
	}

	return sb.String()
}

func (p BannerParams) versionDisplay() string {
	if p.Channel == ChannelDaily {
		return p.LatestVersion
	}

	return fmt.Sprintf("%s -> %s", p.CurrentVersion, p.LatestVersion)
}

type releaseNotesLink struct {
	label string
	url   string
}

func (p BannerParams) releaseNotesLink() releaseNotesLink {
	if p.Channel == ChannelDaily {
		return releaseNotesLink{
			label: "Recent Changes",
			url:   dailyReleaseNotesURL,
		}
	}

	// Use the raw semver version (strip any "(build NNN)" suffix) for the tag.
	version := p.LatestVersion
	if idx := strings.Index(version, " ("); idx != -1 {
		version = version[:idx]
	}

	return releaseNotesLink{
		label: "Release Notes",
		url:   fmt.Sprintf(stableReleaseNotesURL, version),
	}
}

// formatUpdateHint renders the update instruction for new-style banners.
func formatUpdateHint(updateHint UpdateHint) string {
	switch updateHint.kind {
	case updateHintRun:
		return fmt.Sprintf("To update, run `%s`", output.WithHighLightFormat("%s", updateHint.value))
	case updateHintVisit:
		return fmt.Sprintf(
			"To update, visit %s",
			output.WithHyperlink(updateHint.value, updateHint.value),
		)
	default:
		return ""
	}
}
