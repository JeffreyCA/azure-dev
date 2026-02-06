// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

type mockAiCatalogClient struct {
	listLocationsFn func(
		ctx context.Context,
		subscriptionId string,
		allowedLocations []string,
	) ([]string, error)
	listModelCatalogFn func(
		ctx context.Context,
		subscriptionId string,
		filters azapi.AiModelCatalogFilters,
	) ([]azapi.AiModelCatalogItem, error)
	listUsagesFn func(
		ctx context.Context,
		subscriptionId string,
		location string,
		namePrefix string,
	) ([]azapi.AiUsageSnapshot, error)
	findLocationsWithQuotaFn func(
		ctx context.Context,
		subscriptionId string,
		locations []string,
		requirements []azapi.AiUsageRequirement,
		options *azapi.AiLocationsWithQuotaOptions,
	) (*azapi.AiLocationsWithQuotaResult, error)
}

func (m *mockAiCatalogClient) ListAiLocations(
	ctx context.Context,
	subscriptionId string,
	allowedLocations []string,
) ([]string, error) {
	return m.listLocationsFn(ctx, subscriptionId, allowedLocations)
}

func (m *mockAiCatalogClient) ListAiModelCatalog(
	ctx context.Context,
	subscriptionId string,
	filters azapi.AiModelCatalogFilters,
) ([]azapi.AiModelCatalogItem, error) {
	return m.listModelCatalogFn(ctx, subscriptionId, filters)
}

func (m *mockAiCatalogClient) ListAiUsages(
	ctx context.Context,
	subscriptionId string,
	location string,
	namePrefix string,
) ([]azapi.AiUsageSnapshot, error) {
	return m.listUsagesFn(ctx, subscriptionId, location, namePrefix)
}

func (m *mockAiCatalogClient) FindAiLocationsWithQuota(
	ctx context.Context,
	subscriptionId string,
	locations []string,
	requirements []azapi.AiUsageRequirement,
	options *azapi.AiLocationsWithQuotaOptions,
) (*azapi.AiLocationsWithQuotaResult, error) {
	return m.findLocationsWithQuotaFn(ctx, subscriptionId, locations, requirements, options)
}

