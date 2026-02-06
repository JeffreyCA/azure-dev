// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var quotaErrorCodeRegex = regexp.MustCompile(`ERROR CODE:\s*([A-Za-z0-9]+)`)

func newAiCommand() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Examples of AI extension framework capabilities.",
	}

	aiCmd.AddCommand(newAiCatalogCommand())
	aiCmd.AddCommand(newAiUsagesCommand())
	aiCmd.AddCommand(newAiQuotaCommand())

	return aiCmd
}

func newAiCatalogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Interactively explore AI model catalog entries.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				location, err := promptLocationForScope(ctx, azdClient, scope)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().ListModelCatalog(ctx, &azdext.AiListModelCatalogRequest{
					SubscriptionId: scope.SubscriptionId,
					Locations:      []string{location},
				})
				if err != nil {
					return err
				}

				if len(resp.Models) == 0 {
					fmt.Println("No AI model catalog entries found.")
					return nil
				}

				selection, err := promptForModelCatalogSelection(ctx, azdClient, resp.Models, location)
				if err != nil {
					return err
				}

				fmt.Println("Catalog selection:")
				printAiModelSelection(selection)

				return nil
			})
		},
	}

	return cmd
}

func newAiUsagesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usages",
		Short: "Interactively select and inspect AI quota usage.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				location, err := promptLocationForScope(ctx, azdClient, scope)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().ListUsages(ctx, &azdext.AiListUsagesRequest{
					SubscriptionId: scope.SubscriptionId,
					Location:       location,
				})
				if err != nil {
					return err
				}

				if len(resp.Usages) == 0 {
					fmt.Println("No AI usage records found.")
					return nil
				}

				selectedUsage, err := promptUsageSelection(ctx, azdClient, resp.Usages)
				if err != nil {
					return err
				}
				printUsageDetails(selectedUsage, location)

				return nil
			})
		},
	}

	return cmd
}

func newAiQuotaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Interactively find locations that can deploy a model and satisfy quota.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				limitLocationResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Limit quota check to one location?",
						DefaultValue: boolPtr(false),
					},
				})
				if err != nil {
					return err
				}

				locations := []string{}
				if limitLocationResp.GetValue() {
					location, err := promptLocationForScope(ctx, azdClient, scope)
					if err != nil {
						return err
					}

					locations = []string{location}
				}

				catalogResp, err := azdClient.Ai().ListModelCatalog(ctx, &azdext.AiListModelCatalogRequest{
					SubscriptionId: scope.SubscriptionId,
					Locations:      locations,
				})
				if err != nil {
					return err
				}
				if len(catalogResp.GetModels()) == 0 {
					fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("No AI model catalog entries found."))
					return nil
				}

				modelSelection, err := promptForModelQuotaSelection(ctx, azdClient, catalogResp.GetModels())
				if err != nil {
					return err
				}
				fmt.Println("Model deployment selection:")
				printAiModelSelection(modelSelection)

				requirements := []*azdext.AiUsageRequirement{}
				addAdditionalResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Add extra usage requirements?",
						DefaultValue: boolPtr(false),
					},
				})
				if err != nil {
					return err
				}
				if addAdditionalResp.GetValue() {
					usageMeters, err := resolveUsageMetersForPrompt(ctx, azdClient, scope.SubscriptionId, locations)
					if err != nil {
						return err
					}

					requirements, err = promptQuotaRequirements(ctx, azdClient, usageMeters)
					if err != nil {
						return err
					}
				}

				req, err := buildAiFindLocationsForModelWithQuotaRequest(
					scope.SubscriptionId,
					locations,
					modelSelection,
					requirements,
				)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().FindLocationsForModelWithQuota(ctx, req)
				if err != nil {
					return err
				}

				if len(resp.MatchedLocations) == 0 {
					fmt.Println(color.New(color.FgYellow, color.Bold).Sprint("No matching locations found."))
				} else {
					fmt.Printf(
						"%s %s\n",
						color.New(color.FgGreen, color.Bold).Sprint("Matched locations:"),
						strings.Join(resp.MatchedLocations, ", "),
					)
				}

				printModelQuotaSummary(resp.Results)

				return nil
			})
		},
	}

	return cmd
}

