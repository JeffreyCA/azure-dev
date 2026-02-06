// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/prompt"
	"github.com/azure/azure-dev/cli/azd/pkg/ux"
)

type promptService struct {
	azdext.UnimplementedPromptServiceServer
	prompter        prompt.PromptService
	aiClient        aiCatalogClient
	resourceService *azapi.ResourceService
	globalOptions   *internal.GlobalCommandOptions
	lock            *promptLock
}

func NewPromptService(
	prompter prompt.PromptService,
	aiClient *azapi.AzureClient,
	resourceService *azapi.ResourceService,
	globalOptions *internal.GlobalCommandOptions,
) azdext.PromptServiceServer {
	return &promptService{
		prompter:        prompter,
		aiClient:        aiClient,
		resourceService: resourceService,
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

func (s *promptService) PromptAiLocation(
	ctx context.Context,
	req *azdext.PromptAiLocationRequest,
) (*azdext.PromptAiLocationResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	if azureContext.Scope.SubscriptionId == "" {
		return nil, fmt.Errorf("azure context must include subscription_id")
	}

	requirements := make([]azapi.AiUsageRequirement, 0, len(req.Requirements))
	for _, requirement := range req.Requirements {
		requiredCapacity := float64(requirement.GetRequiredCapacity())
		if requiredCapacity <= 0 {
			requiredCapacity = 1
		}

		requirements = append(requirements, azapi.AiUsageRequirement{
			UsageName:        requirement.GetUsageName(),
			RequiredCapacity: requiredCapacity,
		})
	}

	locationsWithQuota, err := s.aiClient.FindAiLocationsWithQuota(
		ctx,
		azureContext.Scope.SubscriptionId,
		req.GetAllowedLocations(),
		requirements,
		nil,
	)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	if len(locationsWithQuota.MatchedLocations) == 0 {
		return nil, fmt.Errorf("no AI locations found that satisfy the requested quota")
	}

	if s.globalOptions.NoPrompt {
		currentLocation := azureContext.Scope.Location
		if currentLocation == "" {
			return nil, fmt.Errorf("no location in azure context for --no-prompt mode")
		}

		selectedLocation, has := findCaseInsensitive(locationsWithQuota.MatchedLocations, currentLocation)
		if !has {
			return nil, fmt.Errorf(
				"azure context location '%s' does not satisfy the requested AI quota",
				currentLocation,
			)
		}

		return &azdext.PromptAiLocationResponse{
			Location: &azdext.Location{
				Name:                selectedLocation,
				DisplayName:         selectedLocation,
				RegionalDisplayName: selectedLocation,
			},
		}, nil
	}

	if len(locationsWithQuota.MatchedLocations) == 1 {
		selectedLocation := locationsWithQuota.MatchedLocations[0]
		return &azdext.PromptAiLocationResponse{
			Location: &azdext.Location{
				Name:                selectedLocation,
				DisplayName:         selectedLocation,
				RegionalDisplayName: selectedLocation,
			},
		}, nil
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	choices := make([]*ux.SelectChoice, 0, len(locationsWithQuota.MatchedLocations))
	for _, location := range locationsWithQuota.MatchedLocations {
		choices = append(choices, &ux.SelectChoice{
			Value: location,
			Label: location,
		})
	}

	message := req.GetMessage()
	if message == "" {
		message = "Select an Azure location for AI deployments:"
	}

	selectPrompt := ux.NewSelect(&ux.SelectOptions{
		Message:         message,
		HelpMessage:     req.GetHelpMessage(),
		Choices:         choices,
		EnableFiltering: to.Ptr(true),
		DisplayCount:    min(12, len(choices)),
	})

	selectedIndex, err := selectPrompt.Ask(ctx)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}
	if selectedIndex == nil {
		return nil, fmt.Errorf("no AI location selected")
	}

	selectedLocation := locationsWithQuota.MatchedLocations[*selectedIndex]
	return &azdext.PromptAiLocationResponse{
		Location: &azdext.Location{
			Name:                selectedLocation,
			DisplayName:         selectedLocation,
			RegionalDisplayName: selectedLocation,
		},
	}, nil
}

func (s *promptService) PromptAiModel(
	ctx context.Context,
	req *azdext.PromptAiModelRequest,
) (*azdext.PromptAiModelResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	azureContext, err := s.createAzureContext(req.AzureContext)
	if err != nil {
		return nil, err
	}

	if azureContext.Scope.SubscriptionId == "" {
		return nil, fmt.Errorf("azure context must include subscription_id")
	}

	location := req.GetLocation()
	if location == "" {
		location = azureContext.Scope.Location
	}

	if location == "" {
		return nil, fmt.Errorf("location is required for AI model selection")
	}

	modelCatalog, err := s.aiClient.ListAiModelCatalog(
		ctx,
		azureContext.Scope.SubscriptionId,
		azapi.AiModelCatalogFilters{
			Locations:    []string{location},
			Kinds:        req.GetKinds(),
			Formats:      req.GetFormats(),
			Statuses:     req.GetStatuses(),
			Capabilities: req.GetCapabilities(),
		},
	)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	candidates := flattenAiModelCandidates(modelCatalog, location)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no AI models found matching the provided filters")
	}

	preferredSkus := req.GetPreferredSkus()
	if s.globalOptions.NoPrompt {
		selected, err := chooseDeterministicAiModel(candidates, preferredSkus)
		if err != nil {
			return nil, err
		}

		return &azdext.PromptAiModelResponse{
			Model: selected.toProto(),
		}, nil
	}

	sortAiModelCandidates(candidates, preferredSkus)
	if len(candidates) == 1 {
		return &azdext.PromptAiModelResponse{
			Model: candidates[0].toProto(),
		}, nil
	}

	selectedIndex, err := s.promptAiModelCandidateSelection(ctx, candidates, preferredSkus, req.GetMessage(), req.GetHelpMessage())
	if err != nil {
		return nil, err
	}

	return &azdext.PromptAiModelResponse{
		Model: candidates[selectedIndex].toProto(),
	}, nil
}

func (s *promptService) PromptAiDeployment(
	ctx context.Context,
	req *azdext.PromptAiDeploymentRequest,
) (*azdext.PromptAiDeploymentResponse, error) {
	selectionMode := req.GetSelectionMode()
	switch selectionMode {
	case azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_UNSPECIFIED,
		azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_LOCATION_FIRST:
		return s.promptAiDeploymentLocationFirst(ctx, req)
	case azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_MODEL_FIRST:
		return s.promptAiDeploymentModelFirst(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported AI deployment selection_mode: %s", selectionMode.String())
	}
}

func (s *promptService) promptAiDeploymentLocationFirst(
	ctx context.Context,
	req *azdext.PromptAiDeploymentRequest,
) (*azdext.PromptAiDeploymentResponse, error) {
	locationResp, err := s.PromptAiLocation(ctx, &azdext.PromptAiLocationRequest{
		AzureContext:     req.GetAzureContext(),
		AllowedLocations: req.GetAllowedLocations(),
		Requirements:     req.GetRequirements(),
		Message:          req.GetLocationMessage(),
		HelpMessage:      req.GetLocationHelpMessage(),
	})
	if err != nil {
		return nil, err
	}
	if locationResp.GetLocation() == nil || locationResp.GetLocation().GetName() == "" {
		return nil, fmt.Errorf("no AI location selected")
	}

	modelResp, err := s.PromptAiModel(ctx, &azdext.PromptAiModelRequest{
		AzureContext:  req.GetAzureContext(),
		Location:      locationResp.GetLocation().GetName(),
		Kinds:         req.GetKinds(),
		Statuses:      req.GetStatuses(),
		Formats:       req.GetFormats(),
		Capabilities:  req.GetCapabilities(),
		PreferredSkus: req.GetPreferredSkus(),
		Message:       req.GetModelMessage(),
		HelpMessage:   req.GetModelHelpMessage(),
	})
	if err != nil {
		return nil, err
	}
	if modelResp.GetModel() == nil {
		return nil, fmt.Errorf("no AI model selected")
	}

	// Ensure location is always set on the deployment selection payload.
	if modelResp.GetModel().GetLocation() == "" {
		modelResp.Model.Location = locationResp.GetLocation().GetName()
	}

	return &azdext.PromptAiDeploymentResponse{
		Model: modelResp.GetModel(),
	}, nil
}

func (s *promptService) promptAiDeploymentModelFirst(
	ctx context.Context,
	req *azdext.PromptAiDeploymentRequest,
) (*azdext.PromptAiDeploymentResponse, error) {
	if s.aiClient == nil {
		return nil, fmt.Errorf("ai service is unavailable")
	}

	azureContext, err := s.createAzureContext(req.GetAzureContext())
	if err != nil {
		return nil, err
	}
	if azureContext.Scope.SubscriptionId == "" {
		return nil, fmt.Errorf("azure context must include subscription_id")
	}

	modelSelection, err := s.promptAiDeploymentModelConfig(ctx, req, azureContext.Scope.SubscriptionId)
	if err != nil {
		return nil, err
	}
	if modelSelection == nil || modelSelection.GetSku() == nil {
		return nil, fmt.Errorf("no AI model selected")
	}

	requirements := toAiUsageRequirements(req.GetRequirements())
	matchedLocationsResult, err := s.aiClient.FindAiLocationsForModelWithQuota(
		ctx,
		azureContext.Scope.SubscriptionId,
		modelSelection.GetName(),
		&azapi.AiFindLocationsForModelWithQuotaOptions{
			Locations:    req.GetAllowedLocations(),
			Versions:     []string{modelSelection.GetVersion()},
			Skus:         []string{modelSelection.GetSku().GetName()},
			Kinds:        req.GetKinds(),
			Formats:      req.GetFormats(),
			Statuses:     req.GetStatuses(),
			Capabilities: req.GetCapabilities(),
			Requirements: requirements,
		},
	)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	if len(matchedLocationsResult.MatchedLocations) == 0 {
		return nil, fmt.Errorf("no AI locations found that can deploy the selected model and satisfy the requested quota")
	}

	locationName, err := s.selectAiLocation(
		ctx,
		matchedLocationsResult.MatchedLocations,
		azureContext.Scope.Location,
		req.GetLocationMessage(),
		req.GetLocationHelpMessage(),
	)
	if err != nil {
		return nil, err
	}

	modelSelection.Location = locationName
	return &azdext.PromptAiDeploymentResponse{
		Model: modelSelection,
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
	if wire == nil {
		return nil, fmt.Errorf("azure_context is required")
	}
	if wire.Scope == nil {
		return nil, fmt.Errorf("azure_context.scope is required")
	}

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

type aiModelPromptOption struct {
	ModelName        string
	Location         string
	Version          string
	IsDefaultVersion bool
	Kind             string
	Format           string
	Status           string
	Capabilities     []string
	Sku              azapi.AiModelSku
}

type aiModelConfigPromptOption struct {
	ModelName        string
	Version          string
	IsDefaultVersion bool
	Kind             string
	Format           string
	Status           string
	Capabilities     []string
	Sku              azapi.AiModelSku
	Locations        []string
}

func (o aiModelPromptOption) id() string {
	return strings.Join([]string{
		o.ModelName,
		o.Location,
		o.Version,
		o.Sku.Name,
		o.Sku.UsageName,
	}, "|")
}

func (o aiModelPromptOption) label() string {
	parts := []string{
		fmt.Sprintf("%s/%s", o.ModelName, o.Version),
		fmt.Sprintf("sku=%s", o.Sku.Name),
		fmt.Sprintf("usage=%s", o.Sku.UsageName),
		fmt.Sprintf("location=%s", o.Location),
	}
	if o.IsDefaultVersion {
		parts = append(parts, "default")
	}

	return strings.Join(parts, " | ")
}

func (o aiModelPromptOption) toProto() *azdext.AiModelSelection {
	capabilities := slices.Clone(o.Capabilities)
	slices.Sort(capabilities)

	return &azdext.AiModelSelection{
		Name:             o.ModelName,
		Location:         o.Location,
		Version:          o.Version,
		IsDefaultVersion: o.IsDefaultVersion,
		Kind:             o.Kind,
		Format:           o.Format,
		Status:           o.Status,
		Capabilities:     capabilities,
		Sku: &azdext.AiModelSku{
			Name:            o.Sku.Name,
			UsageName:       o.Sku.UsageName,
			CapacityDefault: o.Sku.CapacityDefault,
			CapacityMinimum: o.Sku.CapacityMinimum,
			CapacityMaximum: o.Sku.CapacityMaximum,
			CapacityStep:    o.Sku.CapacityStep,
		},
	}
}

func (o aiModelConfigPromptOption) id() string {
	return strings.Join([]string{
		o.ModelName,
		o.Version,
		o.Sku.Name,
		o.Sku.UsageName,
	}, "|")
}

func (o aiModelConfigPromptOption) label() string {
	parts := []string{
		fmt.Sprintf("%s/%s", o.ModelName, o.Version),
		fmt.Sprintf("sku=%s", o.Sku.Name),
		fmt.Sprintf("usage=%s", o.Sku.UsageName),
	}
	if o.IsDefaultVersion {
		parts = append(parts, "default")
	}
	if len(o.Locations) > 0 {
		parts = append(parts, fmt.Sprintf("locations=%d", len(o.Locations)))
	}

	return strings.Join(parts, " | ")
}

func (o aiModelConfigPromptOption) toProto() *azdext.AiModelSelection {
	capabilities := slices.Clone(o.Capabilities)
	slices.Sort(capabilities)

	return &azdext.AiModelSelection{
		Name:             o.ModelName,
		Version:          o.Version,
		IsDefaultVersion: o.IsDefaultVersion,
		Kind:             o.Kind,
		Format:           o.Format,
		Status:           o.Status,
		Capabilities:     capabilities,
		Sku: &azdext.AiModelSku{
			Name:            o.Sku.Name,
			UsageName:       o.Sku.UsageName,
			CapacityDefault: o.Sku.CapacityDefault,
			CapacityMinimum: o.Sku.CapacityMinimum,
			CapacityMaximum: o.Sku.CapacityMaximum,
			CapacityStep:    o.Sku.CapacityStep,
		},
	}
}

func flattenAiModelCandidates(items []azapi.AiModelCatalogItem, location string) []aiModelPromptOption {
	candidates := []aiModelPromptOption{}
	for _, item := range items {
		for _, modelLocation := range item.Locations {
			if !strings.EqualFold(modelLocation.Location, location) {
				continue
			}

			for _, version := range modelLocation.Versions {
				for _, sku := range version.Skus {
					candidates = append(candidates, aiModelPromptOption{
						ModelName:        item.Name,
						Location:         modelLocation.Location,
						Version:          version.Version,
						IsDefaultVersion: version.IsDefaultVersion,
						Kind:             version.Kind,
						Format:           version.Format,
						Status:           version.Status,
						Capabilities:     version.Capabilities,
						Sku:              sku,
					})
				}
			}
		}
	}

	return candidates
}

func flattenAiModelConfigCandidates(items []azapi.AiModelCatalogItem) []aiModelConfigPromptOption {
	type aggregate struct {
		modelName        string
		version          string
		isDefaultVersion bool
		kind             string
		format           string
		status           string
		capabilityMap    map[string]string
		sku              azapi.AiModelSku
		locationMap      map[string]string
	}

	aggregatesByID := map[string]*aggregate{}
	for _, item := range items {
		modelName := strings.TrimSpace(item.Name)
		if modelName == "" {
			continue
		}

		for _, modelLocation := range item.Locations {
			locationName := strings.TrimSpace(modelLocation.Location)
			if locationName == "" {
				continue
			}

			for _, version := range modelLocation.Versions {
				for _, sku := range version.Skus {
					skuName := strings.TrimSpace(sku.Name)
					if skuName == "" {
						continue
					}

					id := strings.Join([]string{
						strings.ToLower(modelName),
						strings.ToLower(strings.TrimSpace(version.Version)),
						strings.ToLower(strings.TrimSpace(version.Kind)),
						strings.ToLower(strings.TrimSpace(version.Format)),
						strings.ToLower(strings.TrimSpace(version.Status)),
						strings.ToLower(skuName),
						strings.ToLower(strings.TrimSpace(sku.UsageName)),
						fmt.Sprintf("%d", sku.CapacityDefault),
						fmt.Sprintf("%d", sku.CapacityMinimum),
						fmt.Sprintf("%d", sku.CapacityMaximum),
						fmt.Sprintf("%d", sku.CapacityStep),
					}, "|")

					entry, has := aggregatesByID[id]
					if !has {
						entry = &aggregate{
							modelName:        modelName,
							version:          version.Version,
							isDefaultVersion: version.IsDefaultVersion,
							kind:             version.Kind,
							format:           version.Format,
							status:           version.Status,
							capabilityMap:    map[string]string{},
							sku:              sku,
							locationMap:      map[string]string{},
						}
						aggregatesByID[id] = entry
					} else {
						entry.isDefaultVersion = entry.isDefaultVersion || version.IsDefaultVersion
					}

					for _, capability := range version.Capabilities {
						trimmed := strings.TrimSpace(capability)
						if trimmed == "" {
							continue
						}

						key := strings.ToLower(trimmed)
						if _, exists := entry.capabilityMap[key]; !exists {
							entry.capabilityMap[key] = trimmed
						}
					}

					locationKey := strings.ToLower(locationName)
					if _, exists := entry.locationMap[locationKey]; !exists {
						entry.locationMap[locationKey] = locationName
					}
				}
			}
		}
	}

	candidates := make([]aiModelConfigPromptOption, 0, len(aggregatesByID))
	for _, entry := range aggregatesByID {
		capabilities := make([]string, 0, len(entry.capabilityMap))
		for _, capability := range entry.capabilityMap {
			capabilities = append(capabilities, capability)
		}
		slices.SortFunc(capabilities, func(a, b string) int {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		})

		locations := make([]string, 0, len(entry.locationMap))
		for _, location := range entry.locationMap {
			locations = append(locations, location)
		}
		slices.SortFunc(locations, func(a, b string) int {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		})

		candidates = append(candidates, aiModelConfigPromptOption{
			ModelName:        entry.modelName,
			Version:          entry.version,
			IsDefaultVersion: entry.isDefaultVersion,
			Kind:             entry.kind,
			Format:           entry.format,
			Status:           entry.status,
			Capabilities:     capabilities,
			Sku:              entry.sku,
			Locations:        locations,
		})
	}

	return candidates
}

func chooseDeterministicAiModel(
	candidates []aiModelPromptOption,
	preferredSkus []string,
) (*aiModelPromptOption, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no AI models found matching the provided filters")
	}

	filtered := slices.Clone(candidates)
	if len(preferredSkus) > 0 {
		preferredSet := make(map[string]struct{}, len(preferredSkus))
		for _, sku := range preferredSkus {
			preferredSet[strings.ToLower(strings.TrimSpace(sku))] = struct{}{}
		}

		preferredCandidates := []aiModelPromptOption{}
		for _, candidate := range filtered {
			if _, has := preferredSet[strings.ToLower(candidate.Sku.Name)]; has {
				preferredCandidates = append(preferredCandidates, candidate)
			}
		}

		if len(preferredCandidates) > 0 {
			filtered = preferredCandidates
		}
	}

	defaultCandidates := []aiModelPromptOption{}
	for _, candidate := range filtered {
		if candidate.IsDefaultVersion {
			defaultCandidates = append(defaultCandidates, candidate)
		}
	}
	if len(defaultCandidates) > 0 {
		filtered = defaultCandidates
	}

	sortAiModelCandidates(filtered, preferredSkus)
	if len(filtered) == 1 {
		return &filtered[0], nil
	}

	return nil, fmt.Errorf("multiple AI model candidates found; cannot select deterministically in --no-prompt mode")
}

func chooseDeterministicAiModelConfig(
	candidates []aiModelConfigPromptOption,
	preferredSkus []string,
) (*aiModelConfigPromptOption, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no AI models found matching the provided filters")
	}

	filtered := slices.Clone(candidates)
	if len(preferredSkus) > 0 {
		preferredSet := make(map[string]struct{}, len(preferredSkus))
		for _, sku := range preferredSkus {
			preferredSet[strings.ToLower(strings.TrimSpace(sku))] = struct{}{}
		}

		preferredCandidates := []aiModelConfigPromptOption{}
		for _, candidate := range filtered {
			if _, has := preferredSet[strings.ToLower(candidate.Sku.Name)]; has {
				preferredCandidates = append(preferredCandidates, candidate)
			}
		}

		if len(preferredCandidates) > 0 {
			filtered = preferredCandidates
		}
	}

	defaultCandidates := []aiModelConfigPromptOption{}
	for _, candidate := range filtered {
		if candidate.IsDefaultVersion {
			defaultCandidates = append(defaultCandidates, candidate)
		}
	}
	if len(defaultCandidates) > 0 {
		filtered = defaultCandidates
	}

	sortAiModelConfigCandidates(filtered, preferredSkus)
	if len(filtered) == 1 {
		return &filtered[0], nil
	}

	return nil, fmt.Errorf("multiple AI model candidates found; cannot select deterministically in --no-prompt mode")
}

func sortAiModelCandidates(candidates []aiModelPromptOption, preferredSkus []string) {
	preferredOrder := make(map[string]int, len(preferredSkus))
	for i, sku := range preferredSkus {
		preferredOrder[strings.ToLower(strings.TrimSpace(sku))] = i
	}

	const preferredDefaultRank = 1_000_000
	slices.SortFunc(candidates, func(a, b aiModelPromptOption) int {
		aRank, aHas := preferredOrder[strings.ToLower(a.Sku.Name)]
		bRank, bHas := preferredOrder[strings.ToLower(b.Sku.Name)]

		if !aHas {
			aRank = preferredDefaultRank
		}
		if !bHas {
			bRank = preferredDefaultRank
		}

		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}

		if a.IsDefaultVersion != b.IsDefaultVersion {
			if a.IsDefaultVersion {
				return -1
			}
			return 1
		}

		if cmp := strings.Compare(strings.ToLower(a.ModelName), strings.ToLower(b.ModelName)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.Version), strings.ToLower(b.Version)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.Sku.Name), strings.ToLower(b.Sku.Name)); cmp != 0 {
			return cmp
		}

		return strings.Compare(strings.ToLower(a.Location), strings.ToLower(b.Location))
	})
}

