// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/ai"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

type aiModelService struct {
	azdext.UnimplementedAiModelServiceServer
	modelService *ai.AiModelService
}

// NewAiModelService creates a new AI model gRPC service.
func NewAiModelService(
	modelService *ai.AiModelService,
) azdext.AiModelServiceServer {
	return &aiModelService{
		modelService: modelService,
	}
}

// --- Primitives ---

func (s *aiModelService) ListModels(
	ctx context.Context, req *azdext.ListModelsRequest,
) (*azdext.ListModelsResponse, error) {
	subscriptionId, _ := extractScope(req.AzureContext)

	var filterOpts *ai.FilterOptions
	if req.Filter != nil {
		filterOpts = protoToFilterOptions(req.Filter)
	}
	locations := effectiveLocations(filterOpts, req.AzureContext)

	models, err := s.modelService.ListModels(ctx, subscriptionId, locations)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	if filterOpts != nil {
		models = ai.FilterModels(models, filterOpts)
	}

	protoModels := make([]*azdext.AiModel, len(models))
	for i, m := range models {
		protoModels[i] = aiModelToProto(&m)
	}

	return &azdext.ListModelsResponse{Models: protoModels}, nil
}

func (s *aiModelService) ListModelVersions(
	ctx context.Context, req *azdext.ListModelVersionsRequest,
) (*azdext.ListModelVersionsResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)
	location := pickLocation(req.Location, scopeLocation)
	if location == "" {
		return nil, fmt.Errorf("location is required for listing model versions")
	}

	versions, defaultVersion, err := s.modelService.ListModelVersions(
		ctx, subscriptionId, req.ModelName, location)
	if err != nil {
		return nil, fmt.Errorf("listing model versions: %w", err)
	}

	protoVersions := make([]*azdext.AiModelVersion, len(versions))
	for i, v := range versions {
		protoVersions[i] = aiModelVersionToProto(&v)
	}

	return &azdext.ListModelVersionsResponse{
		Versions:       protoVersions,
		DefaultVersion: defaultVersion,
	}, nil
}

func (s *aiModelService) ListModelSkus(
	ctx context.Context, req *azdext.ListModelSkusRequest,
) (*azdext.ListModelSkusResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)
	location := pickLocation(req.Location, scopeLocation)
	if location == "" {
		return nil, fmt.Errorf("location is required for listing model SKUs")
	}

	skus, err := s.modelService.ListModelSkus(
		ctx, subscriptionId, req.ModelName, location, req.Version)
	if err != nil {
		return nil, fmt.Errorf("listing model SKUs: %w", err)
	}

	protoSkus := make([]*azdext.AiModelSku, len(skus))
	for i, sku := range skus {
		protoSkus[i] = aiModelSkuToProto(&sku)
	}

	return &azdext.ListModelSkusResponse{Skus: protoSkus}, nil
}

func (s *aiModelService) ResolveModelDeployments(
	ctx context.Context, req *azdext.ResolveModelDeploymentsRequest,
) (*azdext.ResolveModelDeploymentsResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)

	options := protoToDeploymentOptions(req.Options)
	if options == nil {
		options = &ai.DeploymentOptions{}
	}
	if len(options.Locations) == 0 && scopeLocation != "" {
		options.Locations = []string{scopeLocation}
	}

	var deployments []ai.AiModelDeployment
	var err error

	if req.Quota != nil {
		quotaOpts := protoToQuotaCheckOptions(req.Quota)
		deployments, err = s.modelService.ResolveModelDeploymentsWithQuota(
			ctx, subscriptionId, req.ModelName, options, quotaOpts)
	} else {
		deployments, err = s.modelService.ResolveModelDeployments(
			ctx, subscriptionId, req.ModelName, options)
	}
	if err != nil {
		return nil, fmt.Errorf("resolving model deployments: %w", err)
	}

	protoDeployments := make([]*azdext.AiModelDeployment, len(deployments))
	for i := range deployments {
		protoDeployments[i] = aiModelDeploymentToProto(&deployments[i])
	}

	return &azdext.ResolveModelDeploymentsResponse{
		Deployments: protoDeployments,
	}, nil
}

func (s *aiModelService) ListUsages(
	ctx context.Context, req *azdext.ListUsagesRequest,
) (*azdext.ListUsagesResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)
	location := pickLocation(req.Location, scopeLocation)
	if location == "" {
		return nil, fmt.Errorf("location is required for listing usages")
	}

	usages, err := s.modelService.ListUsages(ctx, subscriptionId, location)
	if err != nil {
		return nil, fmt.Errorf("listing usages: %w", err)
	}

	protoUsages := make([]*azdext.AiModelUsage, len(usages))
	for i, u := range usages {
		protoUsages[i] = &azdext.AiModelUsage{
			Name:         u.Name,
			CurrentValue: u.CurrentValue,
			Limit:        u.Limit,
		}
	}

	return &azdext.ListUsagesResponse{Usages: protoUsages}, nil
}