func runWithAzdClient(cmd *cobra.Command, run func(context.Context, *azdext.AzdClient) error) error {
	ctx := azdext.WithAccessToken(cmd.Context())
	azdClient, err := azdext.NewAzdClient()
	if err != nil {
		return fmt.Errorf("failed to create azd client: %w", err)
	}
	defer azdClient.Close()

	if err := azdext.WaitForDebugger(ctx, azdClient); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, azdext.ErrDebuggerAborted) {
			return nil
		}

		return fmt.Errorf("failed waiting for debugger: %w", err)
	}

	return run(ctx, azdClient)
}

func promptSubscriptionScope(
	ctx context.Context,
	azdClient *azdext.AzdClient,
) (*azdext.AzureScope, error) {
	subscriptionResponse, err := azdClient.Prompt().PromptSubscription(ctx, &azdext.PromptSubscriptionRequest{
		Message: "Select an Azure subscription for this command:",
	})
	if err != nil {
		return nil, err
	}
	if subscriptionResponse.GetSubscription() == nil || subscriptionResponse.GetSubscription().GetId() == "" {
		return nil, fmt.Errorf("subscription id is required")
	}

	return &azdext.AzureScope{
		SubscriptionId: subscriptionResponse.GetSubscription().GetId(),
		TenantId:       subscriptionResponse.GetSubscription().GetTenantId(),
	}, nil
}

func promptLocationForScope(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	scope *azdext.AzureScope,
) (string, error) {
	if scope == nil || scope.GetSubscriptionId() == "" {
		return "", fmt.Errorf("azure scope with subscription id is required")
	}

	locationResponse, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
		AzureContext: &azdext.AzureContext{
			Scope: scope,
		},
	})
	if err != nil {
		return "", err
	}
	if locationResponse.GetLocation() == nil || strings.TrimSpace(locationResponse.GetLocation().GetName()) == "" {
		return "", fmt.Errorf("location is required")
	}

	return strings.TrimSpace(locationResponse.GetLocation().GetName()), nil
}

func promptQuotaRequirements(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	usageMeters []*azdext.AiUsage,
) ([]*azdext.AiUsageRequirement, error) {
	requirements := []*azdext.AiUsageRequirement{}
	for {
		selectedUsage, err := promptUsageSelection(ctx, azdClient, usageMeters)
		if err != nil {
			return nil, err
		}

		requiredCapacity, err := promptRequiredCapacity(ctx, azdClient, selectedUsage)
		if err != nil {
			return nil, err
		}

		requirements = append(requirements, &azdext.AiUsageRequirement{
			UsageName:        selectedUsage.GetName(),
			RequiredCapacity: requiredCapacity,
		})

		addAnotherResponse, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
			Options: &azdext.ConfirmOptions{
				Message:      "Add another usage requirement?",
				DefaultValue: boolPtr(false),
			},
		})
		if err != nil {
			return nil, err
		}
		if !addAnotherResponse.GetValue() {
			break
		}
	}

	return requirements, nil
}