func sortAiModelConfigCandidates(candidates []aiModelConfigPromptOption, preferredSkus []string) {
	preferredOrder := make(map[string]int, len(preferredSkus))
	for i, sku := range preferredSkus {
		preferredOrder[strings.ToLower(strings.TrimSpace(sku))] = i
	}

	const preferredDefaultRank = 1_000_000
	slices.SortFunc(candidates, func(a, b aiModelConfigPromptOption) int {
		aRank, aHas := preferredOrder[strings.ToLower(a.Sku.Name)]
		bRank, bHas := preferredOrder[strings.ToLower(b.Sku.Name)]

		if !aHas {
			aRank = preferredDefaultRank
		}
		if !bHas {
			bRank = preferredDefaultRank
		}

		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}

		if a.IsDefaultVersion != b.IsDefaultVersion {
			if a.IsDefaultVersion {
				return -1
			}
			return 1
		}

		if cmp := strings.Compare(strings.ToLower(a.ModelName), strings.ToLower(b.ModelName)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.Version), strings.ToLower(b.Version)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.Sku.Name), strings.ToLower(b.Sku.Name)); cmp != 0 {
			return cmp
		}

		return strings.Compare(strings.ToLower(a.Sku.UsageName), strings.ToLower(b.Sku.UsageName))
	})
}

