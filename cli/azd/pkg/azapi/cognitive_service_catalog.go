// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azapi

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
)

const (
	defaultAiLookupConcurrency = 8
	aiSkuKind                  = "AIServices"
	aiSkuName                  = "S0"
	aiSkuTier                  = "Standard"
	aiSkuResourceType          = "accounts"

	// AiAccountQuotaUsageName is the usage meter required to create Azure AI Services accounts.
	AiAccountQuotaUsageName = "OpenAI.S0.AccountCount"
)

// AiUsageRequirement defines a usage meter name and required capacity.
type AiUsageRequirement struct {
	UsageName        string
	RequiredCapacity float64
}

// AiUsageSnapshot captures usage values for a meter in a location.
type AiUsageSnapshot struct {
	Name      string
	Current   float64
	Limit     float64
	Remaining float64
	Unit      string
}

// AiModelSku describes a model SKU in the model catalog.
type AiModelSku struct {
	Name            string
	UsageName       string
	CapacityDefault int32
	CapacityMinimum int32
	CapacityMaximum int32
	CapacityStep    int32
}

// AiModelVersion describes a model version in a location.
type AiModelVersion struct {
	Version          string
	IsDefaultVersion bool
	Kind             string
	Format           string
	Status           string
	Capabilities     []string
	Skus             []AiModelSku
}

// AiModelLocation describes model versions for a location.
type AiModelLocation struct {
	Location string
	Versions []AiModelVersion
}

// AiModelCatalogItem describes a model and its per-location availability.
type AiModelCatalogItem struct {
	Name      string
	Locations []AiModelLocation
}

// AiModelCatalogFilters defines filters for model catalog queries.
type AiModelCatalogFilters struct {
	Locations    []string
	Kinds        []string
	Formats      []string
	Statuses     []string
	Capabilities []string
}

// AiLocationQuotaUsage captures requirement evaluation for one usage meter.
type AiLocationQuotaUsage struct {
	UsageName         string
	RequiredCapacity  float64
	AvailableCapacity float64
}

// AiLocationQuotaResult captures quota evaluation for one location.
type AiLocationQuotaResult struct {
	Location     string
	Matched      bool
	Requirements []AiLocationQuotaUsage
	Error        string
}

// AiLocationsWithQuotaOptions controls quota lookup behavior.
type AiLocationsWithQuotaOptions struct {
	RequireAccountQuota bool
	MinimumAccountQuota float64
	MaxConcurrency      int
}

// AiLocationsWithQuotaResult contains locations that matched all requirements and diagnostic details.
type AiLocationsWithQuotaResult struct {
	MatchedLocations []string
	Results          []AiLocationQuotaResult
}

// AiFindLocationsForModelWithQuotaOptions controls model + quota lookup behavior.
type AiFindLocationsForModelWithQuotaOptions struct {
	Locations           []string
	Versions            []string
	Skus                []string
	Kinds               []string
	Formats             []string
	Statuses            []string
	Capabilities        []string
	Requirements        []AiUsageRequirement
	RequireAccountQuota bool
	MinimumAccountQuota float64
	MaxConcurrency      int
}

// AiModelDeployment describes a model deployment candidate.
type AiModelDeployment struct {
	ModelName        string
	Version          string
	IsDefaultVersion bool
	Kind             string
	Format           string
	Status           string
	Capabilities     []string
	Sku              AiModelSku
}

// AiModelLocationQuotaResult captures model and quota evaluation for one location.
type AiModelLocationQuotaResult struct {
	Location     string
	Matched      bool
	Deployment   *AiModelDeployment
	Requirements []AiLocationQuotaUsage
	Error        string
}

// AiLocationsForModelWithQuotaResult contains locations where a model deployment matched all quota requirements.
type AiLocationsForModelWithQuotaResult struct {
	MatchedLocations []string
	Results          []AiModelLocationQuotaResult
}

type versionByKey struct {
	key     string
	version AiModelVersion
}