func resolveUsageMetersForPrompt(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	subscriptionID string,
	locations []string,
) ([]*azdext.AiUsage, error) {
	candidateLocations := slices.Clone(locations)
	if len(candidateLocations) == 0 {
		locationsResp, err := azdClient.Ai().ListLocations(ctx, &azdext.AiListLocationsRequest{
			SubscriptionId: subscriptionID,
		})
		if err != nil {
			return nil, err
		}

		candidateLocations = locationsResp.GetLocations()
	}

	usagesByName := map[string]*azdext.AiUsage{}
	for _, location := range candidateLocations {
		usagesResp, err := azdClient.Ai().ListUsages(ctx, &azdext.AiListUsagesRequest{
			SubscriptionId: subscriptionID,
			Location:       location,
		})
		if err != nil {
			continue
		}

		for _, usage := range usagesResp.GetUsages() {
			usageName := strings.TrimSpace(usage.GetName())
			if usageName == "" {
				continue
			}

			key := strings.ToLower(usageName)
			existing, has := usagesByName[key]
			if !has {
				usagesByName[key] = &azdext.AiUsage{
					Name:      usageName,
					Current:   usage.GetCurrent(),
					Limit:     usage.GetLimit(),
					Remaining: usage.GetRemaining(),
					Unit:      usage.GetUnit(),
				}
				continue
			}

			// Keep the largest observed current/limit snapshot as a stable hint value for interactive selection labels.
			if usage.GetCurrent() > existing.GetCurrent() {
				existing.Current = usage.GetCurrent()
			}
			if usage.GetLimit() > existing.GetLimit() {
				existing.Limit = usage.GetLimit()
			}
			existing.Remaining = existing.GetLimit() - existing.GetCurrent()
		}
	}

	if len(usagesByName) == 0 {
		return nil, fmt.Errorf("unable to retrieve usage meters for interactive selection")
	}

	usageMeters := make([]*azdext.AiUsage, 0, len(usagesByName))
	for _, usage := range usagesByName {
		usageMeters = append(usageMeters, usage)
	}
	slices.SortFunc(usageMeters, func(a *azdext.AiUsage, b *azdext.AiUsage) int {
		return strings.Compare(strings.ToLower(a.GetName()), strings.ToLower(b.GetName()))
	})

	return usageMeters, nil
}

func promptRequiredCapacity(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	usage *azdext.AiUsage,
) (int32, error) {
	if usage == nil {
		return 0, fmt.Errorf("usage meter is required")
	}

	usageName := strings.TrimSpace(usage.GetName())
	if usageName == "" {
		usageName = "selected usage meter"
	}

	response, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
		Options: &azdext.PromptOptions{
			Message:      fmt.Sprintf("Required capacity for %s", usageName),
			Required:     true,
			DefaultValue: "1",
			HelpMessage:  fmt.Sprintf("Current %.0f / Limit %.0f", usage.GetCurrent(), usage.GetLimit()),
		},
	})
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(response.GetValue())
	capacity, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid capacity '%s': must be a positive integer", trimmed)
	}
	if capacity <= 0 {
		return 0, fmt.Errorf("capacity must be greater than 0")
	}

	return int32(capacity), nil
}

func boolPtr(value bool) *bool {
	return &value
}

func promptUsageSelection(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	usages []*azdext.AiUsage,
) (*azdext.AiUsage, error) {
	if len(usages) == 0 {
		return nil, fmt.Errorf("no usage records available for selection")
	}

	choices := make([]*azdext.SelectChoice, 0, len(usages))
	for _, usage := range usages {
		name := strings.TrimSpace(usage.GetName())
		if name == "" {
			name = "<unnamed usage>"
		}

		usageStats := color.HiBlackString("(current %.0f / limit %.0f)", usage.GetCurrent(), usage.GetLimit())

		choices = append(choices, &azdext.SelectChoice{
			Label: fmt.Sprintf("%s %s", name, usageStats),
			Value: name,
		})
	}

	enableFiltering := true
	displayCount := int32(min(12, len(choices)))
	response, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
		Options: &azdext.SelectOptions{
			Message:         "Select a usage meter:",
			Choices:         choices,
			EnableFiltering: &enableFiltering,
			DisplayCount:    displayCount,
		},
	})
	if err != nil {
		return nil, err
	}

	index := int(response.GetValue())
	if index < 0 || index >= len(usages) {
		return nil, fmt.Errorf("invalid usage selection index: %d", response.GetValue())
	}

	return usages[index], nil
}

