// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

func TestParseAiUsageRequirementArg(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedName    string
		expectedCap     int32
		expectErr       bool
		expectedErrText string
	}{
		{
			name:         "usageOnlyDefaultsCapacity",
			input:        "OpenAI.Standard",
			expectedName: "OpenAI.Standard",
			expectedCap:  1,
		},
		{
			name:         "usageAndCapacity",
			input:        "OpenAI.Standard,10",
			expectedName: "OpenAI.Standard",
			expectedCap:  10,
		},
		{
			name:            "emptyUsage",
			input:           ",10",
			expectErr:       true,
			expectedErrText: "usage name is required",
		},
		{
			name:            "invalidCapacity",
			input:           "OpenAI.Standard,abc",
			expectErr:       true,
			expectedErrText: "capacity must be an integer",
		},
		{
			name:            "zeroCapacity",
			input:           "OpenAI.Standard,0",
			expectErr:       true,
			expectedErrText: "capacity must be greater than 0",
		},
		{
			name:            "tooManyParts",
			input:           "a,b,c",
			expectErr:       true,
			expectedErrText: "invalid requirement format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirement, err := parseAiUsageRequirementArg(tt.input)
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrText)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedName, requirement.UsageName)
			require.Equal(t, tt.expectedCap, requirement.RequiredCapacity)
		})
	}
}

func TestBuildAiFindLocationsWithQuotaRequest(t *testing.T) {
	req, err := buildAiFindLocationsWithQuotaRequest(
		"sub-123",
		[]string{"eastus", "westus"},
		[]string{"OpenAI.Standard,10", "OpenAI.S0.AccountCount,2"},
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

func TestBuildPromptAiLocationRequest(t *testing.T) {
	scope := &azdext.AzureScope{
		SubscriptionId: "sub-123",
		Location:       "eastus",
	}

	req, err := buildPromptAiLocationRequest(scope, []string{"eastus", "westus"}, []string{"OpenAI.Standard,10"})
	require.NoError(t, err)
	require.NotNil(t, req.AzureContext)
	require.Equal(t, scope, req.AzureContext.Scope)
	require.Equal(t, []string{"eastus", "westus"}, req.AllowedLocations)
	require.Len(t, req.Requirements, 1)
	require.Equal(t, "OpenAI.Standard", req.Requirements[0].UsageName)
	require.Equal(t, int32(10), req.Requirements[0].RequiredCapacity)
}
