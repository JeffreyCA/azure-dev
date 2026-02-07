// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/ai"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/prompt"
	"github.com/azure/azure-dev/cli/azd/pkg/ux"
)

type promptService struct {
	azdext.UnimplementedPromptServiceServer
	prompter        prompt.PromptService
	resourceService *azapi.ResourceService
	aiModelService  *ai.AiModelService
	globalOptions   *internal.GlobalCommandOptions
	lock            *promptLock
}

func NewPromptService(
	prompter prompt.PromptService,
	resourceService *azapi.ResourceService,
	aiModelService *ai.AiModelService,
	globalOptions *internal.GlobalCommandOptions,
) azdext.PromptServiceServer {
	return &promptService{
		prompter:        prompter,
		resourceService: resourceService,
		aiModelService:  aiModelService,
		globalOptions:   globalOptions,
		lock:            newPromptLock(),
	}
}

func (s *promptService) Confirm(ctx context.Context, req *azdext.ConfirmRequest) (*azdext.ConfirmResponse, error) {
	if s.globalOptions.NoPrompt {
		if req.Options.DefaultValue == nil {
			return nil, fmt.Errorf("no default response for prompt '%s'", req.Options.Message)
		} else {
			return &azdext.ConfirmResponse{
				Value: req.Options.DefaultValue,
			}, nil
		}
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	options := &ux.ConfirmOptions{
		DefaultValue: req.Options.DefaultValue,
		Message:      req.Options.Message,
		HelpMessage:  req.Options.HelpMessage,
		Hint:         req.Options.Hint,
		PlaceHolder:  req.Options.Placeholder,
	}

	confirm := ux.NewConfirm(options)
	value, err := confirm.Ask(ctx)

	return &azdext.ConfirmResponse{
		Value: value,
	}, err
}

func (s *promptService) Select(ctx context.Context, req *azdext.SelectRequest) (*azdext.SelectResponse, error) {
	if s.globalOptions.NoPrompt {
		if req.Options.SelectedIndex == nil {
			return nil, fmt.Errorf("no default selection for prompt '%s'", req.Options.Message)
		} else {
			return &azdext.SelectResponse{
				Value: req.Options.SelectedIndex,
			}, nil
		}
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	choices := make([]*ux.SelectChoice, len(req.Options.Choices))
	for i, choice := range req.Options.Choices {
		choices[i] = &ux.SelectChoice{
			Value: choice.Value,
			Label: choice.Label,
		}
	}

	options := &ux.SelectOptions{
		SelectedIndex:   convertToInt(req.Options.SelectedIndex),
		Message:         req.Options.Message,
		Choices:         choices,
		HelpMessage:     req.Options.HelpMessage,
		DisplayCount:    int(req.Options.DisplayCount),
		DisplayNumbers:  req.Options.DisplayNumbers,
		EnableFiltering: req.Options.EnableFiltering,
	}

	selectPrompt := ux.NewSelect(options)
	value, err := selectPrompt.Ask(ctx)

	return &azdext.SelectResponse{
		Value: convertToInt32(value),
	}, err
}

func (s *promptService) MultiSelect(
	ctx context.Context,
	req *azdext.MultiSelectRequest,
) (*azdext.MultiSelectResponse, error) {
	if s.globalOptions.NoPrompt {
		var selectedChoices []*azdext.MultiSelectChoice
		for _, choice := range req.Options.Choices {
			if choice.Selected {
				selectedChoices = append(selectedChoices, choice)
			}
		}

		return &azdext.MultiSelectResponse{
			Values: selectedChoices,
		}, nil
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	choices := make([]*ux.MultiSelectChoice, len(req.Options.Choices))
	for i, choice := range req.Options.Choices {
		choices[i] = &ux.MultiSelectChoice{
			Value:    choice.Value,
			Label:    choice.Label,
			Selected: choice.Selected,
		}
	}

	options := &ux.MultiSelectOptions{
		Message:         req.Options.Message,
		Choices:         choices,
		HelpMessage:     req.Options.HelpMessage,
		DisplayCount:    int(req.Options.DisplayCount),
		DisplayNumbers:  req.Options.DisplayNumbers,
		EnableFiltering: req.Options.EnableFiltering,
	}

	selectPrompt := ux.NewMultiSelect(options)
	values, err := selectPrompt.Ask(ctx)

	resultValues := make([]*azdext.MultiSelectChoice, len(values))
	for i, value := range values {
		resultValues[i] = &azdext.MultiSelectChoice{
			Value:    value.Value,
			Label:    value.Label,
			Selected: value.Selected,
		}
	}

	return &azdext.MultiSelectResponse{
		Values: resultValues,
	}, err
}

func (s *promptService) Prompt(ctx context.Context, req *azdext.PromptRequest) (*azdext.PromptResponse, error) {
	if s.globalOptions.NoPrompt {
		if req.Options.Required && req.Options.DefaultValue == "" {
			return nil, fmt.Errorf("no default response for prompt '%s'", req.Options.Message)
		} else {
			return &azdext.PromptResponse{
				Value: req.Options.DefaultValue,
			}, nil
		}
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	options := &ux.PromptOptions{
		DefaultValue:      req.Options.DefaultValue,
		Message:           req.Options.Message,
		HelpMessage:       req.Options.HelpMessage,
		Hint:              req.Options.Hint,
		PlaceHolder:       req.Options.Placeholder,
		ValidationMessage: req.Options.ValidationMessage,
		RequiredMessage:   req.Options.RequiredMessage,
		Required:          req.Options.Required,
		ClearOnCompletion: req.Options.ClearOnCompletion,
		IgnoreHintKeys:    req.Options.IgnoreHintKeys,
	}

	prompt := ux.NewPrompt(options)
	value, err := prompt.Ask(ctx)

	return &azdext.PromptResponse{
		Value: value,
	}, err
}

func (s *promptService) PromptSubscription(
	ctx context.Context,
	req *azdext.PromptSubscriptionRequest,
) (*azdext.PromptSubscriptionResponse, error) {
	// Delegate to prompt service which handles --no-prompt mode
	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	selectedSubscription, err := s.prompter.PromptSubscription(ctx, &prompt.SelectOptions{
		Message:     req.Message,
		HelpMessage: req.HelpMessage,
	})
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	subscription := &azdext.Subscription{
		Id:           selectedSubscription.Id,
		Name:         selectedSubscription.Name,
		TenantId:     selectedSubscription.TenantId,
		UserTenantId: selectedSubscription.UserAccessTenantId,
		IsDefault:    selectedSubscription.IsDefault,
	}

	return &azdext.PromptSubscriptionResponse{
		Subscription: subscription,
	}, nil
}

func (s *promptService) PromptLocation(
	ctx context.Context,
	req *azdext.PromptLocationRequest,
) (*azdext.PromptLocationResponse, error) {
	// Delegate to prompt service which handles --no-prompt mode
	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	selectedLocation, err := s.prompter.PromptLocation(ctx, azureContext, nil)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	location := &azdext.Location{
		Name:                selectedLocation.Name,
		DisplayName:         selectedLocation.DisplayName,
		RegionalDisplayName: selectedLocation.RegionalDisplayName,
	}

	return &azdext.PromptLocationResponse{
		Location: location,
	}, nil
}

func (s *promptService) PromptResourceGroup(
	ctx context.Context,
	req *azdext.PromptResourceGroupRequest,
) (*azdext.PromptResourceGroupResponse, error) {
	// Delegate to prompt service which handles --no-prompt mode
	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	options := createResourceGroupOptions(req.Options)

	selectedResourceGroup, err := s.prompter.PromptResourceGroup(ctx, azureContext, options)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	resourceGroup := &azdext.ResourceGroup{
		Id:       selectedResourceGroup.Id,
		Name:     selectedResourceGroup.Name,
		Location: selectedResourceGroup.Location,
	}

	return &azdext.PromptResourceGroupResponse{
		ResourceGroup: resourceGroup,
	}, nil
}

func (s *promptService) PromptSubscriptionResource(
	ctx context.Context,
	req *azdext.PromptSubscriptionResourceRequest,
) (*azdext.PromptSubscriptionResourceResponse, error) {
	// Delegate to prompt service which handles --no-prompt mode
	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	options := createResourceOptions(req.Options)

	resource, err := s.prompter.PromptSubscriptionResource(ctx, azureContext, options)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	return &azdext.PromptSubscriptionResourceResponse{
		Resource: &azdext.ResourceExtended{
			Id:       resource.Id,
			Name:     resource.Name,
			Type:     resource.Type,
			Location: resource.Location,
			Kind:     resource.Kind,
		},
	}, nil
}

func (s *promptService) PromptResourceGroupResource(
	ctx context.Context,
	req *azdext.PromptResourceGroupResourceRequest,
) (*azdext.PromptResourceGroupResourceResponse, error) {
	// Delegate to prompt service which handles --no-prompt mode
	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	options := createResourceOptions(req.Options)

	resource, err := s.prompter.PromptResourceGroupResource(ctx, azureContext, options)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	return &azdext.PromptResourceGroupResourceResponse{
		Resource: &azdext.ResourceExtended{
			Id:       resource.Id,
			Name:     resource.Name,
			Type:     resource.Type,
			Location: resource.Location,
			Kind:     resource.Kind,
		},
	}, nil
}

func (s *promptService) createAzureContext(wire *azdext.AzureContext) (*prompt.AzureContext, error) {
	scope := prompt.AzureScope{
		TenantId:       wire.Scope.TenantId,
		SubscriptionId: wire.Scope.SubscriptionId,
		Location:       wire.Scope.Location,
		ResourceGroup:  wire.Scope.ResourceGroup,
	}

	resources := []*arm.ResourceID{}
	for _, resourceId := range wire.Resources {
		parsedResource, err := arm.ParseResourceID(resourceId)
		if err != nil {
			return nil, err
		}

		resources = append(resources, parsedResource)
	}

	resourceList := prompt.NewAzureResourceList(s.resourceService, resources)

	return prompt.NewAzureContext(s.prompter, scope, resourceList), nil
}

func createResourceOptions(options *azdext.PromptResourceOptions) prompt.ResourceOptions {
	if options == nil {
		return prompt.ResourceOptions{}
	}

	var resourceType *azapi.AzureResourceType
	if options.ResourceType != "" {
		resourceType = to.Ptr(azapi.AzureResourceType(options.ResourceType))
	}

	var selectOptions *prompt.SelectOptions

	if options.SelectOptions != nil {
		selectOptions = &prompt.SelectOptions{
			ForceNewResource:   options.SelectOptions.ForceNewResource,
			NewResourceMessage: options.SelectOptions.NewResourceMessage,
			Message:            options.SelectOptions.Message,
			HelpMessage:        options.SelectOptions.HelpMessage,
			LoadingMessage:     options.SelectOptions.LoadingMessage,
			DisplayCount:       int(options.SelectOptions.DisplayCount),
			DisplayNumbers:     options.SelectOptions.DisplayNumbers,
			AllowNewResource:   options.SelectOptions.AllowNewResource,
			Hint:               options.SelectOptions.Hint,
			EnableFiltering:    options.SelectOptions.EnableFiltering,
		}
	}

	resourceOptions := prompt.ResourceOptions{
		ResourceType:            resourceType,
		Kinds:                   options.Kinds,
		ResourceTypeDisplayName: options.ResourceTypeDisplayName,
		SelectorOptions:         selectOptions,
	}

	return resourceOptions
}

func createResourceGroupOptions(options *azdext.PromptResourceGroupOptions) *prompt.ResourceGroupOptions {
	if options == nil || options.SelectOptions == nil {
		return nil
	}

	return &prompt.ResourceGroupOptions{
		SelectorOptions: &prompt.SelectOptions{
			ForceNewResource:   options.SelectOptions.ForceNewResource,
			AllowNewResource:   options.SelectOptions.AllowNewResource,
			NewResourceMessage: options.SelectOptions.NewResourceMessage,
			Message:            options.SelectOptions.Message,
			HelpMessage:        options.SelectOptions.HelpMessage,
			LoadingMessage:     options.SelectOptions.LoadingMessage,
			DisplayCount:       int(options.SelectOptions.DisplayCount),
			DisplayNumbers:     options.SelectOptions.DisplayNumbers,
			Hint:               options.SelectOptions.Hint,
			EnableFiltering:    options.SelectOptions.EnableFiltering,
		},
	}
}

func convertToInt32(input *int) *int32 {
	if input == nil {
		return nil // Handle the nil case
	}

	// nolint:gosec // G115
	value := int32(*input) // Convert the dereferenced value to int32
	return &value          // Return the address of the new int32 value
}

func convertToInt(input *int32) *int {
	if input == nil {
		return nil // Handle the nil case
	}
	value := int(*input) // Convert the dereferenced value to int
	return &value        // Return the address of the new int value
}

// --- AI Model Prompt Methods ---

func (s *promptService) PromptAiModel(
	ctx context.Context, req *azdext.PromptAiModelRequest,
) (*azdext.PromptAiModelResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)

	var filterOpts *ai.FilterOptions
	var locations []string
	if req.Filter != nil {
		filterOpts = protoToFilterOptions(req.Filter)
		locations = filterOpts.Locations
	}
	if len(locations) == 0 && scopeLocation != "" {
		locations = []string{scopeLocation}
	}

	models, err := s.aiModelService.ListModels(ctx, subscriptionId, locations)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	if filterOpts != nil {
		models = ai.FilterModels(models, filterOpts)
	}

	// Quota-aware filtering requires exactly one location for usage data.
	var usageMap map[string]ai.AiModelUsage
	if req.Quota != nil {
		if len(locations) != 1 {
			return nil, fmt.Errorf(
				"quota checking requires exactly one effective location, got %d", len(locations))
		}
		usages, err := s.aiModelService.ListUsages(ctx, subscriptionId, locations[0])
		if err != nil {
			return nil, fmt.Errorf("listing usages for quota check: %w", err)
		}
		usageMap = make(map[string]ai.AiModelUsage, len(usages))
		for _, u := range usages {
			usageMap[u.Name] = u
		}
		models = ai.FilterModelsByQuota(models, usages, req.Quota.MinRemainingCapacity)
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found matching the specified criteria")
	}

	if s.globalOptions.NoPrompt {
		return nil, fmt.Errorf("cannot prompt for model selection in non-interactive mode")
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	message := "Select an AI model"
	if req.SelectOptions != nil && req.SelectOptions.Message != "" {
		message = req.SelectOptions.Message
	}

	selectOpts := &ux.SelectOptions{
		Message:         message,
		Choices:         make([]*ux.SelectChoice, len(models)),
		EnableFiltering: to.Ptr(true),
	}
	for i, m := range models {
		label := m.Name
		if req.Quota != nil && usageMap != nil {
			label += " " + modelQuotaSummary(m, usageMap)
		}
		selectOpts.Choices[i] = &ux.SelectChoice{
			Value: m.Name,
			Label: label,
		}
	}

	selected, err := ux.NewSelect(selectOpts).Ask(ctx)
	if err != nil {
		return nil, fmt.Errorf("prompting for model selection: %w", err)
	}

	return &azdext.PromptAiModelResponse{
		Model: aiModelToProto(&models[*selected]),
	}, nil
}

func (s *promptService) PromptAiModelDeployment(
	ctx context.Context, req *azdext.PromptAiModelDeploymentRequest,
) (*azdext.PromptAiModelDeploymentResponse, error) {
	subscriptionId, scopeLocation := extractScope(req.AzureContext)

	options := protoToDeploymentOptions(req.Options)
	if options == nil {
		options = &ai.DeploymentOptions{}
	}
	if len(options.Locations) == 0 && scopeLocation != "" {
		options.Locations = []string{scopeLocation}
	}

	// Fail explicitly if quota is requested without exactly one location.
	if req.Quota != nil && len(options.Locations) != 1 {
		return nil, fmt.Errorf(
			"quota checking requires exactly one effective location, got %d", len(options.Locations))
	}

	// Fetch the model catalog
	models, err := s.aiModelService.ListModels(ctx, subscriptionId, options.Locations)
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	var targetModel *ai.AiModel
	for i := range models {
		if models[i].Name == req.ModelName {
			targetModel = &models[i]
			break
		}
	}
	if targetModel == nil {
		return nil, fmt.Errorf("model %q not found", req.ModelName)
	}

	// Fetch quota data (guaranteed single location by check above)
	var usageMap map[string]ai.AiModelUsage
	if req.Quota != nil {
		usages, err := s.aiModelService.ListUsages(ctx, subscriptionId, options.Locations[0])
		if err != nil {
			return nil, fmt.Errorf("getting usages: %w", err)
		}
		usageMap = make(map[string]ai.AiModelUsage, len(usages))
		for _, u := range usages {
			usageMap[u.Name] = u
		}
	}

	if s.globalOptions.NoPrompt {
		return nil, fmt.Errorf("cannot prompt for deployment configuration in non-interactive mode")
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// --- Step 1: Select version ---
	// Collect available versions (filtered by options.versions if provided).
	var availableVersions []ai.AiModelVersion
	for _, v := range targetModel.Versions {
		if len(options.Versions) > 0 && !slices.Contains(options.Versions, v.Version) {
			continue
		}
		availableVersions = append(availableVersions, v)
	}
	if len(availableVersions) == 0 {
		return nil, fmt.Errorf("no versions available for model %q with the specified options", req.ModelName)
	}

	var selectedVersion ai.AiModelVersion
	selectedVersionChosen := false
	if req.UseDefaultVersion {
		for _, v := range availableVersions {
			if v.IsDefault {
				selectedVersion = v
				selectedVersionChosen = true
				break
			}
		}
	}

	if !selectedVersionChosen && len(availableVersions) == 1 {
		selectedVersion = availableVersions[0]
		selectedVersionChosen = true
	}

	if !selectedVersionChosen {
		versionChoices := make([]*ux.SelectChoice, len(availableVersions))
		for i, v := range availableVersions {
			label := v.Version
			if v.IsDefault {
				label += " (default)"
			}
			versionChoices[i] = &ux.SelectChoice{Value: v.Version, Label: label}
		}
		vIdx, err := ux.NewSelect(&ux.SelectOptions{
			Message:         fmt.Sprintf("Select a version for %s", req.ModelName),
			Choices:         versionChoices,
			EnableFiltering: to.Ptr(true),
		}).Ask(ctx)
		if err != nil {
			return nil, fmt.Errorf("prompting for version: %w", err)
		}
		selectedVersion = availableVersions[*vIdx]
	}

	// --- Step 2: Select SKU ---
	// Collect SKUs for the selected version, filtered by options and quota.
	type skuCandidate struct {
		sku       ai.AiModelSku
		remaining *float64
		label     string
	}
	var skuCandidates []skuCandidate

	for _, sku := range selectedVersion.Skus {
		if len(options.Skus) > 0 && !slices.Contains(options.Skus, sku.Name) {
			continue
		}

		// Exclude fine-tune SKUs by default — they only apply to fine-tuned model deployments.
		if !req.IncludeFinetuneSkus && strings.HasSuffix(sku.UsageName, "-finetune") {
			continue
		}

		capacity := ai.ResolveCapacity(sku, options.Capacity)

		var remaining *float64
		if req.Quota != nil && usageMap != nil {
			usage, ok := usageMap[sku.UsageName]
			if !ok {
				continue
			}

			rem := usage.Limit - usage.CurrentValue
			remaining = &rem
			minReq := req.Quota.MinRemainingCapacity
			if minReq <= 0 {
				minReq = 1
			}
			if rem < minReq || (capacity > 0 && float64(capacity) > rem) {
				continue
			}
		}

		skuCandidates = append(skuCandidates, skuCandidate{sku: sku, remaining: remaining})
	}

	if len(skuCandidates) == 0 {
		return nil, fmt.Errorf("no valid SKUs found for model %q version %q", req.ModelName, selectedVersion.Version)
	}

	// Build labels: only include usage_name when SKU names are ambiguous.
	skuNameCount := make(map[string]int, len(skuCandidates))
	for _, c := range skuCandidates {
		skuNameCount[c.sku.Name]++
	}
	for i, c := range skuCandidates {
		label := c.sku.Name
		if skuNameCount[c.sku.Name] > 1 {
			label += fmt.Sprintf(" (%s)", c.sku.UsageName)
		}
		if c.remaining != nil {
			label += " " + output.WithGrayFormat("[%.0f quota remaining]", *c.remaining)
		}
		skuCandidates[i].label = label
	}

	selectedSku := skuCandidates[0]
	if len(skuCandidates) > 1 {
		skuChoices := make([]*ux.SelectChoice, len(skuCandidates))
		for i, c := range skuCandidates {
			skuChoices[i] = &ux.SelectChoice{Value: c.label, Label: c.label}
		}
		sIdx, err := ux.NewSelect(&ux.SelectOptions{
			Message:         fmt.Sprintf("Select a SKU for %s v%s", req.ModelName, selectedVersion.Version),
			Choices:         skuChoices,
			EnableFiltering: to.Ptr(true),
		}).Ask(ctx)
		if err != nil {
			return nil, fmt.Errorf("prompting for SKU: %w", err)
		}
		selectedSku = skuCandidates[*sIdx]
	}

	// --- Step 3: Resolve capacity, optionally prompting ---
	capacity := ai.ResolveCapacity(selectedSku.sku, options.Capacity)

	if !req.UseDefaultCapacity {
		sku := selectedSku.sku
		defaultVal := fmt.Sprintf("%d", capacity)
		if capacity == 0 && sku.DefaultCapacity > 0 {
			defaultVal = fmt.Sprintf("%d", sku.DefaultCapacity)
		}

		hint := ""
		if sku.MinCapacity > 0 || sku.MaxCapacity > 0 {
			hint = fmt.Sprintf("min: %d, max: %d, step: %d", sku.MinCapacity, sku.MaxCapacity, sku.CapacityStep)
		}

		prompt := ux.NewPrompt(&ux.PromptOptions{
			Message:      fmt.Sprintf("Enter deployment capacity for %s (%s)", req.ModelName, sku.Name),
			DefaultValue: defaultVal,
			HelpMessage:  hint,
			Required:     true,
			ValidationFn: func(value string) (bool, string) {
				_, err := validateDeploymentCapacity(value, sku)
				if err != nil {
					return false, err.Error()
				}

				return true, ""
			},
		})
		capStr, err := prompt.Ask(ctx)
		if err != nil {
			return nil, fmt.Errorf("prompting for capacity: %w", err)
		}

		parsed, err := validateDeploymentCapacity(capStr, sku)
		if err != nil {
			return nil, fmt.Errorf("invalid capacity %q: %w", capStr, err)
		}
		capacity = parsed
	}

	deployLocation := ""
	if len(options.Locations) == 1 {
		deployLocation = options.Locations[0]
	}

	deployment := &ai.AiModelDeployment{
		ModelName:      req.ModelName,
		Format:         targetModel.Format,
		Version:        selectedVersion.Version,
		Location:       deployLocation,
		Sku:            selectedSku.sku,
		Capacity:       capacity,
		RemainingQuota: selectedSku.remaining,
	}

	return &azdext.PromptAiModelDeploymentResponse{
		Deployment: aiModelDeploymentToProto(deployment),
	}, nil
}

func (s *promptService) PromptAiLocationWithQuota(
	ctx context.Context, req *azdext.PromptAiLocationWithQuotaRequest,
) (*azdext.PromptAiLocationWithQuotaResponse, error) {
	subscriptionId, _ := extractScope(req.AzureContext)

	requirements := make([]ai.QuotaRequirement, len(req.Requirements))
	for i, r := range req.Requirements {
		requirements[i] = ai.QuotaRequirement{
			UsageName:   r.UsageName,
			MinCapacity: r.MinCapacity,
		}
	}

	locations, err := s.aiModelService.ListLocationsWithQuota(
		ctx, subscriptionId, req.AllowedLocations, requirements)
	if err != nil {
		return nil, fmt.Errorf("listing locations with quota: %w", err)
	}

	if len(locations) == 0 {
		return nil, fmt.Errorf("no locations found with sufficient quota")
	}

	if s.globalOptions.NoPrompt {
		return nil, fmt.Errorf("cannot prompt for location selection in non-interactive mode")
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	message := "Select a location"
	if req.SelectOptions != nil && req.SelectOptions.Message != "" {
		message = req.SelectOptions.Message
	}

	selectOpts := &ux.SelectOptions{
		Message:         message,
		Choices:         make([]*ux.SelectChoice, len(locations)),
		EnableFiltering: to.Ptr(true),
	}
	for i, loc := range locations {
		selectOpts.Choices[i] = &ux.SelectChoice{
			Value: loc,
			Label: loc,
		}
	}

	selected, err := ux.NewSelect(selectOpts).Ask(ctx)
	if err != nil {
		return nil, fmt.Errorf("prompting for location selection: %w", err)
	}

	return &azdext.PromptAiLocationWithQuotaResponse{
		Location: &azdext.Location{Name: locations[*selected]},
	}, nil
}

// modelQuotaSummary builds a gray-formatted quota summary for a model's SKUs.
// Shows the max remaining quota across all SKUs, e.g. "[1000 quota remaining]".
func modelQuotaSummary(model ai.AiModel, usageMap map[string]ai.AiModelUsage) string {
	var maxRemaining float64
	found := false
	for _, v := range model.Versions {
		for _, sku := range v.Skus {
			if usage, ok := usageMap[sku.UsageName]; ok {
				rem := usage.Limit - usage.CurrentValue
				if !found || rem > maxRemaining {
					maxRemaining = rem
					found = true
				}
			}
		}
	}
	if !found {
		return output.WithGrayFormat("[no quota info]")
	}
	return output.WithGrayFormat("[%.0f quota remaining]", maxRemaining)
}

func validateDeploymentCapacity(value string, sku ai.AiModelSku) (int32, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("capacity must be a whole number")
	}

	capacity := int32(parsed)

	if sku.MinCapacity > 0 && capacity < sku.MinCapacity {
		return 0, fmt.Errorf("capacity must be at least %d", sku.MinCapacity)
	}

	if sku.MaxCapacity > 0 && capacity > sku.MaxCapacity {
		return 0, fmt.Errorf("capacity must be at most %d", sku.MaxCapacity)
	}

	if sku.CapacityStep > 0 && capacity%sku.CapacityStep != 0 {
		return 0, fmt.Errorf("capacity must be a multiple of %d", sku.CapacityStep)
	}

	return capacity, nil
}

// promptLock is a context-aware mutual exclusion mechanism for serializing interactive prompts.
// It prevents concurrent prompt access which could cause prompts to freeze up when multiple
// extensions with "listen" capability are installed and running simultaneously.
type promptLock struct {
	ch chan struct{}
}

// newPromptLock creates a new promptLock instance.
func newPromptLock() *promptLock {
	return &promptLock{ch: make(chan struct{}, 1)}
}

// acquirePromptLock acquires the prompt lock, blocking until available or context is cancelled.
// Returns a release function that must be called to release the lock (typically via defer).
// Returns an error if the context is cancelled while waiting for the lock.
func (s *promptService) acquirePromptLock(ctx context.Context) (func(), error) {
	select {
	case s.lock.ch <- struct{}{}:
		return func() {
			<-s.lock.ch
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// wrapErrorWithSuggestion checks if the error contains an ErrorWithSuggestion and if so,
// returns a new error that includes the suggestion text in the error message.
// This ensures that helpful suggestions (like "run azd auth login") are preserved
// when errors are transmitted over gRPC, where only the error message string is sent.
func wrapErrorWithSuggestion(err error) error {
	if err == nil {
		return nil
	}

	var suggestionErr *internal.ErrorWithSuggestion
	if errors.As(err, &suggestionErr) {
		return fmt.Errorf("%w\n%s", err, suggestionErr.Suggestion)
	}

	return err
}
