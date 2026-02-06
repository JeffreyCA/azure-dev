// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

func TestBuildAiFindLocationsForModelWithQuotaRequest(t *testing.T) {
	req, err := buildAiFindLocationsForModelWithQuotaRequest(
		"sub-123",
		[]string{"eastus", "westus"},
		&azdext.AiModelSelection{
			Name:    "gpt-4o",
			Version: "0613",
			Sku: &azdext.AiModelSku{
				Name: "Standard",
			},
		},
		[]*azdext.AiUsageRequirement{
			{
				UsageName:        "OpenAI.Standard",
				RequiredCapacity: 10,
			},
			{
				UsageName:        "OpenAI.S0.AccountCount",
				RequiredCapacity: 2,
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "sub-123", req.SubscriptionId)
	require.Equal(t, "gpt-4o", req.ModelName)
	require.Equal(t, []string{"eastus", "westus"}, req.Locations)
	require.Equal(t, []string{"0613"}, req.Versions)
	require.Equal(t, []string{"Standard"}, req.Skus)
	require.Len(t, req.Requirements, 2)
	require.Equal(t, "OpenAI.Standard", req.Requirements[0].UsageName)
	require.Equal(t, int32(10), req.Requirements[0].RequiredCapacity)
	require.Equal(t, "OpenAI.S0.AccountCount", req.Requirements[1].UsageName)
	require.Equal(t, int32(2), req.Requirements[1].RequiredCapacity)
}

func TestBuildAiFindLocationsForModelWithQuotaRequest_RequiresModelSelection(t *testing.T) {
	req, err := buildAiFindLocationsForModelWithQuotaRequest("sub-123", []string{"eastus"}, nil, nil)
	require.Error(t, err)
	require.Nil(t, req)
	require.Contains(t, err.Error(), "model selection is required")
}

func TestBuildAiFindLocationsForModelWithQuotaRequest_RequiresSkuSelection(t *testing.T) {
	req, err := buildAiFindLocationsForModelWithQuotaRequest(
		"sub-123",
		[]string{"eastus"},
		&azdext.AiModelSelection{
			Name: "gpt-4o",
		},
		nil,
	)
	require.Error(t, err)
	require.Nil(t, req)
	require.Contains(t, err.Error(), "model SKU selection is required")
}

func TestSummarizeQuotaError(t *testing.T) {
	require.Equal(t, "quota lookup unavailable (NoRegisteredProviderFound)", summarizeQuotaError(`
	RESPONSE 400: 400 Bad Request
	ERROR CODE: NoRegisteredProviderFound
	`))
	require.Equal(t, "quota lookup unavailable in this location", summarizeQuotaError(""))
}