type aiModelQuotaCandidate struct {
	location     string
	deployment   AiModelDeployment
	requirements []AiUsageRequirement
}

// ParseAiUsageRequirement parses usage requirement values in format "usageName[,capacity]".
func ParseAiUsageRequirement(value string) (AiUsageRequirement, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return AiUsageRequirement{}, fmt.Errorf("empty usage name")
	}

	parts := strings.Split(trimmed, ",")
	switch len(parts) {
	case 1:
		usageName := strings.TrimSpace(parts[0])
		if usageName == "" {
			return AiUsageRequirement{}, fmt.Errorf("empty usage name")
		}

		return AiUsageRequirement{
			UsageName:        usageName,
			RequiredCapacity: 1,
		}, nil
	case 2:
		usageName := strings.TrimSpace(parts[0])
		if usageName == "" {
			return AiUsageRequirement{}, fmt.Errorf("empty usage name")
		}
		required, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return AiUsageRequirement{}, fmt.Errorf("invalid capacity '%s': %w", strings.TrimSpace(parts[1]), err)
		}
		if required <= 0 {
			return AiUsageRequirement{}, fmt.Errorf("invalid capacity '%v': must be greater than 0", required)
		}

		return AiUsageRequirement{
			UsageName:        usageName,
			RequiredCapacity: required,
		}, nil
	default:
		return AiUsageRequirement{}, fmt.Errorf("invalid usage name format '%s'", trimmed)
	}
}

// ParseAiUsageRequirements parses a list of requirement strings in format "usageName[,capacity]".
func ParseAiUsageRequirements(values []string) ([]AiUsageRequirement, error) {
	requirements := make([]AiUsageRequirement, len(values))
	for i, value := range values {
		req, err := ParseAiUsageRequirement(value)
		if err != nil {
			return nil, err
		}

		requirements[i] = req
	}

	return requirements, nil
}

// ListAiLocations returns Azure AI Services locations for a subscription and optional allow-list.
func (cli *AzureClient) ListAiLocations(
	ctx context.Context,
	subscriptionId string,
	allowedLocations []string,
) ([]string, error) {
	locations, err := cli.GetResourceSkuLocations(
		ctx,
		subscriptionId,
		aiSkuKind,
		aiSkuName,
		aiSkuTier,
		aiSkuResourceType,
	)
	if err != nil {
		return nil, fmt.Errorf("getting Azure AI Services locations: %w", err)
	}

	if len(allowedLocations) == 0 {
		return locations, nil
	}

	allowed := make(map[string]struct{}, len(allowedLocations))
	for _, location := range allowedLocations {
		allowed[strings.ToLower(location)] = struct{}{}
	}

	filtered := make([]string, 0, len(locations))
	for _, location := range locations {
		if _, has := allowed[strings.ToLower(location)]; has {
			filtered = append(filtered, location)
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no Azure AI Services locations found in the provided allow-list")
	}

	return filtered, nil
}

// ListAiUsages lists usage values for a subscription location and optional usage name prefix.
func (cli *AzureClient) ListAiUsages(
	ctx context.Context,
	subscriptionId string,
	location string,
	namePrefix string,
) ([]AiUsageSnapshot, error) {
	usages, err := cli.GetAiUsages(ctx, subscriptionId, strings.ToLower(location))
	if err != nil {
		return nil, fmt.Errorf("getting AI usages for location '%s': %w", location, err)
	}

	prefix := strings.ToLower(strings.TrimSpace(namePrefix))
	results := make([]AiUsageSnapshot, 0, len(usages))
	for _, usage := range usages {
		name := safeUsageName(usage)
		if name == "" {
			continue
		}

		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), prefix) {
			continue
		}

		current := safeUsageCurrentValue(usage)
		limit := safeUsageLimit(usage)
		results = append(results, AiUsageSnapshot{
			Name:      name,
			Current:   current,
			Limit:     limit,
			Remaining: limit - current,
			Unit:      safeUsageUnit(usage),
		})
	}

	slices.SortFunc(results, func(a, b AiUsageSnapshot) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	return results, nil
}