func printUsageDetails(usage *azdext.AiUsage, location string) {
	if usage == nil {
		fmt.Println("No usage selected.")
		return
	}

	fmt.Println("Usage details:")
	fmt.Printf("  location: %s\n", location)
	fmt.Printf("  name: %s\n", usage.GetName())
	fmt.Printf("  current: %.0f\n", usage.GetCurrent())
	fmt.Printf("  limit: %.0f\n", usage.GetLimit())
	fmt.Printf("  remaining: %.0f\n", usage.GetRemaining())
	if usage.GetUnit() != "" {
		fmt.Printf("  unit: %s\n", usage.GetUnit())
	}
	if usage.GetLimit() > 0 {
		utilization := (usage.GetCurrent() / usage.GetLimit()) * 100
		if utilization < 0 {
			utilization = 0
		}
		fmt.Printf("  utilization: %.1f%%\n", utilization)
	}
}

func printModelQuotaSummary(results []*azdext.AiModelLocationQuotaResult) {
	matchedCount := 0
	for _, result := range results {
		if result.GetMatched() {
			matchedCount++
		}
	}

	fmt.Printf("Quota check summary: %d/%d locations matched\n", matchedCount, len(results))

	for _, result := range results {
		location := result.GetLocation()
		if location == "" {
			location = "<unknown>"
		}

		if result.GetMatched() {
			fmt.Printf("  %s %s\n", color.New(color.FgGreen).Sprint("[MATCH]"), location)
			continue
		}

		if result.GetError() != "" {
			fmt.Printf(
				"  %s %s - %s\n",
				color.New(color.FgHiBlack).Sprint("[SKIP]"),
				location,
				summarizeQuotaError(result.GetError()),
			)
			continue
		}

		reason := "does not satisfy one or more requirements"
		for _, requirement := range result.GetRequirements() {
			if requirement.GetAvailableCapacity() < float64(requirement.GetRequiredCapacity()) {
				usageName := color.New(color.FgCyan, color.Bold).Sprintf("%s", requirement.GetUsageName())
				requiredValue := color.New(color.FgYellow, color.Bold).Sprintf("%d", requirement.GetRequiredCapacity())
				availableValue := color.New(color.FgYellow, color.Bold).Sprintf("%.0f", requirement.GetAvailableCapacity())
				reason = fmt.Sprintf(
					"%s requires %s but has %s",
					usageName,
					requiredValue,
					availableValue,
				)
				break
			}
		}

		fmt.Printf("  %s %s - %s\n", color.New(color.FgYellow).Sprint("[MISS]"), location, reason)
	}
}

func summarizeQuotaError(raw string) string {
	errorText := strings.TrimSpace(raw)
	if errorText == "" {
		return "quota lookup unavailable in this location"
	}

	matches := quotaErrorCodeRegex.FindStringSubmatch(errorText)
	if len(matches) >= 2 {
		return fmt.Sprintf("quota lookup unavailable (%s)", matches[1])
	}

	return "quota lookup unavailable in this location"
}

func printCatalogSummary(models []*azdext.AiModelCatalogItem) {
	const maxRows = 20

	fmt.Printf("Models found: %d\n", len(models))
	for i, model := range models {
		if i >= maxRows {
			fmt.Printf("... and %d more (add filters to narrow)\n", len(models)-maxRows)
			break
		}

		versionCount := 0
		skuCount := 0
		for _, location := range model.GetLocations() {
			versionCount += len(location.GetVersions())
			for _, version := range location.GetVersions() {
				skuCount += len(version.GetSkus())
			}
		}

		fmt.Printf(
			"- %s (locations=%d versions=%d skus=%d)\n",
			model.GetName(),
			len(model.GetLocations()),
			versionCount,
			skuCount,
		)
	}
}

