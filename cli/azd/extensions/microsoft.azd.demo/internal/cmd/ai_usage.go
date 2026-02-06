// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

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

			// Keep the largest observed current/limit snapshot as a stable hint value for selection labels.
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

	defaultCapacity := int32(1)
	helpMessage := fmt.Sprintf("Current %.0f / Limit %.0f", usage.GetCurrent(), usage.GetLimit())
	return promptRequiredCapacityForUsage(ctx, azdClient, usageName, defaultCapacity, helpMessage)
}

func promptRequiredCapacityForUsage(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	usageName string,
	defaultCapacity int32,
	helpMessage string,
) (int32, error) {
	trimmedUsageName := strings.TrimSpace(usageName)
	if trimmedUsageName == "" {
		trimmedUsageName = "selected usage meter"
	}

	if defaultCapacity <= 0 {
		defaultCapacity = 1
	}

	response, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
		Options: &azdext.PromptOptions{
			Message:      fmt.Sprintf("Required capacity for %s", trimmedUsageName),
			Required:     true,
			DefaultValue: fmt.Sprintf("%d", defaultCapacity),
			HelpMessage:  helpMessage,
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

func filterUsageMeters(usages []*azdext.AiUsage, excludedUsageNames map[string]struct{}) []*azdext.AiUsage {
	if len(usages) == 0 || len(excludedUsageNames) == 0 {
		return usages
	}

	filtered := make([]*azdext.AiUsage, 0, len(usages))
	for _, usage := range usages {
		usageName := strings.ToLower(strings.TrimSpace(usage.GetName()))
		if _, has := excludedUsageNames[usageName]; has {
			continue
		}

		filtered = append(filtered, usage)
	}

	return filtered
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
