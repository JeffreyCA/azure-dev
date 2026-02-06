// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package ai

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
)

// Model represents an AI model aggregated across locations.
type Model struct {
	Name                   string
	DetailsByLocation      map[string][]ModelVersion
}

// ModelVersion represents a specific version of a model at a location.
type ModelVersion struct {
	Version          string
	Format           string
	Kind             string
	IsDefaultVersion bool
	LifecycleStatus  string
	Capabilities     map[string]string
	Skus             []ModelSku
}

// ModelSku represents a SKU available for a model version.
type ModelSku struct {
	Name      string
	UsageName string
	Capacity  ModelSkuCapacity
}

// ModelSkuCapacity represents capacity limits for a model SKU.
type ModelSkuCapacity struct {
	Default int32
	Maximum int32
	Minimum int32
	Step    int32
}

// Usage represents quota/usage data for AI services in a location.
type Usage struct {
	Name         string
	CurrentValue float64
	Limit        float64
}

// QuotaRequirement specifies a minimum capacity requirement for a usage name.
type QuotaRequirement struct {
	UsageName string
	Capacity  float64
}

// FilterOptions defines criteria for filtering AI models.
type FilterOptions struct {
	Capabilities []string
	Statuses     []string
	Formats      []string
	Kinds        []string
	Locations    []string
}

// ModelDeployment represents a resolved deployment configuration for a model.
type ModelDeployment struct {
	Name     string
	Format   string
	Version  string
	Location string
	Sku      ModelDeploymentSku
}

// ModelDeploymentSku represents the SKU portion of a deployment configuration.
type ModelDeploymentSku struct {
	Name      string
	UsageName string
	Capacity  int32
}

// ModelService provides operations for discovering AI models, checking quota, and resolving
// deployment configurations. It wraps the Azure Cognitive Services SDK via azapi.AzureClient.
type ModelService struct {
	azureClient    *azapi.AzureClient
	accountManager account.Manager
}

// NewModelService creates a new ModelService.
func NewModelService(azureClient *azapi.AzureClient, accountManager account.Manager) *ModelService {
	return &ModelService{
		azureClient:    azureClient,
		accountManager: accountManager,
	}
}

// ListModels lists AI models for the given subscription, optionally filtered.
// If location is empty, models are fetched concurrently across all subscription locations.
func (s *ModelService) ListModels(
	ctx context.Context,
	subscriptionId string,
	location string,
	filter *FilterOptions,
) ([]*Model, error) {
	if filter == nil {
		filter = &FilterOptions{}
	}

	var locations []string
	if location != "" {
		locations = []string{location}
	} else {
		allLocations, err := s.accountManager.GetLocations(ctx, subscriptionId)
		if err != nil {
			return nil, fmt.Errorf("getting locations: %w", err)
		}
		locations = make([]string, len(allLocations))
		for i, loc := range allLocations {
			locations[i] = loc.Name
		}
	}

	modelMap := s.fetchModelsFromLocations(ctx, subscriptionId, locations)
	return filterModels(modelMap, filter), nil
}

// ListModelVersions returns available versions for a model in a location.
func (s *ModelService) ListModelVersions(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	location string,
) ([]string, string, error) {
	models, err := s.azureClient.GetAiModels(ctx, subscriptionId, location)
	if err != nil {
		return nil, "", fmt.Errorf("getting models: %w", err)
	}

	versions := make(map[string]struct{})
	defaultVersion := ""

	for _, m := range models {
		if m.Model == nil || m.Model.Name == nil || *m.Model.Name != modelName {
			continue
		}
		if m.Model.Version != nil {
			versions[*m.Model.Version] = struct{}{}
		}
		if m.Model.IsDefaultVersion != nil && *m.Model.IsDefaultVersion && m.Model.Version != nil {
			defaultVersion = *m.Model.Version
		}
	}

	versionList := make([]string, 0, len(versions))
	for v := range versions {
		versionList = append(versionList, v)
	}
	slices.Sort(versionList)

	return versionList, defaultVersion, nil
}

// ListModelSkus returns available SKU names for a model+version in a location.
func (s *ModelService) ListModelSkus(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	location string,
	version string,
) ([]string, error) {
	models, err := s.azureClient.GetAiModels(ctx, subscriptionId, location)
	if err != nil {
		return nil, fmt.Errorf("getting models: %w", err)
	}

	skus := make(map[string]struct{})
	for _, m := range models {
		if m.Model == nil || m.Model.Name == nil || *m.Model.Name != modelName {
			continue
		}
		if m.Model.Version != nil && *m.Model.Version == version {
			for _, sku := range m.Model.SKUs {
				if sku.Name != nil {
					skus[*sku.Name] = struct{}{}
				}
			}
		}
	}

	skuList := make([]string, 0, len(skus))
	for sku := range skus {
		skuList = append(skuList, sku)
	}
	slices.Sort(skuList)

	return skuList, nil
}

