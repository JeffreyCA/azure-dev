// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

func TestBuildAiQuotaVersionChoices_DeduplicatesVersionsAndSkus(t *testing.T) {
	model := &azdext.AiModelCatalogItem{
		Name: "gpt-4o",
		Locations: []*azdext.AiModelLocation{
			{
				Location: "eastus",
				Versions: []*azdext.AiModelVersion{
					{
						Version:          "0613",
						IsDefaultVersion: true,
						Kind:             "Chat",
						Format:           "OpenAI",
						Status:           "GA",
						Capabilities:     []string{"Vision", "chat"},
						Skus: []*azdext.AiModelSku{
							{
								Name:            "Standard",
								UsageName:       "OpenAI.Standard",
								CapacityDefault: 1,
								CapacityMinimum: 1,
								CapacityMaximum: 10,
								CapacityStep:    1,
							},
						},
					},
				},
			},
			{
				Location: "westus",
				Versions: []*azdext.AiModelVersion{
					{
						Version:          "0613",
						IsDefaultVersion: false,
						Kind:             "Chat",
						Format:           "OpenAI",
						Status:           "GA",
						Capabilities:     []string{"chat", "Vision"},
						Skus: []*azdext.AiModelSku{
							{
								Name:            "Standard",
								UsageName:       "OpenAI.Standard",
								CapacityDefault: 1,
								CapacityMinimum: 1,
								CapacityMaximum: 10,
								CapacityStep:    1,
							},
						},
					},
				},
			},
		},
	}

	versions := buildAiQuotaVersionChoices(model)
	require.Len(t, versions, 1)

	version := versions[0]
	require.Equal(t, "0613", version.Version)
	require.True(t, version.IsDefaultVersion)
	require.Len(t, version.Skus, 1)

	sku := version.Skus[0]
	require.Equal(t, "Standard", sku.Sku.GetName())
	require.Equal(t, "OpenAI.Standard", sku.Sku.GetUsageName())
	require.Equal(t, []string{"eastus", "westus"}, sku.Locations)
}
