// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

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
