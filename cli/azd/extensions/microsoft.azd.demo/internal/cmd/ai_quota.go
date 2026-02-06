// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var quotaErrorCodeRegex = regexp.MustCompile(`ERROR CODE:\s*([A-Za-z0-9]+)`)

func newAiQuotaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Interactively find locations that satisfy model deployment and quota requirements.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				catalogResp, err := azdClient.Ai().ListModelCatalog(ctx, &azdext.AiListModelCatalogRequest{
					SubscriptionId: scope.SubscriptionId,
				})
				if err != nil {
					return err
				}
				if len(catalogResp.GetModels()) == 0 {
					fmt.Println("No AI model catalog entries found.")
					return nil
				}

				modelSelection, err := promptQuotaModelSelection(ctx, azdClient, catalogResp.GetModels())
				if err != nil {
					return err
				}
				if modelSelection.Sku == nil {
					return fmt.Errorf("model SKU selection is required")
				}
				if strings.TrimSpace(modelSelection.Sku.GetUsageName()) == "" {
					return fmt.Errorf("selected model SKU does not have a usage meter name")
				}
				if len(modelSelection.Locations) == 0 {
					return fmt.Errorf("selected model does not have any eligible locations")
				}

				defaultCapacity := modelSelection.Sku.GetCapacityDefault()
				if defaultCapacity <= 0 {
					defaultCapacity = 1
				}

				baseRequiredCapacity, err := promptRequiredCapacityForUsage(
					ctx,
					azdClient,
					modelSelection.Sku.GetUsageName(),
					defaultCapacity,
					"Set required capacity for the selected deployment SKU.",
				)
				if err != nil {
					return err
				}

				requirements := []*azdext.AiUsageRequirement{
					{
						UsageName:        modelSelection.Sku.GetUsageName(),
						RequiredCapacity: baseRequiredCapacity,
					},
				}

				addExtraResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Add extra usage requirements?",
						DefaultValue: boolPtr(false),
					},
				})
				if err != nil {
					return err
				}

				if addExtraResp.GetValue() {
					usageMeters, err := resolveUsageMetersForPrompt(
						ctx,
						azdClient,
						scope.SubscriptionId,
						modelSelection.Locations,
					)
					if err != nil {
						return err
					}

					excludedUsageNames := map[string]struct{}{
						strings.ToLower(strings.TrimSpace(modelSelection.Sku.GetUsageName())): {},
					}
					usageMeters = filterUsageMeters(usageMeters, excludedUsageNames)
					if len(usageMeters) > 0 {
						extraRequirements, err := promptQuotaRequirements(ctx, azdClient, usageMeters)
						if err != nil {
							return err
						}

						requirements = append(requirements, extraRequirements...)
					}
				}

				fmt.Println("Quota check target:")
				fmt.Printf("  model: %s\n", modelSelection.ModelName)
				fmt.Printf("  version: %s\n", modelSelection.Version)
				fmt.Printf("  sku: %s\n", modelSelection.Sku.GetName())
				fmt.Printf("  usage meter: %s\n", modelSelection.Sku.GetUsageName())
				fmt.Printf("  required capacity: %d\n", baseRequiredCapacity)

				req, err := buildAiFindLocationsForModelWithQuotaRequest(
					scope.SubscriptionId,
					slices.Clone(modelSelection.Locations),
					&azdext.AiModelSelection{
						Name:    modelSelection.ModelName,
						Version: modelSelection.Version,
						Sku:     modelSelection.Sku,
					},
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
