// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"fmt"
	"math"

	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

type aiCatalogClient interface {
	ListAiLocations(
		ctx context.Context,
		subscriptionId string,
		allowedLocations []string,
	) ([]string, error)
	ListAiModelCatalog(
		ctx context.Context,
		subscriptionId string,
		filters azapi.AiModelCatalogFilters,
	) ([]azapi.AiModelCatalogItem, error)
	ListAiUsages(
		ctx context.Context,
		subscriptionId string,
		location string,
		namePrefix string,
	) ([]azapi.AiUsageSnapshot, error)
	FindAiLocationsWithQuota(
		ctx context.Context,
		subscriptionId string,
		locations []string,
		requirements []azapi.AiUsageRequirement,
		options *azapi.AiLocationsWithQuotaOptions,
	) (*azapi.AiLocationsWithQuotaResult, error)
}

type aiService struct {
	azdext.UnimplementedAiServiceServer
	aiClient aiCatalogClient
}

func NewAiService(aiClient *azapi.AzureClient) azdext.AiServiceServer {
	return &aiService{
		aiClient: aiClient,
	}
}

func (s *aiService) ListLocations(
	ctx context.Context,
	req *azdext.AiListLocationsRequest,
) (*azdext.AiListLocationsResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	locations, err := s.aiClient.ListAiLocations(ctx, req.GetSubscriptionId(), req.GetAllowedLocations())
	if err != nil {
		return nil, err
	}

	return &azdext.AiListLocationsResponse{
		Locations: locations,
	}, nil
}

func (s *aiService) ListModelCatalog(
	ctx context.Context,
	req *azdext.AiListModelCatalogRequest,
) (*azdext.AiListModelCatalogResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	items, err := s.aiClient.ListAiModelCatalog(ctx, req.GetSubscriptionId(), azapi.AiModelCatalogFilters{
		Locations:    req.GetLocations(),
		Kinds:        req.GetKinds(),
		Formats:      req.GetFormats(),
		Statuses:     req.GetStatuses(),
		Capabilities: req.GetCapabilities(),
	})
	if err != nil {
		return nil, err
	}

	models := make([]*azdext.AiModelCatalogItem, 0, len(items))
	for _, item := range items {
		locations := make([]*azdext.AiModelLocation, 0, len(item.Locations))
		for _, location := range item.Locations {
			versions := make([]*azdext.AiModelVersion, 0, len(location.Versions))
			for _, version := range location.Versions {
				skus := make([]*azdext.AiModelSku, 0, len(version.Skus))
				for _, sku := range version.Skus {
					skus = append(skus, &azdext.AiModelSku{
						Name:            sku.Name,
						UsageName:       sku.UsageName,
						CapacityDefault: sku.CapacityDefault,
						CapacityMinimum: sku.CapacityMinimum,
						CapacityMaximum: sku.CapacityMaximum,
						CapacityStep:    sku.CapacityStep,
					})
				}

				versions = append(versions, &azdext.AiModelVersion{
					Version:          version.Version,
					IsDefaultVersion: version.IsDefaultVersion,
					Kind:             version.Kind,
					Format:           version.Format,
					Status:           version.Status,
					Capabilities:     version.Capabilities,
					Skus:             skus,
				})
			}

			locations = append(locations, &azdext.AiModelLocation{
				Location: location.Location,
				Versions: versions,
			})
		}

		models = append(models, &azdext.AiModelCatalogItem{
			Name:      item.Name,
			Locations: locations,
		})
	}

	return &azdext.AiListModelCatalogResponse{
		Models: models,
	}, nil
}

func (s *aiService) ListUsages(
	ctx context.Context,
	req *azdext.AiListUsagesRequest,
) (*azdext.AiListUsagesResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	usages, err := s.aiClient.ListAiUsages(ctx, req.GetSubscriptionId(), req.GetLocation(), req.GetNamePrefix())
	if err != nil {
		return nil, err
	}

	result := make([]*azdext.AiUsage, 0, len(usages))
	for _, usage := range usages {
		result = append(result, &azdext.AiUsage{
			Name:      usage.Name,
			Current:   usage.Current,
			Limit:     usage.Limit,
			Remaining: usage.Remaining,
			Unit:      usage.Unit,
		})
	}

	return &azdext.AiListUsagesResponse{
		Usages: result,
	}, nil
}

func (s *aiService) FindLocationsWithQuota(
	ctx context.Context,
	req *azdext.AiFindLocationsWithQuotaRequest,
) (*azdext.AiFindLocationsWithQuotaResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	requirements := make([]azapi.AiUsageRequirement, 0, len(req.GetRequirements()))
	for _, requirement := range req.GetRequirements() {
		requirements = append(requirements, azapi.AiUsageRequirement{
			UsageName:        requirement.GetUsageName(),
			RequiredCapacity: normalizeRequiredCapacity(requirement.GetRequiredCapacity()),
		})
	}

	locationsResult, err := s.aiClient.FindAiLocationsWithQuota(
		ctx,
		req.GetSubscriptionId(),
		req.GetLocations(),
		requirements,
		nil,
	)
	if err != nil {
		return nil, err
	}

	results := make([]*azdext.AiLocationQuotaResult, 0, len(locationsResult.Results))
	for _, result := range locationsResult.Results {
		requirementResults := make([]*azdext.AiLocationQuotaUsage, 0, len(result.Requirements))
		for _, requirement := range result.Requirements {
			requirementResults = append(requirementResults, &azdext.AiLocationQuotaUsage{
				UsageName:         requirement.UsageName,
				RequiredCapacity:  requiredCapacityToInt32(requirement.RequiredCapacity),
				AvailableCapacity: requirement.AvailableCapacity,
			})
		}

		results = append(results, &azdext.AiLocationQuotaResult{
			Location:     result.Location,
			Matched:      result.Matched,
			Requirements: requirementResults,
			Error:        result.Error,
		})
	}

	return &azdext.AiFindLocationsWithQuotaResponse{
		MatchedLocations: locationsResult.MatchedLocations,
		Results:          results,
	}, nil
}

func normalizeRequiredCapacity(capacity int32) float64 {
	if capacity <= 0 {
		return 1
	}

	return float64(capacity)
}

func requiredCapacityToInt32(capacity float64) int32 {
	if capacity <= 0 {
		return 0
	}
	if capacity >= math.MaxInt32 {
		return math.MaxInt32
	}

	return int32(math.Ceil(capacity))
}