func toAiUsageRequirements(requirements []*azdext.AiUsageRequirement) []azapi.AiUsageRequirement {
	result := make([]azapi.AiUsageRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		requiredCapacity := float64(requirement.GetRequiredCapacity())
		if requiredCapacity <= 0 {
			requiredCapacity = 1
		}

		result = append(result, azapi.AiUsageRequirement{
			UsageName:        requirement.GetUsageName(),
			RequiredCapacity: requiredCapacity,
		})
	}

	return result
}

func (s *promptService) promptAiDeploymentModelConfig(
	ctx context.Context,
	req *azdext.PromptAiDeploymentRequest,
	subscriptionId string,
) (*azdext.AiModelSelection, error) {
	modelCatalog, err := s.aiClient.ListAiModelCatalog(
		ctx,
		subscriptionId,
		azapi.AiModelCatalogFilters{
			Locations:    req.GetAllowedLocations(),
			Kinds:        req.GetKinds(),
			Formats:      req.GetFormats(),
			Statuses:     req.GetStatuses(),
			Capabilities: req.GetCapabilities(),
		},
	)
	if err != nil {
		return nil, wrapErrorWithSuggestion(err)
	}

	candidates := flattenAiModelConfigCandidates(modelCatalog)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no AI models found matching the provided filters")
	}

	preferredSkus := req.GetPreferredSkus()
	if s.globalOptions.NoPrompt {
		selected, err := chooseDeterministicAiModelConfig(candidates, preferredSkus)
		if err != nil {
			return nil, err
		}

		return selected.toProto(), nil
	}

	sortAiModelConfigCandidates(candidates, preferredSkus)
	if len(candidates) == 1 {
		return candidates[0].toProto(), nil
	}

	selectedIndex, err := s.promptAiModelConfigCandidateSelection(
		ctx,
		candidates,
		preferredSkus,
		req.GetModelMessage(),
		req.GetModelHelpMessage(),
	)
	if err != nil {
		return nil, err
	}

	return candidates[selectedIndex].toProto(), nil
}