func promptForModelQuotaSelection(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	models []*azdext.AiModelCatalogItem,
) (*azdext.AiModelSelection, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models available for selection")
	}

	modelOptions := slices.Clone(models)
	slices.SortFunc(modelOptions, func(a *azdext.AiModelCatalogItem, b *azdext.AiModelCatalogItem) int {
		return strings.Compare(strings.ToLower(a.GetName()), strings.ToLower(b.GetName()))
	})

	selectedModel := modelOptions[0]
	if len(modelOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(modelOptions))
		for _, model := range modelOptions {
			choices = append(choices, &azdext.SelectChoice{
				Label: model.GetName(),
				Value: model.GetName(),
			})
		}

		enableFiltering := true
		modelResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message:         "Select an AI model",
				Choices:         choices,
				EnableFiltering: &enableFiltering,
				DisplayCount:    int32(min(12, len(choices))),
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model selection: %w", err)
		}

		index := int(modelResp.GetValue())
		if index < 0 || index >= len(modelOptions) {
			return nil, fmt.Errorf("invalid model selection index: %d", modelResp.GetValue())
		}

		selectedModel = modelOptions[index]
	}

	versionOptions := collectModelVersions(selectedModel)
	if len(versionOptions) == 0 {
		return nil, fmt.Errorf("no model versions found for '%s'", selectedModel.GetName())
	}

	selectedVersion := versionOptions[0]
	if len(versionOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(versionOptions))
		for _, version := range versionOptions {
			label := version.GetVersion()
			if version.GetIsDefaultVersion() {
				label = fmt.Sprintf("%s (default)", label)
			}
			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: version.GetVersion(),
			})
		}

		versionResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message: "Select a model version",
				Choices: choices,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model version selection: %w", err)
		}

		index := int(versionResp.GetValue())
		if index < 0 || index >= len(versionOptions) {
			return nil, fmt.Errorf("invalid model version selection index: %d", versionResp.GetValue())
		}

		selectedVersion = versionOptions[index]
	}

	skuOptions := selectedVersion.GetSkus()
	if len(skuOptions) == 0 {
		return nil, fmt.Errorf("no SKUs found for model '%s' version '%s'", selectedModel.GetName(), selectedVersion.GetVersion())
	}

	selectedSku := skuOptions[0]
	if len(skuOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(skuOptions))
		for _, sku := range skuOptions {
			label := fmt.Sprintf(
				"%s (usage=%s, default_capacity=%d)",
				sku.GetName(),
				sku.GetUsageName(),
				sku.GetCapacityDefault(),
			)
			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: sku.GetName(),
			})
		}

		skuResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message: "Select a model SKU",
				Choices: choices,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model SKU selection: %w", err)
		}

		index := int(skuResp.GetValue())
		if index < 0 || index >= len(skuOptions) {
			return nil, fmt.Errorf("invalid model SKU selection index: %d", skuResp.GetValue())
		}

		selectedSku = skuOptions[index]
	}

	return &azdext.AiModelSelection{
		Name:             selectedModel.GetName(),
		Version:          selectedVersion.GetVersion(),
		IsDefaultVersion: selectedVersion.GetIsDefaultVersion(),
		Kind:             selectedVersion.GetKind(),
		Format:           selectedVersion.GetFormat(),
		Status:           selectedVersion.GetStatus(),
		Capabilities:     selectedVersion.GetCapabilities(),
		Sku:              selectedSku,
	}, nil
}

func collectModelVersions(model *azdext.AiModelCatalogItem) []*azdext.AiModelVersion {
	if model == nil {
		return nil
	}

	versionByKey := map[string]*azdext.AiModelVersion{}
	order := []string{}
	for _, location := range model.GetLocations() {
		for _, version := range location.GetVersions() {
			versionName := strings.TrimSpace(version.GetVersion())
			if versionName == "" {
				continue
			}

			key := strings.ToLower(versionName)
			existing, has := versionByKey[key]
			if !has {
				clonedVersion := cloneAiModelVersion(version)
				versionByKey[key] = clonedVersion
				order = append(order, key)
				continue
			}

			existing.IsDefaultVersion = existing.GetIsDefaultVersion() || version.GetIsDefaultVersion()
			existing.Skus = mergeAiModelSkus(existing.GetSkus(), version.GetSkus())
		}
	}

	versions := make([]*azdext.AiModelVersion, 0, len(order))
	for _, key := range order {
		versions = append(versions, versionByKey[key])
	}

	slices.SortFunc(versions, func(a *azdext.AiModelVersion, b *azdext.AiModelVersion) int {
		return strings.Compare(a.GetVersion(), b.GetVersion())
	})

	return versions
}