// ListAiModelCatalog lists models grouped by model name and location, with optional filters.
func (cli *AzureClient) ListAiModelCatalog(
	ctx context.Context,
	subscriptionId string,
	filters AiModelCatalogFilters,
) ([]AiModelCatalogItem, error) {
	locations, err := cli.ListAiLocations(ctx, subscriptionId, filters.Locations)
	if err != nil {
		return nil, err
	}

	maxConcurrency := defaultAiLookupConcurrency
	type modelsByLocation struct {
		location string
		models   []*armcognitiveservices.Model
		err      error
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		sem     = make(chan struct{}, maxConcurrency)
		results []modelsByLocation
	)

	for _, location := range locations {
		wg.Add(1)
		go func(location string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			models, err := cli.GetAiModels(ctx, subscriptionId, location)

			mu.Lock()
			results = append(results, modelsByLocation{
				location: location,
				models:   models,
				err:      err,
			})
			mu.Unlock()
		}(location)
	}
	wg.Wait()

	kindFilter := normalizeFilter(filters.Kinds)
	formatFilter := normalizeFilter(filters.Formats)
	statusFilter := normalizeFilter(filters.Statuses)
	capabilityFilter := normalizeFilter(filters.Capabilities)

	modelMap := map[string]map[string]map[string]*AiModelVersion{}

	successfulLocations := 0
	for _, locationResult := range results {
		if locationResult.err != nil {
			continue
		}
		successfulLocations++

		for _, model := range locationResult.models {
			catalogName := safeModelName(model)
			if catalogName == "" {
				continue
			}

			versionWithKey, ok := toAiModelVersion(model)
			if !ok {
				continue
			}

			if !matchesFilter(versionWithKey.version.Kind, kindFilter) ||
				!matchesFilter(versionWithKey.version.Format, formatFilter) ||
				!matchesFilter(versionWithKey.version.Status, statusFilter) ||
				!matchesCapabilities(versionWithKey.version.Capabilities, capabilityFilter) {
				continue
			}

			locationMap, has := modelMap[catalogName]
			if !has {
				locationMap = map[string]map[string]*AiModelVersion{}
				modelMap[catalogName] = locationMap
			}

			versionMap, has := locationMap[locationResult.location]
			if !has {
				versionMap = map[string]*AiModelVersion{}
				locationMap[locationResult.location] = versionMap
			}

			existing, has := versionMap[versionWithKey.key]
			if !has {
				versionCopy := versionWithKey.version
				versionMap[versionWithKey.key] = &versionCopy
				continue
			}

			existing.IsDefaultVersion = existing.IsDefaultVersion || versionWithKey.version.IsDefaultVersion
			existing.Capabilities = mergeModelCapabilities(existing.Capabilities, versionWithKey.version.Capabilities)
			existing.Skus = mergeModelSkus(existing.Skus, versionWithKey.version.Skus)
		}
	}

	if successfulLocations == 0 {
		return nil, fmt.Errorf("failed retrieving model catalog for all locations")
	}

	sortedNames := make([]string, 0, len(modelMap))
	for name := range modelMap {
		sortedNames = append(sortedNames, name)
	}
	slices.SortFunc(sortedNames, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})

	items := make([]AiModelCatalogItem, 0, len(sortedNames))
	for _, modelName := range sortedNames {
		locationMap := modelMap[modelName]
		locationNames := make([]string, 0, len(locationMap))
		for location := range locationMap {
			locationNames = append(locationNames, location)
		}

		slices.Sort(locationNames)
		locationItems := make([]AiModelLocation, 0, len(locationNames))
		for _, location := range locationNames {
			versions := make([]AiModelVersion, 0, len(locationMap[location]))
			for _, version := range locationMap[location] {
				sortModelSkus(version.Skus)
				versions = append(versions, *version)
			}

			slices.SortFunc(versions, func(a, b AiModelVersion) int {
				return strings.Compare(a.Version, b.Version)
			})
			locationItems = append(locationItems, AiModelLocation{
				Location: location,
				Versions: versions,
			})
		}

		items = append(items, AiModelCatalogItem{
			Name:      modelName,
			Locations: locationItems,
		})
	}

	return items, nil
}