// GetModelDeployment resolves a deployment configuration for a model with fallback defaults.
func (s *ModelService) GetModelDeployment(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	preferredLocations []string,
	preferredVersions []string,
	preferredSkus []string,
) (*ModelDeployment, error) {
	if len(preferredSkus) == 0 {
		preferredSkus = []string{"GlobalStandard", "Standard"}
	}

	// Determine locations to search
	var locations []string
	if len(preferredLocations) > 0 {
		locations = preferredLocations
	} else {
		allLocations, err := s.accountManager.GetLocations(ctx, subscriptionId)
		if err != nil {
			return nil, fmt.Errorf("getting locations: %w", err)
		}
		locations = make([]string, len(allLocations))
		for i, loc := range allLocations {
			locations[i] = loc.Name
		}
	}

	modelMap := s.fetchModelsFromLocations(ctx, subscriptionId, locations)

	model, exists := modelMap[modelName]
	if !exists {
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}

	hasDefault := hasDefaultVersion(model)

	for locationName, versions := range model.DetailsByLocation {
		for _, mv := range versions {
			// Check version match
			if len(preferredVersions) > 0 {
				if !slices.Contains(preferredVersions, mv.Version) {
					continue
				}
			} else if hasDefault && !mv.IsDefaultVersion {
				continue
			}

			// Check SKU match
			for _, sku := range mv.Skus {
				if !slices.Contains(preferredSkus, sku.Name) {
					continue
				}

				// Build the full model-qualified usage name.
				// The SKU UsageName (e.g. "OpenAI.Standard") is just the prefix;
				// Azure usage entries are "{UsageName}.{modelName}" (e.g. "OpenAI.Standard.gpt-4o").
				usageName := sku.UsageName
				if usageName != "" {
					usageName = usageName + "." + modelName
				}

				return &ModelDeployment{
					Name:     modelName,
					Format:   mv.Format,
					Version:  mv.Version,
					Location: locationName,
					Sku: ModelDeploymentSku{
						Name:      sku.Name,
						UsageName: usageName,
						Capacity:  sku.Capacity.Default,
					},
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no deployment configuration found for model '%s'", modelName)
}

// ListUsages returns quota/usage data for AI services in a location.
func (s *ModelService) ListUsages(
	ctx context.Context,
	subscriptionId string,
	location string,
) ([]Usage, error) {
	sdkUsages, err := s.azureClient.GetAiUsages(ctx, subscriptionId, location)
	if err != nil {
		return nil, fmt.Errorf("getting usages: %w", err)
	}

	usages := make([]Usage, 0, len(sdkUsages))
	for _, u := range sdkUsages {
		if u.Name == nil || u.Name.Value == nil {
			continue
		}
		usage := Usage{
			Name: *u.Name.Value,
		}
		if u.CurrentValue != nil {
			usage.CurrentValue = *u.CurrentValue
		}
		if u.Limit != nil {
			usage.Limit = *u.Limit
		}
		usages = append(usages, usage)
	}

	return usages, nil
}

// ListLocationsWithQuota returns locations that have sufficient quota for the specified requirements.
func (s *ModelService) ListLocationsWithQuota(
	ctx context.Context,
	subscriptionId string,
	candidateLocations []string,
	requirements []QuotaRequirement,
) ([]string, error) {
	// Get AI Services locations to constrain the search
	aiServicesLocations, err := s.azureClient.GetResourceSkuLocations(
		ctx, subscriptionId, "AIServices", "S0", "Standard", "accounts")
	if err != nil {
		return nil, fmt.Errorf("getting AI Services locations: %w", err)
	}

	if len(candidateLocations) == 0 {
		candidateLocations = aiServicesLocations
	}

	var sharedResults sync.Map
	var wg sync.WaitGroup

	for _, location := range candidateLocations {
		if !slices.Contains(aiServicesLocations, location) {
			continue
		}
		wg.Add(1)
		go func(loc string) {
			defer wg.Done()
			usages, err := s.azureClient.GetAiUsages(ctx, subscriptionId, loc)
			if err != nil {
				log.Println("error getting usage for location", loc, ":", err)
				return
			}
			sharedResults.Store(loc, usages)
		}(location)
	}
	wg.Wait()

	var results []string
	sharedResults.Range(func(key, value any) bool {
		usages := value.([]*armcognitiveservices.Usage)

		// Check S0 baseline quota (minimum 2 capacity units)
		hasS0Quota := slices.ContainsFunc(usages, func(q *armcognitiveservices.Usage) bool {
			return q.Name != nil && q.Name.Value != nil &&
				*q.Name.Value == "OpenAI.S0.AccountCount" &&
				q.Limit != nil && q.CurrentValue != nil &&
				(*q.Limit-*q.CurrentValue) >= 2
		})
		if !hasS0Quota {
			return true
		}

		// Check all additional requirements
		for _, req := range requirements {
			hasQuota := slices.ContainsFunc(usages, func(u *armcognitiveservices.Usage) bool {
				if u.Name == nil || u.Name.Value == nil || u.Limit == nil || u.CurrentValue == nil {
					return false
				}
				return *u.Name.Value == req.UsageName && (*u.Limit-*u.CurrentValue) >= req.Capacity
			})
			if !hasQuota {
				return true
			}
		}

		results = append(results, key.(string))
		return true
	})

	slices.Sort(results)
	return results, nil
}

// ListSkuLocations returns locations where a specific AI Services resource SKU is available.
func (s *ModelService) ListSkuLocations(
	ctx context.Context,
	subscriptionId string,
	kind, skuName, tier, resourceType string,
) ([]string, error) {
	return s.azureClient.GetResourceSkuLocations(ctx, subscriptionId, kind, skuName, tier, resourceType)
}

// ResolvedDeployment extends ModelDeployment with quota validation results.
type ResolvedDeployment struct {
	ModelDeployment
	QuotaValidated    bool
	AvailableCapacity float64
}

// ResolveModelDeployment resolves a deployment configuration and validates quota availability.
// It tries each matching SKU/location candidate and picks the first with sufficient quota.
// If no candidate has quota, it returns the best-effort deployment with QuotaValidated=false.
func (s *ModelService) ResolveModelDeployment(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	preferredLocation string,
	preferredVersions []string,
	preferredSkus []string,
	minCapacity *int32,
	filter *FilterOptions,
) (*ResolvedDeployment, error) {
	if len(preferredSkus) == 0 {
		preferredSkus = []string{"GlobalStandard", "DataZoneStandard", "Standard"}
	}

	// Build location list: preferred first, then all others
	allLocations, err := s.azureClient.GetResourceSkuLocations(
		ctx, subscriptionId, "AIServices", "S0", "Standard", "accounts")
	if err != nil {
		return nil, fmt.Errorf("getting AI Services locations: %w", err)
	}

	var locations []string
	if preferredLocation != "" {
		locations = append(locations, preferredLocation)
		for _, loc := range allLocations {
			if loc != preferredLocation {
				locations = append(locations, loc)
			}
		}
	} else {
		locations = allLocations
	}

	modelMap := s.fetchModelsFromLocations(ctx, subscriptionId, locations)

	model, exists := modelMap[modelName]
	if !exists {
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}

	hasDefault := hasDefaultVersion(model)

	// Collect all deployment candidates
	type candidate struct {
		deployment ModelDeployment
	}
	var candidates []candidate

	// Try preferred location first, then others
	for _, loc := range locations {
		versions, ok := model.DetailsByLocation[loc]
		if !ok {
			continue
		}
		for _, mv := range versions {
			if len(preferredVersions) > 0 {
				if !slices.Contains(preferredVersions, mv.Version) {
					continue
				}
			} else if hasDefault && !mv.IsDefaultVersion {
				continue
			}

			// Apply filter if provided
			if filter != nil {
				if len(filter.Formats) > 0 && !slices.Contains(filter.Formats, mv.Format) {
					continue
				}
				if len(filter.Kinds) > 0 && !slices.Contains(filter.Kinds, mv.Kind) {
					continue
				}
				if len(filter.Statuses) > 0 && !slices.Contains(filter.Statuses, mv.LifecycleStatus) {
					continue
				}
			}

			for _, skuPref := range preferredSkus {
				for _, sku := range mv.Skus {
					if sku.Name != skuPref {
						continue
					}
					usageName := sku.UsageName
					if usageName != "" {
						usageName = usageName + "." + modelName
					}
					capacity := sku.Capacity.Default
					if minCapacity != nil && *minCapacity > capacity {
						capacity = *minCapacity
					}

					candidates = append(candidates, candidate{
						deployment: ModelDeployment{
							Name:     modelName,
							Format:   mv.Format,
							Version:  mv.Version,
							Location: loc,
							Sku: ModelDeploymentSku{
								Name:      sku.Name,
								UsageName: usageName,
								Capacity:  capacity,
							},
						},
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no deployment configuration found for model '%s'", modelName)
	}

	// Try each candidate and validate quota
	for _, c := range candidates {
		if c.deployment.Sku.UsageName == "" {
			continue
		}
		usages, err := s.azureClient.GetAiUsages(ctx, subscriptionId, c.deployment.Location)
		if err != nil {
			continue
		}

		for _, u := range usages {
			if u.Name == nil || u.Name.Value == nil || u.Limit == nil || u.CurrentValue == nil {
				continue
			}
			if *u.Name.Value == c.deployment.Sku.UsageName {
				available := *u.Limit - *u.CurrentValue
				if available >= float64(c.deployment.Sku.Capacity) {
					return &ResolvedDeployment{
						ModelDeployment:   c.deployment,
						QuotaValidated:    true,
						AvailableCapacity: available,
					}, nil
				}
			}
		}
	}

	// No candidate had sufficient quota — return best-effort (first candidate)
	return &ResolvedDeployment{
		ModelDeployment:   candidates[0].deployment,
		QuotaValidated:    false,
		AvailableCapacity: 0,
	}, nil
}

// ModelAvailability holds the result of a model availability check.
type ModelAvailability struct {
	Available            bool
	AlternativeLocations []string
	AlternativeModels    []string
}

// ValidateModelAvailability checks whether a model is available in a location,
// and returns alternatives if not.
func (s *ModelService) ValidateModelAvailability(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	location string,
	filter *FilterOptions,
	maxAlternatives int,
) (*ModelAvailability, error) {
	if maxAlternatives <= 0 {
		maxAlternatives = 10
	}

	// Get all AI Services locations for broad search
	aiLocations, err := s.azureClient.GetResourceSkuLocations(
		ctx, subscriptionId, "AIServices", "S0", "Standard", "accounts")
	if err != nil {
		return nil, fmt.Errorf("getting AI Services locations: %w", err)
	}

	// Fetch models from the requested location first
	localModels, err := s.azureClient.GetAiModels(ctx, subscriptionId, location)
	if err != nil {
		return nil, fmt.Errorf("getting models in location '%s': %w", location, err)
	}

	// Check if the requested model is available
	available := slices.ContainsFunc(localModels, func(m *armcognitiveservices.Model) bool {
		return m.Model != nil && m.Model.Name != nil && *m.Model.Name == modelName
	})

	if available {
		return &ModelAvailability{Available: true}, nil
	}

	result := &ModelAvailability{Available: false}

	// Find alternative models in the requested location
	altModelSet := make(map[string]struct{})
	for _, m := range localModels {
		if m.Model == nil || m.Model.Name == nil {
			continue
		}
		name := *m.Model.Name
		if name == modelName {
			continue
		}
		if _, seen := altModelSet[name]; seen {
			continue
		}

		// Apply filter if provided
		if filter != nil {
			mv := convertSDKModel(m)
			tempModel := &Model{
				Name:              name,
				DetailsByLocation: map[string][]ModelVersion{location: {mv}},
			}
			if !matchesFilter(tempModel, filter) {
				continue
			}
		}

		altModelSet[name] = struct{}{}
		result.AlternativeModels = append(result.AlternativeModels, name)
		if len(result.AlternativeModels) >= maxAlternatives {
			break
		}
	}
	slices.Sort(result.AlternativeModels)

	// Find alternative locations — search remaining locations concurrently
	otherLocations := make([]string, 0, len(aiLocations))
	for _, loc := range aiLocations {
		if loc != location {
			otherLocations = append(otherLocations, loc)
		}
	}

	modelMap := s.fetchModelsFromLocations(ctx, subscriptionId, otherLocations)
	if model, exists := modelMap[modelName]; exists {
		for loc := range model.DetailsByLocation {
			result.AlternativeLocations = append(result.AlternativeLocations, loc)
			if len(result.AlternativeLocations) >= maxAlternatives {
				break
			}
		}
		slices.Sort(result.AlternativeLocations)
	}

	return result, nil
}

// fetchModelsFromLocations concurrently fetches models from multiple locations and builds
// an aggregated map of model name -> Model.
func (s *ModelService) fetchModelsFromLocations(
	ctx context.Context,
	subscriptionId string,
	locations []string,
) map[string]*Model {
	var locationResults sync.Map
	var wg sync.WaitGroup

	for _, loc := range locations {
		wg.Add(1)
		go func(location string) {
			defer wg.Done()
			models, err := s.azureClient.GetAiModels(ctx, subscriptionId, location)
			if err != nil {
				log.Println("error getting models in location", location, ":", err, "skipping")
				return
			}
			locationResults.Store(location, models)
		}(loc)
	}
	wg.Wait()

	modelMap := map[string]*Model{}
	locationResults.Range(func(key, value any) bool {
		location := key.(string)
		models := value.([]*armcognitiveservices.Model)

		for _, m := range models {
			if m.Model == nil || m.Model.Name == nil {
				continue
			}
			modelName := *m.Model.Name
			existing, exists := modelMap[modelName]
			if !exists {
				existing = &Model{
					Name:              modelName,
					DetailsByLocation: make(map[string][]ModelVersion),
				}
				modelMap[modelName] = existing
			}

			mv := convertSDKModel(m)
			existing.DetailsByLocation[location] = append(existing.DetailsByLocation[location], mv)
		}
		return true
	})

	return modelMap
}

// convertSDKModel converts an ARM SDK model to our domain ModelVersion type.
func convertSDKModel(m *armcognitiveservices.Model) ModelVersion {
	mv := ModelVersion{}
	if m.Model.Version != nil {
		mv.Version = *m.Model.Version
	}
	if m.Model.Format != nil {
		mv.Format = *m.Model.Format
	}
	if m.Kind != nil {
		mv.Kind = *m.Kind
	}
	if m.Model.IsDefaultVersion != nil {
		mv.IsDefaultVersion = *m.Model.IsDefaultVersion
	}
	if m.Model.LifecycleStatus != nil {
		mv.LifecycleStatus = string(*m.Model.LifecycleStatus)
	}
	if m.Model.Capabilities != nil {
		mv.Capabilities = make(map[string]string)
		for k, v := range m.Model.Capabilities {
			if v != nil {
				mv.Capabilities[k] = *v
			}
		}
	}
	for _, sku := range m.Model.SKUs {
		ms := ModelSku{}
		if sku.Name != nil {
			ms.Name = *sku.Name
		}
		if sku.UsageName != nil {
			ms.UsageName = *sku.UsageName
		}
		if sku.Capacity != nil {
			if sku.Capacity.Default != nil {
				ms.Capacity.Default = *sku.Capacity.Default
			}
			if sku.Capacity.Maximum != nil {
				ms.Capacity.Maximum = *sku.Capacity.Maximum
			}
			if sku.Capacity.Minimum != nil {
				ms.Capacity.Minimum = *sku.Capacity.Minimum
			}
			if sku.Capacity.Step != nil {
				ms.Capacity.Step = *sku.Capacity.Step
			}
		}
		mv.Skus = append(mv.Skus, ms)
	}
	return mv
}

// filterModels applies FilterOptions to a model map and returns matching models sorted by name.
func filterModels(modelMap map[string]*Model, filter *FilterOptions) []*Model {
	var result []*Model

	for _, model := range modelMap {
		if matchesFilter(model, filter) {
			result = append(result, model)
		}
	}

	slices.SortFunc(result, func(a, b *Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	return result
}

// matchesFilter checks if a model matches all filter criteria (AND logic across filter types).
func matchesFilter(model *Model, filter *FilterOptions) bool {
	isCapabilityMatch := len(filter.Capabilities) == 0
	isLocationMatch := len(filter.Locations) == 0
	isStatusMatch := len(filter.Statuses) == 0
	isFormatMatch := len(filter.Formats) == 0
	isKindMatch := len(filter.Kinds) == 0

	for locationName, versions := range model.DetailsByLocation {
		for _, mv := range versions {
			if !isCapabilityMatch {
				for cap := range mv.Capabilities {
					if slices.Contains(filter.Capabilities, cap) {
						isCapabilityMatch = true
						break
					}
				}
			}

			if !isLocationMatch && slices.Contains(filter.Locations, locationName) {
				isLocationMatch = true
			}

			if !isStatusMatch && slices.Contains(filter.Statuses, mv.LifecycleStatus) {
				isStatusMatch = true
			}

			if !isFormatMatch && slices.Contains(filter.Formats, mv.Format) {
				isFormatMatch = true
			}

			if !isKindMatch && slices.Contains(filter.Kinds, mv.Kind) {
				isKindMatch = true
			}
		}
	}

	return isCapabilityMatch && isLocationMatch && isStatusMatch && isFormatMatch && isKindMatch
}

// hasDefaultVersion checks if any version of the model is marked as default.
func hasDefaultVersion(model *Model) bool {
	for _, versions := range model.DetailsByLocation {
		for _, mv := range versions {
			if mv.IsDefaultVersion {
				return true
			}
		}
	}
	return false
}
