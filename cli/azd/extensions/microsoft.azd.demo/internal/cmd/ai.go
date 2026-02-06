// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

func newAiCommand() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Examples of AI extension framework capabilities.",
	}

	aiCmd.AddCommand(newAiCatalogCommand())
	aiCmd.AddCommand(newAiUsagesCommand())
	aiCmd.AddCommand(newAiQuotaCommand())
	aiCmd.AddCommand(newAiPromptCommand())

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
		Short: "Interactively list AI quota usage.",
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

				namePrefix, err := promptUsageNamePrefix(ctx, azdClient)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().ListUsages(ctx, &azdext.AiListUsagesRequest{
					SubscriptionId: scope.SubscriptionId,
					Location:       location,
					NamePrefix:     namePrefix,
				})
				if err != nil {
					return err
				}

				if len(resp.Usages) == 0 {
					fmt.Println("No AI usage records found.")
					return nil
				}

				printUsageSummary(resp.Usages)

				return nil
			})
		},
	}

	return cmd
}

func newAiQuotaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Interactively find locations that satisfy AI quota requirements.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				requirements, err := promptQuotaRequirements(ctx, azdClient)
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

				req, err := buildAiFindLocationsWithQuotaRequest(scope.SubscriptionId, locations, requirements)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().FindLocationsWithQuota(ctx, req)
				if err != nil {
					return err
				}

				if len(resp.MatchedLocations) == 0 {
					fmt.Println("No matching locations found.")
				} else {
					fmt.Printf("Matched locations: %s\n", strings.Join(resp.MatchedLocations, ", "))
				}

				printQuotaSummary(resp.Results)

				return nil
			})
		},
	}

	return cmd
}

func newAiPromptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Run step-by-step AI location and model prompts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				filterByQuotaResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Filter location choices by quota requirements?",
						DefaultValue: boolPtr(true),
					},
				})
				if err != nil {
					return err
				}

				requirements := []string{}
				if filterByQuotaResp.GetValue() {
					requirements, err = promptQuotaRequirements(ctx, azdClient)
					if err != nil {
						return err
					}
				}

				locationPromptReq, err := buildPromptAiLocationRequest(scope, nil, requirements)
				if err != nil {
					return err
				}

				locationResp, err := azdClient.Prompt().PromptAiLocation(ctx, locationPromptReq)
				if err != nil {
					return err
				}

				catalogResp, err := azdClient.Ai().ListModelCatalog(ctx, &azdext.AiListModelCatalogRequest{
					SubscriptionId: scope.SubscriptionId,
					Locations:      []string{locationResp.GetLocation().GetName()},
				})
				if err != nil {
					return err
				}

				if len(catalogResp.Models) == 0 {
					return fmt.Errorf("no AI models found matching the provided filters")
				}

				fmt.Println("Selection:")
				selection, err := promptForModelCatalogSelection(
					ctx,
					azdClient,
					catalogResp.Models,
					locationResp.GetLocation().GetName(),
				)
				if err != nil {
					return err
				}
				printAiModelSelection(selection)

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
) ([]string, error) {
	requirements := []string{}
	for {
		requirementResponse, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
			Options: &azdext.PromptOptions{
				Message:      "Enter quota requirement (usageName[,capacity])",
				Required:     true,
				Placeholder:  "OpenAI.S0.AccountCount,1",
				DefaultValue: "OpenAI.S0.AccountCount,1",
			},
		})
		if err != nil {
			return nil, err
		}

		requirement := strings.TrimSpace(requirementResponse.GetValue())
		if requirement == "" {
			return nil, fmt.Errorf("quota requirement is required")
		}
		requirements = append(requirements, requirement)

		addAnotherResponse, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
			Options: &azdext.ConfirmOptions{
				Message:      "Add another quota requirement?",
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

func boolPtr(value bool) *bool {
	return &value
}

func promptUsageNamePrefix(
	ctx context.Context,
	azdClient *azdext.AzdClient,
) (string, error) {
	response, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
		Options: &azdext.PromptOptions{
			Message:      "Optional usage name prefix (press enter for all usages)",
			DefaultValue: "OpenAI.",
		},
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response.GetValue()), nil
}

func printUsageSummary(usages []*azdext.AiUsage) {
	const maxRows = 15

	fmt.Printf("Usage records: %d\n", len(usages))
	for i, usage := range usages {
		if i >= maxRows {
			fmt.Printf("... and %d more (use --name-prefix to narrow)\n", len(usages)-maxRows)
			break
		}

		fmt.Printf(
			"- %s: %.0f / %.0f remaining %.0f\n",
			usage.Name,
			usage.Current,
			usage.Limit,
			usage.Remaining,
		)
	}
}