func (s *promptService) promptAiModelCandidateSelection(
	ctx context.Context,
	candidates []aiModelPromptOption,
	preferredSkus []string,
	message string,
	helpMessage string,
) (int, error) {
	type group struct {
		modelName        string
		location         string
		version          string
		isDefaultVersion bool
		kind             string
		format           string
		status           string
		capabilities     []string
		candidateIndexes []int
	}

	groupByKey := map[string]*group{}
	for i, candidate := range candidates {
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(candidate.ModelName)),
			strings.ToLower(strings.TrimSpace(candidate.Location)),
			strings.ToLower(strings.TrimSpace(candidate.Version)),
			strings.ToLower(strings.TrimSpace(candidate.Kind)),
			strings.ToLower(strings.TrimSpace(candidate.Format)),
			strings.ToLower(strings.TrimSpace(candidate.Status)),
			normalizedCapabilitiesKey(candidate.Capabilities),
		}, "|")

		entry, has := groupByKey[key]
		if !has {
			entry = &group{
				modelName:        candidate.ModelName,
				location:         candidate.Location,
				version:          candidate.Version,
				isDefaultVersion: candidate.IsDefaultVersion,
				kind:             candidate.Kind,
				format:           candidate.Format,
				status:           candidate.Status,
				capabilities:     slices.Clone(candidate.Capabilities),
			}
			groupByKey[key] = entry
		} else {
			entry.isDefaultVersion = entry.isDefaultVersion || candidate.IsDefaultVersion
		}

		entry.candidateIndexes = append(entry.candidateIndexes, i)
	}

	modelGroups := make([]*group, 0, len(groupByKey))
	for _, entry := range groupByKey {
		modelGroups = append(modelGroups, entry)
	}
	slices.SortFunc(modelGroups, func(a, b *group) int {
		if a.isDefaultVersion != b.isDefaultVersion {
			if a.isDefaultVersion {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(strings.ToLower(a.modelName), strings.ToLower(b.modelName)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.version), strings.ToLower(b.version)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.kind), strings.ToLower(b.kind)); cmp != 0 {
			return cmp
		}
		return strings.Compare(strings.ToLower(a.format), strings.ToLower(b.format))
	})

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return 0, err
	}
	defer release()

	type modelTypeGroup struct {
		kind        string
		format      string
		modelGroups []*group
	}

	typeGroupsByKey := map[string]*modelTypeGroup{}
	for _, entry := range modelGroups {
		typeKey := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(entry.kind)),
			strings.ToLower(strings.TrimSpace(entry.format)),
		}, "|")

		typeGroup, has := typeGroupsByKey[typeKey]
		if !has {
			typeGroup = &modelTypeGroup{
				kind:   entry.kind,
				format: entry.format,
			}
			typeGroupsByKey[typeKey] = typeGroup
		}

		typeGroup.modelGroups = append(typeGroup.modelGroups, entry)
	}

	typeGroups := make([]*modelTypeGroup, 0, len(typeGroupsByKey))
	for _, typeGroup := range typeGroupsByKey {
		typeGroups = append(typeGroups, typeGroup)
	}
	slices.SortFunc(typeGroups, func(a, b *modelTypeGroup) int {
		return strings.Compare(
			strings.ToLower(aiModelTypeLabel(a.kind, a.format)),
			strings.ToLower(aiModelTypeLabel(b.kind, b.format)),
		)
	})

	selectedModelGroups := modelGroups
	if len(typeGroups) > 1 {
		typeChoices := make([]*ux.SelectChoice, 0, len(typeGroups))
		for _, typeGroup := range typeGroups {
			label := aiModelTypeLabel(typeGroup.kind, typeGroup.format)
			typeChoices = append(typeChoices, &ux.SelectChoice{
				Value: label,
				Label: label,
			})
		}

		typeSelectPrompt := ux.NewSelect(&ux.SelectOptions{
			Message:         "Select an AI model type:",
			HelpMessage:     helpMessage,
			Choices:         typeChoices,
			EnableFiltering: to.Ptr(true),
			DisplayCount:    min(12, len(typeChoices)),
		})

		selectedTypeIndex, err := typeSelectPrompt.Ask(ctx)
		if err != nil {
			return 0, wrapErrorWithSuggestion(err)
		}
		if selectedTypeIndex == nil {
			return 0, fmt.Errorf("no AI model type selected")
		}

		selectedModelGroups = typeGroups[*selectedTypeIndex].modelGroups
	}

	selectedModelGroup := selectedModelGroups[0]
	if len(selectedModelGroups) > 1 {
		modelChoices := make([]*ux.SelectChoice, 0, len(selectedModelGroups))
		for _, entry := range selectedModelGroups {
			label := fmt.Sprintf("%s/%s", entry.modelName, entry.version)
			if entry.isDefaultVersion {
				label = fmt.Sprintf("%s (default)", label)
			}

			modelChoices = append(modelChoices, &ux.SelectChoice{
				Value: label,
				Label: label,
			})
		}

		modelPromptMessage := message
		if modelPromptMessage == "" {
			modelPromptMessage = "Select an AI model:"
		}

		modelSelectPrompt := ux.NewSelect(&ux.SelectOptions{
			Message:         modelPromptMessage,
			HelpMessage:     helpMessage,
			Choices:         modelChoices,
			EnableFiltering: to.Ptr(true),
			DisplayCount:    min(12, len(modelChoices)),
		})

		selectedModelIndex, err := modelSelectPrompt.Ask(ctx)
		if err != nil {
			return 0, wrapErrorWithSuggestion(err)
		}
		if selectedModelIndex == nil {
			return 0, fmt.Errorf("no AI model selected")
		}

		selectedModelGroup = selectedModelGroups[*selectedModelIndex]
	}

	selectedCandidateIndex := selectedModelGroup.candidateIndexes[0]
	if len(selectedModelGroup.candidateIndexes) == 1 {
		return selectedCandidateIndex, nil
	}

	sortedCandidateIndexes := slices.Clone(selectedModelGroup.candidateIndexes)
	preferredOrder := map[string]int{}
	for i, sku := range preferredSkus {
		preferredOrder[strings.ToLower(strings.TrimSpace(sku))] = i
	}
	const preferredDefaultRank = 1_000_000
	slices.SortFunc(sortedCandidateIndexes, func(a, b int) int {
		aSku := strings.ToLower(strings.TrimSpace(candidates[a].Sku.Name))
		bSku := strings.ToLower(strings.TrimSpace(candidates[b].Sku.Name))

		aRank, aHas := preferredOrder[aSku]
		bRank, bHas := preferredOrder[bSku]
		if !aHas {
			aRank = preferredDefaultRank
		}
		if !bHas {
			bRank = preferredDefaultRank
		}

		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}

		if cmp := strings.Compare(aSku, bSku); cmp != 0 {
			return cmp
		}

		return strings.Compare(
			strings.ToLower(strings.TrimSpace(candidates[a].Sku.UsageName)),
			strings.ToLower(strings.TrimSpace(candidates[b].Sku.UsageName)),
		)
	})

	skuChoices := make([]*ux.SelectChoice, 0, len(sortedCandidateIndexes))
	for _, candidateIndex := range sortedCandidateIndexes {
		sku := candidates[candidateIndex].Sku
		label := fmt.Sprintf(
			"%s (usage=%s, default capacity=%d)",
			sku.Name,
			sku.UsageName,
			max(1, sku.CapacityDefault),
		)

		skuChoices = append(skuChoices, &ux.SelectChoice{
			Value: fmt.Sprintf("%d", candidateIndex),
			Label: label,
		})
	}

	skuSelectPrompt := ux.NewSelect(&ux.SelectOptions{
		Message:         "Select an AI model SKU:",
		HelpMessage:     helpMessage,
		Choices:         skuChoices,
		EnableFiltering: to.Ptr(true),
		DisplayCount:    min(12, len(skuChoices)),
	})

	selectedSkuIndex, err := skuSelectPrompt.Ask(ctx)
	if err != nil {
		return 0, wrapErrorWithSuggestion(err)
	}
	if selectedSkuIndex == nil {
		return 0, fmt.Errorf("no AI model SKU selected")
	}

	return sortedCandidateIndexes[*selectedSkuIndex], nil
}