// FindAiLocationsWithQuota checks requested usage requirements across locations.
func (cli *AzureClient) FindAiLocationsWithQuota(
	ctx context.Context,
	subscriptionId string,
	locations []string,
	requirements []AiUsageRequirement,
	options *AiLocationsWithQuotaOptions,
) (*AiLocationsWithQuotaResult, error) {
	resolvedOptions := AiLocationsWithQuotaOptions{
		RequireAccountQuota: true,
		MinimumAccountQuota: 2,
		MaxConcurrency:      defaultAiLookupConcurrency,
	}
	if options != nil {
		resolvedOptions = *options
	}
	if resolvedOptions.MinimumAccountQuota <= 0 {
		resolvedOptions.MinimumAccountQuota = 2
	}
	if resolvedOptions.MaxConcurrency <= 0 {
		resolvedOptions.MaxConcurrency = defaultAiLookupConcurrency
	}

	resolvedLocations, err := cli.ListAiLocations(ctx, subscriptionId, locations)
	if err != nil {
		return nil, err
	}

	allRequirements := slices.Clone(requirements)
	if resolvedOptions.RequireAccountQuota {
		allRequirements = append(allRequirements, AiUsageRequirement{
			UsageName:        AiAccountQuotaUsageName,
			RequiredCapacity: resolvedOptions.MinimumAccountQuota,
		})
	}
	allRequirements = mergeAiRequirements(allRequirements)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		sem     = make(chan struct{}, resolvedOptions.MaxConcurrency)
		results = make([]AiLocationQuotaResult, 0, len(resolvedLocations))
	)

	for _, location := range resolvedLocations {
		wg.Add(1)
		go func(location string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := AiLocationQuotaResult{
				Location: location,
			}

			usages, err := cli.GetAiUsages(ctx, subscriptionId, location)
			if err != nil {
				result.Error = err.Error()
				result.Matched = false
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return
			}

			usageByName := make(map[string]*armcognitiveservices.Usage, len(usages))
			for _, usage := range usages {
				usageName := safeUsageName(usage)
				if usageName == "" {
					continue
				}
				usageByName[strings.ToLower(usageName)] = usage
			}

			result.Matched = true
			for _, requirement := range allRequirements {
				usage, has := usageByName[strings.ToLower(requirement.UsageName)]
				available := 0.0
				if has {
					available = safeUsageLimit(usage) - safeUsageCurrentValue(usage)
				}

				result.Requirements = append(result.Requirements, AiLocationQuotaUsage{
					UsageName:         requirement.UsageName,
					RequiredCapacity:  requirement.RequiredCapacity,
					AvailableCapacity: available,
				})

				if !has || available < requirement.RequiredCapacity {
					result.Matched = false
				}
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(location)
	}

	wg.Wait()

	slices.SortFunc(results, func(a, b AiLocationQuotaResult) int {
		return strings.Compare(strings.ToLower(a.Location), strings.ToLower(b.Location))
	})

	matchedLocations := make([]string, 0, len(results))
	for _, result := range results {
		if result.Matched {
			matchedLocations = append(matchedLocations, result.Location)
		}
	}

	return &AiLocationsWithQuotaResult{
		MatchedLocations: matchedLocations,
		Results:          results,
	}, nil
}

// FindAiLocationsForModelWithQuota checks where a model deployment is available and quota requirements are met.
func (cli *AzureClient) FindAiLocationsForModelWithQuota(
	ctx context.Context,
	subscriptionId string,
	modelName string,
	options *AiFindLocationsForModelWithQuotaOptions,
) (*AiLocationsForModelWithQuotaResult, error) {
	trimmedModelName := strings.TrimSpace(modelName)
	if trimmedModelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedOptions := AiFindLocationsForModelWithQuotaOptions{
		RequireAccountQuota: true,
		MinimumAccountQuota: 2,
		MaxConcurrency:      defaultAiLookupConcurrency,
	}
	if options != nil {
		resolvedOptions = *options
	}
	if resolvedOptions.MinimumAccountQuota <= 0 {
		resolvedOptions.MinimumAccountQuota = 2
	}
	if resolvedOptions.MaxConcurrency <= 0 {
		resolvedOptions.MaxConcurrency = defaultAiLookupConcurrency
	}

	models, err := cli.ListAiModelCatalog(ctx, subscriptionId, AiModelCatalogFilters{
		Locations:    resolvedOptions.Locations,
		Kinds:        resolvedOptions.Kinds,
		Formats:      resolvedOptions.Formats,
		Statuses:     resolvedOptions.Statuses,
		Capabilities: resolvedOptions.Capabilities,
	})
	if err != nil {
		return nil, fmt.Errorf("listing AI model catalog: %w", err)
	}

	var selectedModel *AiModelCatalogItem
	for i := range models {
		if strings.EqualFold(models[i].Name, trimmedModelName) {
			selectedModel = &models[i]
			break
		}
	}
	if selectedModel == nil {
		return nil, fmt.Errorf("model '%s' not found", trimmedModelName)
	}

	versionFilter := normalizeFilter(resolvedOptions.Versions)
	skuFilter := normalizeFilter(resolvedOptions.Skus)

	candidatesByLocation := map[string][]aiModelQuotaCandidate{}
	for _, location := range selectedModel.Locations {
		locationName := strings.ToLower(strings.TrimSpace(location.Location))
		if locationName == "" {
			continue
		}

		for _, version := range location.Versions {
			if !matchesFilter(version.Version, versionFilter) {
				continue
			}

			for _, sku := range version.Skus {
				if !matchesFilter(sku.Name, skuFilter) {
					continue
				}

				usageName := strings.TrimSpace(sku.UsageName)
				if usageName == "" {
					continue
				}

				deploymentRequirements := slices.Clone(resolvedOptions.Requirements)
				requiredCapacity := float64(sku.CapacityDefault)
				if requiredCapacity <= 0 {
					requiredCapacity = 1
				}

				deploymentRequirements = append(deploymentRequirements, AiUsageRequirement{
					UsageName:        usageName,
					RequiredCapacity: requiredCapacity,
				})

				candidatesByLocation[locationName] = append(candidatesByLocation[locationName], aiModelQuotaCandidate{
					location: locationName,
					deployment: AiModelDeployment{
						ModelName:        selectedModel.Name,
						Version:          version.Version,
						IsDefaultVersion: version.IsDefaultVersion,
						Kind:             version.Kind,
						Format:           version.Format,
						Status:           version.Status,
						Capabilities:     slices.Clone(version.Capabilities),
						Sku:              sku,
					},
					requirements: mergeAiRequirements(deploymentRequirements),
				})
			}
		}
	}

	evaluationLocations := make([]string, 0, len(candidatesByLocation))
	for location := range candidatesByLocation {
		evaluationLocations = append(evaluationLocations, location)
	}
	if len(resolvedOptions.Locations) > 0 {
		evaluationLocations = normalizeLocations(resolvedOptions.Locations)
	}
	if len(evaluationLocations) == 0 {
		return nil, fmt.Errorf("no model deployments found for model '%s' with the requested filters", trimmedModelName)
	}

	results := make([]AiModelLocationQuotaResult, 0, len(evaluationLocations))
	for _, location := range evaluationLocations {
		candidates := candidatesByLocation[strings.ToLower(location)]
		if len(candidates) == 0 {
			results = append(results, AiModelLocationQuotaResult{
				Location: strings.ToLower(location),
				Matched:  false,
				Error:    "model deployment is unavailable for this location with the requested filters",
			})
			continue
		}

		slices.SortFunc(candidates, compareAiModelQuotaCandidates)
		candidatesByLocation[strings.ToLower(location)] = candidates
	}

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		sem = make(chan struct{}, resolvedOptions.MaxConcurrency)
	)

	for _, location := range evaluationLocations {
		candidateSet := candidatesByLocation[strings.ToLower(location)]
		if len(candidateSet) == 0 {
			continue
		}

		wg.Add(1)
		go func(location string, candidates []aiModelQuotaCandidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := evaluateModelQuotaCandidates(
				ctx,
				cli,
				subscriptionId,
				strings.ToLower(location),
				candidates,
				resolvedOptions.RequireAccountQuota,
				resolvedOptions.MinimumAccountQuota,
			)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(location, candidateSet)
	}

	wg.Wait()

	slices.SortFunc(results, func(a, b AiModelLocationQuotaResult) int {
		return strings.Compare(strings.ToLower(a.Location), strings.ToLower(b.Location))
	})

	matchedLocations := make([]string, 0, len(results))
	for _, result := range results {
		if result.Matched {
			matchedLocations = append(matchedLocations, result.Location)
		}
	}

	return &AiLocationsForModelWithQuotaResult{
		MatchedLocations: matchedLocations,
		Results:          results,
	}, nil
}

func mergeAiRequirements(requirements []AiUsageRequirement) []AiUsageRequirement {
	if len(requirements) == 0 {
		return nil
	}

	mergedByName := map[string]AiUsageRequirement{}
	order := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		normalizedName := strings.TrimSpace(requirement.UsageName)
		if normalizedName == "" {
			continue
		}

		key := strings.ToLower(normalizedName)
		if existing, has := mergedByName[key]; has {
			if requirement.RequiredCapacity > existing.RequiredCapacity {
				existing.RequiredCapacity = requirement.RequiredCapacity
				mergedByName[key] = existing
			}
			continue
		}

		mergedByName[key] = AiUsageRequirement{
			UsageName:        normalizedName,
			RequiredCapacity: requirement.RequiredCapacity,
		}
		order = append(order, key)
	}

	merged := make([]AiUsageRequirement, 0, len(order))
	for _, key := range order {
		merged = append(merged, mergedByName[key])
	}

	return merged
}

