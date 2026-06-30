// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package tool

// ScenarioCore is the scenario id for azd's standard, non-extension
// workflow commands (init, up, provision, deploy, ...).
//
// Ownership model: a built-in [ToolDefinition]'s own Scenarios map only
// ever contains the ScenarioCore key. Core azd never hardcodes knowledge
// of any other scenario (e.g. an AI/Foundry scenario) — every non-core
// scenario, including which existing built-in tools apply to it and at
// what priority, is declared entirely by the azd extension that owns
// that scenario. Because the two sources never write to the same
// scenario key, there is no conflict to resolve between them.
const ScenarioCore = "core"