func (s *promptService) promptAiModelConfigCandidateSelection(
	ctx context.Context,
	candidates []aiModelConfigPromptOption,
	preferredSkus []string,
	message string,
	helpMessage string,
) (int, error) {
	type group struct {
		modelName        string
		version          string
		isDefaultVersion bool
		kind             string
		format           string
		status           string
		capabilities     []string
		candidateIndexes []int
	}

	groupByKey := map[string]*group{}
	for i, candidate := range candidates {
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(candidate.ModelName)),
			strings.ToLower(strings.TrimSpace(candidate.Version)),
			strings.ToLower(strings.TrimSpace(candidate.Kind)),
			strings.ToLower(strings.TrimSpace(candidate.Format)),
			strings.ToLower(strings.TrimSpace(candidate.Status)),
			normalizedCapabilitiesKey(candidate.Capabilities),
		}, "|")

		entry, has := groupByKey[key]
		if !has {
			entry = &group{
				modelName:        candidate.ModelName,
				version:          candidate.Version,
				isDefaultVersion: candidate.IsDefaultVersion,
				kind:             candidate.Kind,
				format:           candidate.Format,
				status:           candidate.Status,
				capabilities:     slices.Clone(candidate.Capabilities),
			}
			groupByKey[key] = entry
		} else {
			entry.isDefaultVersion = entry.isDefaultVersion || candidate.IsDefaultVersion
		}

		entry.candidateIndexes = append(entry.candidateIndexes, i)
	}

	modelGroups := make([]*group, 0, len(groupByKey))
	for _, entry := range groupByKey {
		modelGroups = append(modelGroups, entry)
	}
	slices.SortFunc(modelGroups, func(a, b *group) int {
		if a.isDefaultVersion != b.isDefaultVersion {
			if a.isDefaultVersion {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(strings.ToLower(a.modelName), strings.ToLower(b.modelName)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.version), strings.ToLower(b.version)); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(strings.ToLower(a.kind), strings.ToLower(b.kind)); cmp != 0 {
			return cmp
		}
		return strings.Compare(strings.ToLower(a.format), strings.ToLower(b.format))
	})

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return 0, err
	}
	defer release()

	type modelTypeGroup struct {
		kind        string
		format      string
		modelGroups []*group
	}

	typeGroupsByKey := map[string]*modelTypeGroup{}
	for _, entry := range modelGroups {
		typeKey := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(entry.kind)),
			strings.ToLower(strings.TrimSpace(entry.format)),
		}, "|")

		typeGroup, has := typeGroupsByKey[typeKey]
		if !has {
			typeGroup = &modelTypeGroup{
				kind:   entry.kind,
				format: entry.format,
			}
			typeGroupsByKey[typeKey] = typeGroup
		}

		typeGroup.modelGroups = append(typeGroup.modelGroups, entry)
	}

	typeGroups := make([]*modelTypeGroup, 0, len(typeGroupsByKey))
	for _, typeGroup := range typeGroupsByKey {
		typeGroups = append(typeGroups, typeGroup)
	}
	slices.SortFunc(typeGroups, func(a, b *modelTypeGroup) int {
		return strings.Compare(
			strings.ToLower(aiModelTypeLabel(a.kind, a.format)),
			strings.ToLower(aiModelTypeLabel(b.kind, b.format)),
		)
	})

	selectedModelGroups := modelGroups
	if len(typeGroups) > 1 {
		typeChoices := make([]*ux.SelectChoice, 0, len(typeGroups))
		for _, typeGroup := range typeGroups {
			label := aiModelTypeLabel(typeGroup.kind, typeGroup.format)
			typeChoices = append(typeChoices, &ux.SelectChoice{
				Value: label,
				Label: label,
			})
		}

		typeSelectPrompt := ux.NewSelect(&ux.SelectOptions{
			Message:         "Select an AI model type:",
			HelpMessage:     helpMessage,
			Choices:         typeChoices,
			EnableFiltering: to.Ptr(true),
			DisplayCount:    min(12, len(typeChoices)),
		})

		selectedTypeIndex, err := typeSelectPrompt.Ask(ctx)
		if err != nil {
			return 0, wrapErrorWithSuggestion(err)
		}
		if selectedTypeIndex == nil {
			return 0, fmt.Errorf("no AI model type selected")
		}

		selectedModelGroups = typeGroups[*selectedTypeIndex].modelGroups
	}

	selectedModelGroup := selectedModelGroups[0]
	if len(selectedModelGroups) > 1 {
		modelChoices := make([]*ux.SelectChoice, 0, len(selectedModelGroups))
		for _, entry := range selectedModelGroups {
			label := fmt.Sprintf("%s/%s", entry.modelName, entry.version)
			if entry.isDefaultVersion {
				label = fmt.Sprintf("%s (default)", label)
			}

			modelChoices = append(modelChoices, &ux.SelectChoice{
				Value: label,
				Label: label,
			})
		}

		modelPromptMessage := message
		if modelPromptMessage == "" {
			modelPromptMessage = "Select an AI model:"
		}

		modelSelectPrompt := ux.NewSelect(&ux.SelectOptions{
			Message:         modelPromptMessage,
			HelpMessage:     helpMessage,
			Choices:         modelChoices,
			EnableFiltering: to.Ptr(true),
			DisplayCount:    min(12, len(modelChoices)),
		})

		selectedModelIndex, err := modelSelectPrompt.Ask(ctx)
		if err != nil {
			return 0, wrapErrorWithSuggestion(err)
		}
		if selectedModelIndex == nil {
			return 0, fmt.Errorf("no AI model selected")
		}

		selectedModelGroup = selectedModelGroups[*selectedModelIndex]
	}

	selectedCandidateIndex := selectedModelGroup.candidateIndexes[0]
	if len(selectedModelGroup.candidateIndexes) == 1 {
		return selectedCandidateIndex, nil
	}

	sortedCandidateIndexes := slices.Clone(selectedModelGroup.candidateIndexes)
	preferredOrder := map[string]int{}
	for i, sku := range preferredSkus {
		preferredOrder[strings.ToLower(strings.TrimSpace(sku))] = i
	}
	const preferredDefaultRank = 1_000_000
	slices.SortFunc(sortedCandidateIndexes, func(a, b int) int {
		aSku := strings.ToLower(strings.TrimSpace(candidates[a].Sku.Name))
		bSku := strings.ToLower(strings.TrimSpace(candidates[b].Sku.Name))

		aRank, aHas := preferredOrder[aSku]
		bRank, bHas := preferredOrder[bSku]
		if !aHas {
			aRank = preferredDefaultRank
		}
		if !bHas {
			bRank = preferredDefaultRank
		}

		if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}

		if cmp := strings.Compare(aSku, bSku); cmp != 0 {
			return cmp
		}

		return strings.Compare(
			strings.ToLower(strings.TrimSpace(candidates[a].Sku.UsageName)),
			strings.ToLower(strings.TrimSpace(candidates[b].Sku.UsageName)),
		)
	})

	skuChoices := make([]*ux.SelectChoice, 0, len(sortedCandidateIndexes))
	for _, candidateIndex := range sortedCandidateIndexes {
		candidate := candidates[candidateIndex]
		sku := candidate.Sku
		label := fmt.Sprintf(
			"%s (usage=%s, default capacity=%d, locations=%d)",
			sku.Name,
			sku.UsageName,
			max(1, sku.CapacityDefault),
			len(candidate.Locations),
		)

		skuChoices = append(skuChoices, &ux.SelectChoice{
			Value: fmt.Sprintf("%d", candidateIndex),
			Label: label,
		})
	}

	skuSelectPrompt := ux.NewSelect(&ux.SelectOptions{
		Message:         "Select an AI model SKU:",
		HelpMessage:     helpMessage,
		Choices:         skuChoices,
		EnableFiltering: to.Ptr(true),
		DisplayCount:    min(12, len(skuChoices)),
	})

	selectedSkuIndex, err := skuSelectPrompt.Ask(ctx)
	if err != nil {
		return 0, wrapErrorWithSuggestion(err)
	}
	if selectedSkuIndex == nil {
		return 0, fmt.Errorf("no AI model SKU selected")
	}

	return sortedCandidateIndexes[*selectedSkuIndex], nil
}