func cloneAiModelVersion(version *azdext.AiModelVersion) *azdext.AiModelVersion {
	if version == nil {
		return nil
	}

	return &azdext.AiModelVersion{
		Version:          version.GetVersion(),
		IsDefaultVersion: version.GetIsDefaultVersion(),
		Kind:             version.GetKind(),
		Format:           version.GetFormat(),
		Status:           version.GetStatus(),
		Capabilities:     slices.Clone(version.GetCapabilities()),
		Skus:             mergeAiModelSkus(nil, version.GetSkus()),
	}
}

func mergeAiModelSkus(existing []*azdext.AiModelSku, incoming []*azdext.AiModelSku) []*azdext.AiModelSku {
	if len(incoming) == 0 {
		return existing
	}

	merged := slices.Clone(existing)
	index := make(map[string]struct{}, len(merged))
	for _, sku := range merged {
		if sku == nil {
			continue
		}
		index[strings.ToLower(strings.TrimSpace(sku.GetName()))] = struct{}{}
	}

	for _, sku := range incoming {
		if sku == nil {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(sku.GetName()))
		if key == "" {
			continue
		}
		if _, has := index[key]; has {
			continue
		}

		merged = append(merged, &azdext.AiModelSku{
			Name:            sku.GetName(),
			UsageName:       sku.GetUsageName(),
			CapacityDefault: sku.GetCapacityDefault(),
			CapacityMinimum: sku.GetCapacityMinimum(),
			CapacityMaximum: sku.GetCapacityMaximum(),
			CapacityStep:    sku.GetCapacityStep(),
		})
		index[key] = struct{}{}
	}

	slices.SortFunc(merged, func(a *azdext.AiModelSku, b *azdext.AiModelSku) int {
		return strings.Compare(strings.ToLower(a.GetName()), strings.ToLower(b.GetName()))
	})

	return merged
}