func (s *aiModelService) ListLocationsWithQuota(
	ctx context.Context, req *azdext.ListLocationsWithQuotaRequest,
) (*azdext.ListLocationsWithQuotaResponse, error) {
	subscriptionId, _ := extractScope(req.AzureContext)

	requirements := make([]ai.QuotaRequirement, len(req.Requirements))
	for i, r := range req.Requirements {
		requirements[i] = ai.QuotaRequirement{
			UsageName:   r.UsageName,
			MinCapacity: r.MinCapacity,
		}
	}

	locations, err := s.modelService.ListLocationsWithQuota(
		ctx, subscriptionId, req.AllowedLocations, requirements)
	if err != nil {
		return nil, fmt.Errorf("listing locations with quota: %w", err)
	}

	protoLocations := make([]*azdext.Location, len(locations))
	for i, loc := range locations {
		protoLocations[i] = &azdext.Location{Name: loc}
	}

	return &azdext.ListLocationsWithQuotaResponse{Locations: protoLocations}, nil
}

// --- Helper functions ---

func extractScope(azureCtx *azdext.AzureContext) (subscriptionId, location string) {
	if azureCtx == nil || azureCtx.Scope == nil {
		return "", ""
	}
	return azureCtx.Scope.SubscriptionId, azureCtx.Scope.Location
}

func protoToFilterOptions(f *azdext.AiModelFilterOptions) *ai.FilterOptions {
	if f == nil {
		return nil
	}
	return &ai.FilterOptions{
		Locations:         f.Locations,
		Capabilities:      f.Capabilities,
		Formats:           f.Formats,
		Statuses:          f.Statuses,
		ExcludeModelNames: f.ExcludeModelNames,
	}
}

func protoToDeploymentOptions(o *azdext.AiModelDeploymentOptions) *ai.DeploymentOptions {
	if o == nil {
		return nil
	}
	opts := &ai.DeploymentOptions{
		Locations: o.Locations,
		Versions:  o.Versions,
		Skus:      o.Skus,
	}
	if o.Capacity != nil {
		cap := *o.Capacity
		opts.Capacity = &cap
	}
	return opts
}

func protoToQuotaCheckOptions(q *azdext.QuotaCheckOptions) *ai.QuotaCheckOptions {
	if q == nil {
		return nil
	}
	return &ai.QuotaCheckOptions{
		MinRemainingCapacity: q.MinRemainingCapacity,
	}
}

func aiModelToProto(m *ai.AiModel) *azdext.AiModel {
	versions := make([]*azdext.AiModelVersion, len(m.Versions))
	for i, v := range m.Versions {
		versions[i] = aiModelVersionToProto(&v)
	}
	return &azdext.AiModel{
		Name:            m.Name,
		Format:          m.Format,
		LifecycleStatus: m.LifecycleStatus,
		Capabilities:    m.Capabilities,
		Versions:        versions,
		Locations:       m.Locations,
	}
}

func aiModelVersionToProto(v *ai.AiModelVersion) *azdext.AiModelVersion {
	skus := make([]*azdext.AiModelSku, len(v.Skus))
	for i, sku := range v.Skus {
		skus[i] = aiModelSkuToProto(&sku)
	}
	return &azdext.AiModelVersion{
		Version:   v.Version,
		IsDefault: v.IsDefault,
		Skus:      skus,
	}
}

func aiModelSkuToProto(s *ai.AiModelSku) *azdext.AiModelSku {
	return &azdext.AiModelSku{
		Name:            s.Name,
		UsageName:       s.UsageName,
		DefaultCapacity: s.DefaultCapacity,
		MinCapacity:     s.MinCapacity,
		MaxCapacity:     s.MaxCapacity,
		CapacityStep:    s.CapacityStep,
	}
}

func aiModelDeploymentToProto(d *ai.AiModelDeployment) *azdext.AiModelDeployment {
	return &azdext.AiModelDeployment{
		ModelName:      d.ModelName,
		Format:         d.Format,
		Version:        d.Version,
		Location:       d.Location,
		Sku:            aiModelSkuToProto(&d.Sku),
		Capacity:       d.Capacity,
		RemainingQuota: d.RemainingQuota,
	}
}

func effectiveLocations(filter *ai.FilterOptions, azureCtx *azdext.AzureContext) []string {
	if filter != nil && len(filter.Locations) > 0 {
		return filter.Locations
	}

	_, scopeLocation := extractScope(azureCtx)
	if scopeLocation != "" {
		return []string{scopeLocation}
	}

	return nil
}

func pickLocation(explicitLocation, scopeLocation string) string {
	if explicitLocation != "" {
		return explicitLocation
	}

	return scopeLocation
}