func TestAiService_ListLocations(t *testing.T) {
	service := &aiService{
		aiClient: &mockAiCatalogClient{
			listLocationsFn: func(
				_ context.Context,
				subscriptionId string,
				allowedLocations []string,
			) ([]string, error) {
				require.Equal(t, "sub-123", subscriptionId)
				require.Equal(t, []string{"eastus"}, allowedLocations)
				return []string{"eastus", "westus"}, nil
			},
			listModelCatalogFn: func(context.Context, string, azapi.AiModelCatalogFilters) ([]azapi.AiModelCatalogItem, error) {
				return nil, nil
			},
			listUsagesFn: func(context.Context, string, string, string) ([]azapi.AiUsageSnapshot, error) {
				return nil, nil
			},
			findLocationsWithQuotaFn: func(
				context.Context,
				string,
				[]string,
				[]azapi.AiUsageRequirement,
				*azapi.AiLocationsWithQuotaOptions,
			) (*azapi.AiLocationsWithQuotaResult, error) {
				return nil, nil
			},
		},
	}

	resp, err := service.ListLocations(context.Background(), &azdext.AiListLocationsRequest{
		SubscriptionId:   "sub-123",
		AllowedLocations: []string{"eastus"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"eastus", "westus"}, resp.Locations)
}

func TestAiService_ListUsages(t *testing.T) {
	service := &aiService{
		aiClient: &mockAiCatalogClient{
			listLocationsFn: func(context.Context, string, []string) ([]string, error) {
				return nil, nil
			},
			listModelCatalogFn: func(context.Context, string, azapi.AiModelCatalogFilters) ([]azapi.AiModelCatalogItem, error) {
				return nil, nil
			},
			listUsagesFn: func(_ context.Context, subscriptionId, location, namePrefix string) ([]azapi.AiUsageSnapshot, error) {
				require.Equal(t, "sub-123", subscriptionId)
				require.Equal(t, "eastus", location)
				require.Equal(t, "OpenAI", namePrefix)
				return []azapi.AiUsageSnapshot{
					{Name: "OpenAI.Standard", Current: 1, Limit: 10, Remaining: 9, Unit: "Count"},
				}, nil
			},
			findLocationsWithQuotaFn: func(
				context.Context,
				string,
				[]string,
				[]azapi.AiUsageRequirement,
				*azapi.AiLocationsWithQuotaOptions,
			) (*azapi.AiLocationsWithQuotaResult, error) {
				return nil, nil
			},
		},
	}

	resp, err := service.ListUsages(context.Background(), &azdext.AiListUsagesRequest{
		SubscriptionId: "sub-123",
		Location:       "eastus",
		NamePrefix:     "OpenAI",
	})
	require.NoError(t, err)
	require.Len(t, resp.Usages, 1)
	require.Equal(t, "OpenAI.Standard", resp.Usages[0].Name)
	require.Equal(t, float64(9), resp.Usages[0].Remaining)
}

func TestAiService_ListModelCatalog(t *testing.T) {
	service := &aiService{
		aiClient: &mockAiCatalogClient{
			listLocationsFn: func(context.Context, string, []string) ([]string, error) {
				return nil, nil
			},
			listModelCatalogFn: func(
				_ context.Context,
				subscriptionId string,
				filters azapi.AiModelCatalogFilters,
			) ([]azapi.AiModelCatalogItem, error) {
				require.Equal(t, "sub-123", subscriptionId)
				require.Equal(t, []string{"eastus"}, filters.Locations)
				require.Equal(t, []string{"Chat"}, filters.Kinds)

				return []azapi.AiModelCatalogItem{
					{
						Name: "gpt-4o",
						Locations: []azapi.AiModelLocation{
							{
								Location: "eastus",
								Versions: []azapi.AiModelVersion{
									{
										Version:          "2024-05-13",
										IsDefaultVersion: true,
										Kind:             "Chat",
										Format:           "OpenAI",
										Status:           "GenerallyAvailable",
										Capabilities:     []string{"ChatCompletion"},
										Skus: []azapi.AiModelSku{
											{
												Name:            "Standard",
												UsageName:       "OpenAI.Standard",
												CapacityDefault: 10,
												CapacityMinimum: 1,
												CapacityMaximum: 100,
												CapacityStep:    1,
											},
										},
									},
								},
							},
						},
					},
				}, nil
			},
			listUsagesFn: func(context.Context, string, string, string) ([]azapi.AiUsageSnapshot, error) {
				return nil, nil
			},
			findLocationsWithQuotaFn: func(
				context.Context,
				string,
				[]string,
				[]azapi.AiUsageRequirement,
				*azapi.AiLocationsWithQuotaOptions,
			) (*azapi.AiLocationsWithQuotaResult, error) {
				return nil, nil
			},
		},
	}

	resp, err := service.ListModelCatalog(context.Background(), &azdext.AiListModelCatalogRequest{
		SubscriptionId: "sub-123",
		Locations:      []string{"eastus"},
		Kinds:          []string{"Chat"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Models, 1)
	require.Equal(t, "gpt-4o", resp.Models[0].Name)
	require.Len(t, resp.Models[0].Locations, 1)
	require.Equal(t, "eastus", resp.Models[0].Locations[0].Location)
	require.Len(t, resp.Models[0].Locations[0].Versions, 1)
	require.True(t, resp.Models[0].Locations[0].Versions[0].IsDefaultVersion)
	require.Len(t, resp.Models[0].Locations[0].Versions[0].Skus, 1)
	require.Equal(t, "OpenAI.Standard", resp.Models[0].Locations[0].Versions[0].Skus[0].UsageName)
}

func TestAiService_FindLocationsWithQuota(t *testing.T) {
	service := &aiService{
		aiClient: &mockAiCatalogClient{
			listLocationsFn: func(context.Context, string, []string) ([]string, error) {
				return nil, nil
			},
			listModelCatalogFn: func(context.Context, string, azapi.AiModelCatalogFilters) ([]azapi.AiModelCatalogItem, error) {
				return nil, nil
			},
			listUsagesFn: func(context.Context, string, string, string) ([]azapi.AiUsageSnapshot, error) {
				return nil, nil
			},
			findLocationsWithQuotaFn: func(
				_ context.Context,
				subscriptionId string,
				locations []string,
				requirements []azapi.AiUsageRequirement,
				options *azapi.AiLocationsWithQuotaOptions,
			) (*azapi.AiLocationsWithQuotaResult, error) {
				require.Equal(t, "sub-123", subscriptionId)
				require.Equal(t, []string{"eastus", "westus"}, locations)
				require.Nil(t, options)
				require.Len(t, requirements, 2)
				require.Equal(t, "OpenAI.Standard", requirements[0].UsageName)
				require.Equal(t, float64(1), requirements[0].RequiredCapacity) // defaults from zero
				require.Equal(t, float64(10), requirements[1].RequiredCapacity)

				return &azapi.AiLocationsWithQuotaResult{
					MatchedLocations: []string{"eastus"},
					Results: []azapi.AiLocationQuotaResult{
						{
							Location: "eastus",
							Matched:  true,
							Requirements: []azapi.AiLocationQuotaUsage{
								{
									UsageName:         "OpenAI.Standard",
									RequiredCapacity:  10,
									AvailableCapacity: 20,
								},
							},
						},
					},
				}, nil
			},
		},
	}

	resp, err := service.FindLocationsWithQuota(context.Background(), &azdext.AiFindLocationsWithQuotaRequest{
		SubscriptionId: "sub-123",
		Locations:      []string{"eastus", "westus"},
		Requirements: []*azdext.AiUsageRequirement{
			{
				UsageName:        "OpenAI.Standard",
				RequiredCapacity: 0,
			},
			{
				UsageName:        "OpenAI.Premium",
				RequiredCapacity: 10,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"eastus"}, resp.MatchedLocations)
	require.Len(t, resp.Results, 1)
	require.Len(t, resp.Results[0].Requirements, 1)
	require.Equal(t, int32(10), resp.Results[0].Requirements[0].RequiredCapacity)
	require.Equal(t, float64(20), resp.Results[0].Requirements[0].AvailableCapacity)
}