func promptForModelCatalogSelection(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	models []*azdext.AiModelCatalogItem,
	location string,
) (*azdext.AiModelSelection, error) {
	filteredModels := make([]*azdext.AiModelCatalogItem, 0, len(models))
	for _, model := range models {
		for _, modelLocation := range model.GetLocations() {
			if strings.EqualFold(modelLocation.GetLocation(), location) {
				filteredModels = append(filteredModels, model)
				break
			}
		}
	}

	if len(filteredModels) == 0 {
		return nil, fmt.Errorf("no models found in location '%s'", location)
	}

	slices.SortFunc(filteredModels, func(a *azdext.AiModelCatalogItem, b *azdext.AiModelCatalogItem) int {
		return strings.Compare(strings.ToLower(a.GetName()), strings.ToLower(b.GetName()))
	})

	selectedModel := filteredModels[0]
	if len(filteredModels) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(filteredModels))
		for _, model := range filteredModels {
			choices = append(choices, &azdext.SelectChoice{
				Label: model.GetName(),
				Value: model.GetName(),
			})
		}

		modelResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message: "Select an AI model",
				Choices: choices,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model selection: %w", err)
		}
		selectedModel = filteredModels[modelResp.GetValue()]
	}

	var selectedLocation *azdext.AiModelLocation
	for _, modelLocation := range selectedModel.GetLocations() {
		if strings.EqualFold(modelLocation.GetLocation(), location) {
			selectedLocation = modelLocation
			break
		}
	}
	if selectedLocation == nil {
		return nil, fmt.Errorf("selected model is not available in location '%s'", location)
	}
	versionOptions := selectedLocation.GetVersions()
	if len(versionOptions) == 0 {
		return nil, fmt.Errorf("no model versions found for '%s' in '%s'", selectedModel.GetName(), location)
	}

	selectedVersion := versionOptions[0]
	if len(versionOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(versionOptions))
		for _, version := range versionOptions {
			label := version.GetVersion()
			if version.GetIsDefaultVersion() {
				label = fmt.Sprintf("%s (default)", label)
			}
			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: version.GetVersion(),
			})
		}

		versionResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message: "Select a model version",
				Choices: choices,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model version selection: %w", err)
		}
		selectedVersion = versionOptions[versionResp.GetValue()]
	}

	skus := selectedVersion.GetSkus()
	if len(skus) == 0 {
		return nil, fmt.Errorf("no SKUs found for model '%s' version '%s'", selectedModel.GetName(), selectedVersion.GetVersion())
	}

	selectedSku := skus[0]
	if len(skus) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(skus))
		for _, sku := range skus {
			label := fmt.Sprintf(
				"%s (usage=%s, default_capacity=%d)",
				sku.GetName(),
				sku.GetUsageName(),
				sku.GetCapacityDefault(),
			)
			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: sku.GetName(),
			})
		}

		skuResp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
			Options: &azdext.SelectOptions{
				Message: "Select a model SKU",
				Choices: choices,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to prompt for model SKU selection: %w", err)
		}
		selectedSku = skus[skuResp.GetValue()]
	}

	return &azdext.AiModelSelection{
		Name:             selectedModel.GetName(),
		Location:         location,
		Version:          selectedVersion.GetVersion(),
		IsDefaultVersion: selectedVersion.GetIsDefaultVersion(),
		Kind:             selectedVersion.GetKind(),
		Format:           selectedVersion.GetFormat(),
		Status:           selectedVersion.GetStatus(),
		Capabilities:     selectedVersion.GetCapabilities(),
		Sku:              selectedSku,
	}, nil
}

func printAiModelSelection(model *azdext.AiModelSelection) {
	if model == nil {
		fmt.Println("  no model selected")
		return
	}

	if model.GetLocation() != "" {
		fmt.Printf("  location: %s\n", model.GetLocation())
	}
	fmt.Printf("  model: %s\n", model.GetName())
	fmt.Printf("  version: %s\n", model.GetVersion())
	fmt.Printf("  kind: %s\n", model.GetKind())
	fmt.Printf("  format: %s\n", model.GetFormat())
	fmt.Printf("  status: %s\n", model.GetStatus())
	if model.GetSku() != nil {
		fmt.Printf("  sku: %s\n", model.GetSku().GetName())
		fmt.Printf("  usage_name: %s\n", model.GetSku().GetUsageName())
		fmt.Printf("  capacity_default: %d\n", model.GetSku().GetCapacityDefault())
	}
}

func buildAiFindLocationsForModelWithQuotaRequest(
	subscriptionID string,
	locations []string,
	modelSelection *azdext.AiModelSelection,
	requirements []*azdext.AiUsageRequirement,
) (*azdext.AiFindLocationsForModelWithQuotaRequest, error) {
	if modelSelection == nil || strings.TrimSpace(modelSelection.GetName()) == "" {
		return nil, fmt.Errorf("model selection is required")
	}
	if modelSelection.GetSku() == nil || strings.TrimSpace(modelSelection.GetSku().GetName()) == "" {
		return nil, fmt.Errorf("model SKU selection is required")
	}

	return &azdext.AiFindLocationsForModelWithQuotaRequest{
		SubscriptionId: subscriptionID,
		ModelName:      modelSelection.GetName(),
		Locations:      locations,
		Versions:       []string{modelSelection.GetVersion()},
		Skus:           []string{modelSelection.GetSku().GetName()},
		Requirements:   requirements,
	}, nil
}