func normalizeLocations(locations []string) []string {
	normalized := make([]string, 0, len(locations))
	seen := map[string]struct{}{}
	for _, location := range locations {
		trimmed := strings.ToLower(strings.TrimSpace(location))
		if trimmed == "" {
			continue
		}
		if _, has := seen[trimmed]; has {
			continue
		}

		normalized = append(normalized, trimmed)
		seen[trimmed] = struct{}{}
	}

	return normalized
}

func compareAiModelQuotaCandidates(a, b aiModelQuotaCandidate) int {
	if a.deployment.IsDefaultVersion != b.deployment.IsDefaultVersion {
		if a.deployment.IsDefaultVersion {
			return -1
		}

		return 1
	}

	if versionCompare := strings.Compare(a.deployment.Version, b.deployment.Version); versionCompare != 0 {
		return versionCompare
	}

	return strings.Compare(strings.ToLower(a.deployment.Sku.Name), strings.ToLower(b.deployment.Sku.Name))
}

func evaluateModelQuotaCandidates(
	ctx context.Context,
	cli *AzureClient,
	subscriptionId string,
	location string,
	candidates []aiModelQuotaCandidate,
	requireAccountQuota bool,
	minimumAccountQuota float64,
) AiModelLocationQuotaResult {
	result := AiModelLocationQuotaResult{
		Location:   location,
		Matched:    false,
		Deployment: &candidates[0].deployment,
	}

	usages, err := cli.GetAiUsages(ctx, subscriptionId, location)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	usageByName := make(map[string]*armcognitiveservices.Usage, len(usages))
	for _, usage := range usages {
		usageName := safeUsageName(usage)
		if usageName == "" {
			continue
		}

		usageByName[strings.ToLower(usageName)] = usage
	}

	bestRequirements := []AiLocationQuotaUsage{}
	for _, candidate := range candidates {
		allRequirements := slices.Clone(candidate.requirements)
		if requireAccountQuota {
			allRequirements = append(allRequirements, AiUsageRequirement{
				UsageName:        AiAccountQuotaUsageName,
				RequiredCapacity: minimumAccountQuota,
			})
		}
		allRequirements = mergeAiRequirements(allRequirements)

		evaluatedRequirements := make([]AiLocationQuotaUsage, 0, len(allRequirements))
		candidateMatched := true
		for _, requirement := range allRequirements {
			usage, has := usageByName[strings.ToLower(requirement.UsageName)]
			available := 0.0
			if has {
				available = safeUsageLimit(usage) - safeUsageCurrentValue(usage)
			}

			evaluatedRequirements = append(evaluatedRequirements, AiLocationQuotaUsage{
				UsageName:         requirement.UsageName,
				RequiredCapacity:  requirement.RequiredCapacity,
				AvailableCapacity: available,
			})

			if !has || available < requirement.RequiredCapacity {
				candidateMatched = false
			}
		}

		if candidateMatched {
			result.Matched = true
			result.Deployment = &candidate.deployment
			result.Requirements = evaluatedRequirements
			return result
		}

		if len(bestRequirements) == 0 {
			bestRequirements = evaluatedRequirements
			result.Deployment = &candidate.deployment
		}
	}

	result.Requirements = bestRequirements
	return result
}

