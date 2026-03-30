// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import "slices"

// supportedAgentInitLocations mirrors the current Azure AI Foundry hosted agents
// region availability. Keep this list in sync with:
// https://learn.microsoft.com/azure/foundry/agents/concepts/hosted-agents#region-availability
var supportedAgentInitLocations = []string{
	"australiaeast",
	"brazilsouth",
	"canadacentral",
	"canadaeast",
	"eastus",
	"eastus2",
	"francecentral",
	"germanywestcentral",
	"italynorth",
	"japaneast",
	"koreacentral",
	"northcentralus",
	"norwayeast",
	"polandcentral",
	"southafricanorth",
	"southcentralus",
	"southeastasia",
	"southindia",
	"spaincentral",
	"swedencentral",
	"switzerlandnorth",
	"uaenorth",
	"uksouth",
	"westus",
	"westus3",
}

func supportedLocationNamesForInit() []string {
	return slices.Clone(supportedAgentInitLocations)
}