func printQuotaSummary(results []*azdext.AiLocationQuotaResult) {
	unmatched := 0
	for _, result := range results {
		if result.GetMatched() {
			continue
		}

		unmatched++
		if result.GetError() != "" {
			fmt.Printf("- %s: %s\n", result.GetLocation(), result.GetError())
			continue
		}

		reason := "does not satisfy one or more requirements"
		for _, requirement := range result.GetRequirements() {
			if requirement.GetAvailableCapacity() < float64(requirement.GetRequiredCapacity()) {
				reason = fmt.Sprintf(
					"%s requires %d but has %.0f",
					requirement.GetUsageName(),
					requirement.GetRequiredCapacity(),
					requirement.GetAvailableCapacity(),
				)
				break
			}
		}

		fmt.Printf("- %s: %s\n", result.GetLocation(), reason)
	}

	if unmatched == 0 {
		fmt.Println("All evaluated locations satisfied the requirements.")
	}
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
	if len(selectedLocation.GetVersions()) == 0 {
		return nil, fmt.Errorf("no model versions found for '%s' in '%s'", selectedModel.GetName(), location)
	}

	selectedVersion := selectedLocation.GetVersions()[0]
	if len(selectedLocation.GetVersions()) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(selectedLocation.GetVersions()))
		for _, version := range selectedLocation.GetVersions() {
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
		selectedVersion = selectedLocation.GetVersions()[versionResp.GetValue()]
	}

	if len(selectedVersion.GetSkus()) == 0 {
		return nil, fmt.Errorf("no SKUs found for model '%s' version '%s'", selectedModel.GetName(), selectedVersion.GetVersion())
	}

	selectedSku := selectedVersion.GetSkus()[0]
	if len(selectedVersion.GetSkus()) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(selectedVersion.GetSkus()))
		for _, sku := range selectedVersion.GetSkus() {
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
		selectedSku = selectedVersion.GetSkus()[skuResp.GetValue()]
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

	fmt.Printf("  location: %s\n", model.GetLocation())
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

func buildAiFindLocationsWithQuotaRequest(
	subscriptionID string,
	locations []string,
	requirements []string,
) (*azdext.AiFindLocationsWithQuotaRequest, error) {
	parsedRequirements, err := parseAiUsageRequirements(requirements)
	if err != nil {
		return nil, err
	}

	if len(parsedRequirements) == 0 {
		return nil, fmt.Errorf("at least one --require value must be provided")
	}

	return &azdext.AiFindLocationsWithQuotaRequest{
		SubscriptionId: subscriptionID,
		Locations:      locations,
		Requirements:   parsedRequirements,
	}, nil
}

func buildPromptAiLocationRequest(
	scope *azdext.AzureScope,
	allowedLocations []string,
	requirements []string,
) (*azdext.PromptAiLocationRequest, error) {
	parsedRequirements, err := parseAiUsageRequirements(requirements)
	if err != nil {
		return nil, err
	}

	return &azdext.PromptAiLocationRequest{
		AzureContext: &azdext.AzureContext{
			Scope: scope,
		},
		AllowedLocations: allowedLocations,
		Requirements:     parsedRequirements,
	}, nil
}

func parseAiUsageRequirements(values []string) ([]*azdext.AiUsageRequirement, error) {
	requirements := make([]*azdext.AiUsageRequirement, 0, len(values))
	for _, value := range values {
		requirement, err := parseAiUsageRequirementArg(value)
		if err != nil {
			return nil, err
		}

		requirements = append(requirements, requirement)
	}

	return requirements, nil
}

func parseAiUsageRequirementArg(value string) (*azdext.AiUsageRequirement, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) == 0 || len(parts) > 2 {
		return nil, fmt.Errorf("invalid requirement format '%s' (expected usageName[,capacity])", value)
	}

	usageName := strings.TrimSpace(parts[0])
	if usageName == "" {
		return nil, fmt.Errorf("invalid requirement '%s': usage name is required", value)
	}

	requiredCapacity := int32(1)
	if len(parts) == 2 {
		parsedCapacity, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid requirement '%s': capacity must be an integer", value)
		}
		if parsedCapacity <= 0 {
			return nil, fmt.Errorf("invalid requirement '%s': capacity must be greater than 0", value)
		}

		requiredCapacity = int32(parsedCapacity)
	}

	return &azdext.AiUsageRequirement{
		UsageName:        usageName,
		RequiredCapacity: requiredCapacity,
	}, nil
}