func normalizeFilter(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}

	return result
}

func matchesFilter(value string, filter map[string]struct{}) bool {
	if len(filter) == 0 {
		return true
	}

	_, has := filter[strings.ToLower(value)]
	return has
}

func matchesCapabilities(values []string, filter map[string]struct{}) bool {
	if len(filter) == 0 {
		return true
	}

	for _, capability := range values {
		if _, has := filter[strings.ToLower(capability)]; has {
			return true
		}
	}

	return false
}

func toAiModelVersion(model *armcognitiveservices.Model) (versionByKey, bool) {
	version := safeModelVersion(model)
	kind := safeModelKind(model)
	format := safeModelFormat(model)
	status := safeModelLifecycleStatus(model)
	if version == "" {
		return versionByKey{}, false
	}

	capabilities := safeModelCapabilities(model)
	skus := safeModelSkus(model)

	key := strings.Join([]string{
		version,
		kind,
		format,
		status,
	}, "|")

	return versionByKey{
		key: key,
		version: AiModelVersion{
			Version:          version,
			IsDefaultVersion: safeModelIsDefaultVersion(model),
			Kind:             kind,
			Format:           format,
			Status:           status,
			Capabilities:     capabilities,
			Skus:             skus,
		},
	}, true
}

func mergeModelSkus(existing []AiModelSku, incoming []AiModelSku) []AiModelSku {
	if len(incoming) == 0 {
		return existing
	}

	index := make(map[string]int, len(existing))
	for i, sku := range existing {
		index[strings.ToLower(sku.Name)+"|"+strings.ToLower(sku.UsageName)] = i
	}

	merged := slices.Clone(existing)
	for _, sku := range incoming {
		key := strings.ToLower(sku.Name) + "|" + strings.ToLower(sku.UsageName)
		if _, has := index[key]; has {
			continue
		}

		merged = append(merged, sku)
		index[key] = len(merged) - 1
	}

	return merged
}

