// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/azure/azure-dev/cli/azd/test/mocks"
	"github.com/stretchr/testify/require"
)

func Test_GetCognitiveAccount(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := mocks.NewMockContext(context.Background())
		azCli := newAzureClientFromMockContext(mockContext)

		expectedName := "ACCOUNT_NAME"
		mockContext.HttpClient.When(func(request *http.Request) bool {
			return request.Method == http.MethodGet &&
				strings.Contains(request.URL.Path, "/Microsoft.CognitiveServices/accounts/ACCOUNT_NAME")
		}).RespondFn(func(request *http.Request) (*http.Response, error) {
			response := armcognitiveservices.Account{
				Name: to.Ptr(expectedName),
			}

			return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
		})

		cogAccount, err := azCli.GetCognitiveAccount(
			*mockContext.Context,
			"SUBSCRIPTION_ID",
			"RESOURCE_GROUP_ID",
			"ACCOUNT_NAME",
		)
		require.NoError(t, err)
		require.Equal(t, *cogAccount.Name, expectedName)
	})
}

func Test_PurgeCognitiveAccount(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockContext := mocks.NewMockContext(context.Background())
		azCli := newAzureClientFromMockContext(mockContext)
		mockContext.HttpClient.When(func(request *http.Request) bool {
			return request.Method == http.MethodDelete &&
				strings.Contains(request.URL.Path, "/resourceGroups/RESOURCE_GROUP_ID/deletedAccounts/ACCOUNT_NAME")
		}).RespondFn(func(request *http.Request) (*http.Response, error) {
			response := armcognitiveservices.DeletedAccountsClientPurgeResponse{}
			return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
		})

		err := azCli.PurgeCognitiveAccount(
			*mockContext.Context,
			"SUBSCRIPTION_ID",
			"LOCATION",
			"RESOURCE_GROUP_ID",
			"ACCOUNT_NAME",
		)
		require.NoError(t, err)
	})
}

func Test_ParseAiUsageRequirement(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expectedName  string
		expectedCap   float64
		expectErr     bool
		expectedError string
	}{
		{
			name:         "usageOnlyDefaultsCapacity",
			value:        "OpenAI.Standard",
			expectedName: "OpenAI.Standard",
			expectedCap:  1,
		},
		{
			name:         "usageWithCapacity",
			value:        "OpenAI.Standard,10",
			expectedName: "OpenAI.Standard",
			expectedCap:  10,
		},
		{
			name:          "emptyUsage",
			value:         ",10",
			expectErr:     true,
			expectedError: "empty usage name",
		},
		{
			name:          "invalidCapacity",
			value:         "OpenAI.Standard,abc",
			expectErr:     true,
			expectedError: "invalid capacity",
		},
		{
			name:          "invalidFormat",
			value:         "a,b,c",
			expectErr:     true,
			expectedError: "invalid usage name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirement, err := ParseAiUsageRequirement(tt.value)
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedName, requirement.UsageName)
			require.Equal(t, tt.expectedCap, requirement.RequiredCapacity)
		})
	}
}

func Test_FindAiLocationsWithQuota(t *testing.T) {
	t.Run("filters matching locations with default account quota requirement", func(t *testing.T) {
		mockContext := mocks.NewMockContext(context.Background())
		azCli := newAzureClientFromMockContext(mockContext)

		// Resource SKU locations
		mockContext.HttpClient.When(func(request *http.Request) bool {
			return request.Method == http.MethodGet &&
				strings.Contains(request.URL.Path, "/providers/Microsoft.CognitiveServices/skus")
		}).RespondFn(func(request *http.Request) (*http.Response, error) {
			response := armcognitiveservices.ResourceSKUListResult{
				Value: []*armcognitiveservices.ResourceSKU{
					{
						Kind:         to.Ptr("AIServices"),
						Name:         to.Ptr("S0"),
						Tier:         to.Ptr("Standard"),
						ResourceType: to.Ptr("accounts"),
						Locations:    []*string{to.Ptr("eastus"), to.Ptr("westus")},
					},
				},
			}

			return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
		})

		// East US usage values satisfy requirement and account quota gate
		mockContext.HttpClient.When(func(request *http.Request) bool {
			return request.Method == http.MethodGet &&
				strings.Contains(request.URL.Path, "/providers/Microsoft.CognitiveServices/locations/eastus/usages")
		}).RespondFn(func(request *http.Request) (*http.Response, error) {
			response := armcognitiveservices.UsageListResult{
				Value: []*armcognitiveservices.Usage{
					{
						Name: &armcognitiveservices.MetricName{
							Value: to.Ptr("OpenAI.Standard"),
						},
						CurrentValue: to.Ptr[float64](5),
						Limit:        to.Ptr[float64](20),
						Unit:         to.Ptr(armcognitiveservices.UnitTypeCount),
					},
					{
						Name: &armcognitiveservices.MetricName{
							Value: to.Ptr(AiAccountQuotaUsageName),
						},
						CurrentValue: to.Ptr[float64](1),
						Limit:        to.Ptr[float64](5),
						Unit:         to.Ptr(armcognitiveservices.UnitTypeCount),
					},
				},
			}

			return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
		})

		// West US fails quota requirement
		mockContext.HttpClient.When(func(request *http.Request) bool {
			return request.Method == http.MethodGet &&
				strings.Contains(request.URL.Path, "/providers/Microsoft.CognitiveServices/locations/westus/usages")
		}).RespondFn(func(request *http.Request) (*http.Response, error) {
			response := armcognitiveservices.UsageListResult{
				Value: []*armcognitiveservices.Usage{
					{
						Name: &armcognitiveservices.MetricName{
							Value: to.Ptr("OpenAI.Standard"),
						},
						CurrentValue: to.Ptr[float64](12),
						Limit:        to.Ptr[float64](20),
						Unit:         to.Ptr(armcognitiveservices.UnitTypeCount),
					},
					{
						Name: &armcognitiveservices.MetricName{
							Value: to.Ptr(AiAccountQuotaUsageName),
						},
						CurrentValue: to.Ptr[float64](4),
						Limit:        to.Ptr[float64](5),
						Unit:         to.Ptr(armcognitiveservices.UnitTypeCount),
					},
				},
			}

			return mocks.CreateHttpResponseWithBody(request, http.StatusOK, response)
		})

		result, err := azCli.FindAiLocationsWithQuota(
			*mockContext.Context,
			"SUBSCRIPTION_ID",
			nil,
			[]AiUsageRequirement{
				{
					UsageName:        "OpenAI.Standard",
					RequiredCapacity: 10,
				},
			},
			nil,
		)
		require.NoError(t, err)
		require.Equal(t, []string{"eastus"}, result.MatchedLocations)
		require.Len(t, result.Results, 2)
	})
}
