// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/internal/mapper"
	"github.com/azure/azure-dev/cli/azd/pkg/ai"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

type aiModelService struct {
	azdext.UnimplementedAiModelServiceServer
	modelService *ai.ModelService
}

// NewAiModelService creates a new gRPC AiModelService implementation.
func NewAiModelService(modelService *ai.ModelService) azdext.AiModelServiceServer {
	return &aiModelService{
		modelService: modelService,
	}
}

func (s *aiModelService) ListModels(
	ctx context.Context,
	req *azdext.ListModelsRequest,
) (*azdext.ListModelsResponse, error) {
	var location string
	if req.Location != nil {
		location = *req.Location
	}

	var filter *ai.FilterOptions
	if err := mapper.Convert(req.Filter, &filter); err != nil {
		return nil, fmt.Errorf("converting filter: %w", err)
	}

	models, err := s.modelService.ListModels(ctx, req.SubscriptionId, location, filter)
	if err != nil {
		return nil, err
	}

	var protoModels []*azdext.AiModel
	if err := mapper.Convert(models, &protoModels); err != nil {
		return nil, fmt.Errorf("converting models: %w", err)
	}

	return &azdext.ListModelsResponse{
		Models: protoModels,
	}, nil
}

func (s *aiModelService) ListModelVersions(
	ctx context.Context,
	req *azdext.ListModelVersionsRequest,
) (*azdext.ListModelVersionsResponse, error) {
	versions, defaultVersion, err := s.modelService.ListModelVersions(
		ctx, req.SubscriptionId, req.ModelName, req.Location)
	if err != nil {
		return nil, err
	}

	return &azdext.ListModelVersionsResponse{
		Versions:       versions,
		DefaultVersion: defaultVersion,
	}, nil
}

func (s *aiModelService) ListModelSkus(
	ctx context.Context,
	req *azdext.ListModelSkusRequest,
) (*azdext.ListModelSkusResponse, error) {
	skus, err := s.modelService.ListModelSkus(
		ctx, req.SubscriptionId, req.ModelName, req.Location, req.Version)
	if err != nil {
		return nil, err
	}

	return &azdext.ListModelSkusResponse{
		Skus: skus,
	}, nil
}

func (s *aiModelService) GetModelDeployment(
	ctx context.Context,
	req *azdext.GetModelDeploymentRequest,
) (*azdext.GetModelDeploymentResponse, error) {
	deployment, err := s.modelService.GetModelDeployment(
		ctx,
		req.SubscriptionId,
		req.ModelName,
		req.PreferredLocations,
		req.PreferredVersions,
		req.PreferredSkus,
	)
	if err != nil {
		return nil, err
	}

	var response *azdext.GetModelDeploymentResponse
	if err := mapper.Convert(deployment, &response); err != nil {
		return nil, fmt.Errorf("converting deployment: %w", err)
	}

	return response, nil
}

func (s *aiModelService) ListUsages(
	ctx context.Context,
	req *azdext.ListUsagesRequest,
) (*azdext.ListUsagesResponse, error) {
	usages, err := s.modelService.ListUsages(ctx, req.SubscriptionId, req.Location)
	if err != nil {
		return nil, err
	}

	var protoUsages []*azdext.AiUsage
	if err := mapper.Convert(usages, &protoUsages); err != nil {
		return nil, fmt.Errorf("converting usages: %w", err)
	}

	return &azdext.ListUsagesResponse{
		Usages: protoUsages,
	}, nil
}

func (s *aiModelService) ListLocationsWithQuota(
	ctx context.Context,
	req *azdext.ListLocationsWithQuotaRequest,
) (*azdext.ListLocationsWithQuotaResponse, error) {
	var requirements []ai.QuotaRequirement
	if err := mapper.Convert(req.Requirements, &requirements); err != nil {
		return nil, fmt.Errorf("converting requirements: %w", err)
	}

	locations, err := s.modelService.ListLocationsWithQuota(
		ctx, req.SubscriptionId, req.Locations, requirements)
	if err != nil {
		return nil, err
	}

	return &azdext.ListLocationsWithQuotaResponse{
		Locations: locations,
	}, nil
}

func (s *aiModelService) ListSkuLocations(
	ctx context.Context,
	req *azdext.ListSkuLocationsRequest,
) (*azdext.ListSkuLocationsResponse, error) {
	locations, err := s.modelService.ListSkuLocations(
		ctx, req.SubscriptionId, req.Kind, req.SkuName, req.Tier, req.ResourceType)
	if err != nil {
		return nil, err
	}

	return &azdext.ListSkuLocationsResponse{
		Locations: locations,
	}, nil
}

func (s *aiModelService) ResolveModelDeployment(
	ctx context.Context,
	req *azdext.ResolveModelDeploymentRequest,
) (*azdext.ResolveModelDeploymentResponse, error) {
	var filter *ai.FilterOptions
	if err := mapper.Convert(req.Filter, &filter); err != nil {
		return nil, fmt.Errorf("converting filter: %w", err)
	}

	resolved, err := s.modelService.ResolveModelDeployment(
		ctx,
		req.SubscriptionId,
		req.ModelName,
		req.PreferredLocation,
		req.PreferredVersions,
		req.PreferredSkus,
		req.MinCapacity,
		filter,
	)
	if err != nil {
		return nil, err
	}

	var deployResp *azdext.GetModelDeploymentResponse
	if err := mapper.Convert(&resolved.ModelDeployment, &deployResp); err != nil {
		return nil, fmt.Errorf("converting deployment: %w", err)
	}

	return &azdext.ResolveModelDeploymentResponse{
		Deployment:        deployResp,
		QuotaValidated:    resolved.QuotaValidated,
		AvailableCapacity: resolved.AvailableCapacity,
	}, nil
}

func (s *aiModelService) ValidateModelAvailability(
	ctx context.Context,
	req *azdext.ValidateModelAvailabilityRequest,
) (*azdext.ValidateModelAvailabilityResponse, error) {
	var filter *ai.FilterOptions
	if err := mapper.Convert(req.Filter, &filter); err != nil {
		return nil, fmt.Errorf("converting filter: %w", err)
	}

	var maxAlts int
	if req.MaxAlternatives != nil {
		maxAlts = int(*req.MaxAlternatives)
	}

	result, err := s.modelService.ValidateModelAvailability(
		ctx, req.SubscriptionId, req.ModelName, req.Location, filter, maxAlts)
	if err != nil {
		return nil, err
	}

	return &azdext.ValidateModelAvailabilityResponse{
		Available:            result.Available,
		AlternativeLocations: result.AlternativeLocations,
		AlternativeModels:    result.AlternativeModels,
	}, nil
}