func mergeModelCapabilities(existing []string, incoming []string) []string {
	if len(incoming) == 0 {
		return existing
	}

	index := make(map[string]int, len(existing))
	for i, capability := range existing {
		index[strings.ToLower(capability)] = i
	}

	merged := slices.Clone(existing)
	for _, capability := range incoming {
		trimmed := strings.TrimSpace(capability)
		if trimmed == "" {
			continue
		}

		key := strings.ToLower(trimmed)
		if _, has := index[key]; has {
			continue
		}

		merged = append(merged, trimmed)
		index[key] = len(merged) - 1
	}

	slices.Sort(merged)
	return merged
}

func sortModelSkus(skus []AiModelSku) {
	slices.SortFunc(skus, func(a, b AiModelSku) int {
		nameCompare := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		if nameCompare != 0 {
			return nameCompare
		}

		return strings.Compare(strings.ToLower(a.UsageName), strings.ToLower(b.UsageName))
	})
}

func safeUsageName(usage *armcognitiveservices.Usage) string {
	if usage == nil || usage.Name == nil || usage.Name.Value == nil {
		return ""
	}
	return *usage.Name.Value
}

func safeUsageCurrentValue(usage *armcognitiveservices.Usage) float64 {
	if usage == nil || usage.CurrentValue == nil {
		return 0
	}
	return float64(*usage.CurrentValue)
}

