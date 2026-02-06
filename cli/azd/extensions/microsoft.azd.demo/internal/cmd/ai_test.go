// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

func TestBuildAiFindLocationsWithQuotaRequest(t *testing.T) {
	req, err := buildAiFindLocationsWithQuotaRequest(
		"sub-123",
		[]string{"eastus", "westus"},
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
	require.Equal(t, []string{"eastus", "westus"}, req.Locations)
	require.Len(t, req.Requirements, 2)
	require.Equal(t, "OpenAI.Standard", req.Requirements[0].UsageName)
	require.Equal(t, int32(10), req.Requirements[0].RequiredCapacity)
	require.Equal(t, "OpenAI.S0.AccountCount", req.Requirements[1].UsageName)
	require.Equal(t, int32(2), req.Requirements[1].RequiredCapacity)
}

func TestBuildAiFindLocationsWithQuotaRequest_RequiresAtLeastOneRequirement(t *testing.T) {
	req, err := buildAiFindLocationsWithQuotaRequest("sub-123", []string{"eastus"}, nil)
	require.Error(t, err)
	require.Nil(t, req)
	require.Contains(t, err.Error(), "at least one usage requirement must be provided")
}

func TestSummarizeQuotaError(t *testing.T) {
	require.Equal(t, "quota lookup unavailable (NoRegisteredProviderFound)", summarizeQuotaError(`
	RESPONSE 400: 400 Bad Request
	ERROR CODE: NoRegisteredProviderFound
	`))
	require.Equal(t, "quota lookup unavailable in this location", summarizeQuotaError(""))
}
