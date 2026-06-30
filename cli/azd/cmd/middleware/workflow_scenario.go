// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package middleware

import (
	"strings"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/pkg/tool"
)

// workflowScenarios maps root-level azd subcommand names to the scenario id
// used by the tool first-run and tool-update-check middleware. A command
// not listed here is not a recognized "workflow" command for either
// purpose — utility commands (auth, config, env, extension, etc.) are
// intentionally excluded to avoid blocking and to prevent recursive
// subprocess issues when the tool check shells out to commands such as
// `azd extension list` (#8052).
//
// Every entry maps to tool.ScenarioCore today. Non-core scenarios are
// contributed entirely by azd extensions in a later phase — see the
// ownership model documented on tool.ScenarioCore.
var workflowScenarios = map[string]string{
	"init":      tool.ScenarioCore,
	"up":        tool.ScenarioCore,
	"provision": tool.ScenarioCore,
	"deploy":    tool.ScenarioCore,
	"down":      tool.ScenarioCore,
	"publish":   tool.ScenarioCore,
	"build":     tool.ScenarioCore,
	"package":   tool.ScenarioCore,
	"restore":   tool.ScenarioCore,
}

// ScenarioForCommandPath resolves the scenario id for the root-level azd
// subcommand named in commandPath, and reports whether it is a recognized
// workflow command at all. commandPath is expected in the form produced by
// (*cobra.Command).CommandPath(), e.g. "azd up" or "azd ai agent" for an
// azd-extension-bound command (whose own sub-verbs, like "init", are not
// visible to core azd's command tree and so cannot affect resolution here).
func ScenarioForCommandPath(commandPath string) (scenario string, ok bool) {
	parts := strings.Fields(commandPath)
	if len(parts) < 2 {
		return "", false
	}

	scenario, ok = workflowScenarios[parts[1]]
	return scenario, ok
}

// IsWorkflowCommand reports whether descriptor's root-level subcommand is a
// recognized workflow command. Used to gate middleware *attachment* at
// predicate-evaluation time, when only the ActionDescriptor (not the
// per-invocation Options) is available. By the time predicates run, the
// full cobra command tree has already been built (including parent
// linkage), so descriptor.Options.Command.CommandPath() is valid here.
func IsWorkflowCommand(descriptor *actions.ActionDescriptor) bool {
	if descriptor == nil || descriptor.Options == nil || descriptor.Options.Command == nil {
		return false
	}

	_, ok := ScenarioForCommandPath(descriptor.Options.Command.CommandPath())
	return ok
}