func safeUsageLimit(usage *armcognitiveservices.Usage) float64 {
	if usage == nil || usage.Limit == nil {
		return 0
	}
	return float64(*usage.Limit)
}

func safeUsageUnit(usage *armcognitiveservices.Usage) string {
	if usage == nil || usage.Unit == nil {
		return ""
	}
	return string(*usage.Unit)
}

func safeModelName(model *armcognitiveservices.Model) string {
	if model == nil || model.Model == nil || model.Model.Name == nil {
		return ""
	}
	return *model.Model.Name
}

func safeModelVersion(model *armcognitiveservices.Model) string {
	if model == nil || model.Model == nil || model.Model.Version == nil {
		return ""
	}
	return *model.Model.Version
}

func safeModelKind(model *armcognitiveservices.Model) string {
	if model == nil || model.Kind == nil {
		return ""
	}
	return *model.Kind
}

func safeModelFormat(model *armcognitiveservices.Model) string {
	if model == nil || model.Model == nil || model.Model.Format == nil {
		return ""
	}
	return *model.Model.Format
}

func safeModelLifecycleStatus(model *armcognitiveservices.Model) string {
	if model == nil || model.Model == nil || model.Model.LifecycleStatus == nil {
		return ""
	}
	return string(*model.Model.LifecycleStatus)
}

func safeModelIsDefaultVersion(model *armcognitiveservices.Model) bool {
	if model == nil || model.Model == nil || model.Model.IsDefaultVersion == nil {
		return false
	}
	return *model.Model.IsDefaultVersion
}

func safeModelCapabilities(model *armcognitiveservices.Model) []string {
	if model == nil || model.Model == nil || len(model.Model.Capabilities) == 0 {
		return nil
	}

	keys := make([]string, 0, len(model.Model.Capabilities))
	for key := range model.Model.Capabilities {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func safeModelSkus(model *armcognitiveservices.Model) []AiModelSku {
	if model == nil || model.Model == nil || len(model.Model.SKUs) == 0 {
		return nil
	}

	skus := make([]AiModelSku, 0, len(model.Model.SKUs))
	for _, sku := range model.Model.SKUs {
		if sku == nil || sku.Name == nil {
			continue
		}

		usageName := ""
		if sku.UsageName != nil {
			usageName = *sku.UsageName
		}

		capDefault := int32(0)
		capMin := int32(0)
		capMax := int32(0)
		capStep := int32(0)
		if sku.Capacity != nil {
			if sku.Capacity.Default != nil {
				capDefault = *sku.Capacity.Default
			}
			if sku.Capacity.Minimum != nil {
				capMin = *sku.Capacity.Minimum
			}
			if sku.Capacity.Maximum != nil {
				capMax = *sku.Capacity.Maximum
			}
			if sku.Capacity.Step != nil {
				capStep = *sku.Capacity.Step
			}
		}

		skus = append(skus, AiModelSku{
			Name:            *sku.Name,
			UsageName:       usageName,
			CapacityDefault: capDefault,
			CapacityMinimum: capMin,
			CapacityMaximum: capMax,
			CapacityStep:    capStep,
		})
	}

	return skus
}