func normalizedCapabilitiesKey(capabilities []string) string {
	if len(capabilities) == 0 {
		return ""
	}

	values := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		trimmed := strings.ToLower(strings.TrimSpace(capability))
		if trimmed == "" {
			continue
		}

		values = append(values, trimmed)
	}

	if len(values) == 0 {
		return ""
	}

	slices.Sort(values)
	values = slices.Compact(values)
	return strings.Join(values, ",")
}

func aiModelTypeLabel(kind string, format string) string {
	kind = strings.TrimSpace(kind)
	format = strings.TrimSpace(format)

	switch {
	case kind != "" && format != "":
		return fmt.Sprintf("%s / %s", kind, format)
	case kind != "":
		return kind
	case format != "":
		return format
	default:
		return "<unspecified type>"
	}
}

func (s *promptService) selectAiLocation(
	ctx context.Context,
	locations []string,
	currentLocation string,
	message string,
	helpMessage string,
) (string, error) {
	if len(locations) == 0 {
		return "", fmt.Errorf("no AI locations available for selection")
	}

	if s.globalOptions.NoPrompt {
		if currentLocation == "" {
			return "", fmt.Errorf("no location in azure context for --no-prompt mode")
		}

		selectedLocation, has := findCaseInsensitive(locations, currentLocation)
		if !has {
			return "", fmt.Errorf(
				"azure context location '%s' does not satisfy the selected model and requested AI quota",
				currentLocation,
			)
		}

		return selectedLocation, nil
	}

	if len(locations) == 1 {
		return locations[0], nil
	}

	release, err := s.acquirePromptLock(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	choices := make([]*ux.SelectChoice, 0, len(locations))
	for _, location := range locations {
		choices = append(choices, &ux.SelectChoice{
			Value: location,
			Label: location,
		})
	}

	if message == "" {
		message = "Select an Azure location for AI deployments:"
	}

	selectPrompt := ux.NewSelect(&ux.SelectOptions{
		Message:         message,
		HelpMessage:     helpMessage,
		Choices:         choices,
		EnableFiltering: to.Ptr(true),
		DisplayCount:    min(12, len(choices)),
	})

	selectedIndex, err := selectPrompt.Ask(ctx)
	if err != nil {
		return "", wrapErrorWithSuggestion(err)
	}
	if selectedIndex == nil {
		return "", fmt.Errorf("no AI location selected")
	}

	return locations[*selectedIndex], nil
}

func findCaseInsensitive(values []string, target string) (string, bool) {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return value, true
		}
	}

	return "", false
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
