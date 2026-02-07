// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newAiCommand() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Demonstrates AI model service capabilities.",
	}

	aiCmd.AddCommand(newAiListCommand())
	aiCmd.AddCommand(newAiUsagesCommand())
	aiCmd.AddCommand(newAiModelCommand())

	return aiCmd
}

// promptSubscription prompts the user to select an Azure subscription.
func promptSubscription(ctx context.Context, azdClient *azdext.AzdClient) (string, error) {
	resp, err := azdClient.Prompt().PromptSubscription(ctx, &azdext.PromptSubscriptionRequest{
		Message: "Select an Azure subscription",
	})
	if err != nil {
		return "", fmt.Errorf("selecting subscription: %w", err)
	}
	return resp.Subscription.Id, nil
}

// promptLocation prompts the user to select an Azure location.
func promptLocation(ctx context.Context, azdClient *azdext.AzdClient, subId string) (string, error) {
	resp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
		AzureContext: &azdext.AzureContext{
			Scope: &azdext.AzureScope{SubscriptionId: subId},
		},
	})
	if err != nil {
		return "", fmt.Errorf("selecting location: %w", err)
	}
	return resp.Location.Name, nil
}

func newAiListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Browse available AI models interactively.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			subId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			azureContext := &azdext.AzureContext{
				Scope: &azdext.AzureScope{SubscriptionId: subId},
			}

			modelResp, err := azdClient.Prompt().PromptAiModel(ctx, &azdext.PromptAiModelRequest{
				AzureContext: azureContext,
			})
			if err != nil {
				return fmt.Errorf("selecting model: %w", err)
			}

			model := modelResp.Model
			fmt.Println()
			color.HiWhite("Model Details:\n")
			fmt.Printf("  Name:       %s\n", color.CyanString(model.Name))
			fmt.Printf("  Format:     %s\n", model.Format)
			fmt.Printf("  Status:     %s\n", model.LifecycleStatus)
			fmt.Printf("  Locations:  %v\n", model.Locations)
			if len(model.Capabilities) > 0 {
				fmt.Printf("  Capabilities: %v\n", model.Capabilities)
			}

			if len(model.Versions) > 0 {
				fmt.Println()
				color.HiWhite("  Versions:\n")
				for _, v := range model.Versions {
					defaultLabel := ""
					if v.IsDefault {
						defaultLabel = color.YellowString(" (default)")
					}
					fmt.Printf("    %s%s\n", v.Version, defaultLabel)
					for _, sku := range v.Skus {
						fmt.Printf("      SKU: %-20s  usage_name: %s\n", sku.Name, sku.UsageName)
						if sku.DefaultCapacity > 0 || sku.MaxCapacity > 0 {
							fmt.Printf("           capacity: default=%d, min=%d, max=%d, step=%d\n",
								sku.DefaultCapacity, sku.MinCapacity, sku.MaxCapacity, sku.CapacityStep)
						}
					}
				}
			}

			return nil
		},
	}
}

func newAiUsagesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "usages",
		Short: "List AI model quota/usage data for a location.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			subId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			location, err := promptLocation(ctx, azdClient, subId)
			if err != nil {
				return err
			}

			color.Cyan("Listing AI model usages...")
			fmt.Printf("Subscription: %s\n", subId)
			fmt.Printf("Location: %s\n\n", location)

			resp, err := azdClient.Ai().ListUsages(ctx, &azdext.ListUsagesRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subId},
				},
				Location: location,
			})
			if err != nil {
				return fmt.Errorf("listing usages: %w", err)
			}

			color.HiWhite("Found %d usage entries:\n", len(resp.Usages))
			for _, usage := range resp.Usages {
				remaining := usage.Limit - usage.CurrentValue
				usageColor := color.HiGreenString
				if remaining <= 0 {
					usageColor = color.HiRedString
				} else if remaining < usage.Limit*0.2 {
					usageColor = color.HiYellowString
				}

				fmt.Printf("  %s: %s / %.0f (%s remaining)\n",
					color.CyanString(usage.Name),
					usageColor("%.0f", usage.CurrentValue),
					usage.Limit,
					usageColor("%.0f", remaining),
				)
			}

			return nil
		},
	}
}

func newAiModelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "model",
		Short: "Interactively select a model and resolve its deployment configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			subId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			location, err := promptLocation(ctx, azdClient, subId)
			if err != nil {
				return err
			}

			azureContext := &azdext.AzureContext{
				Scope: &azdext.AzureScope{
					SubscriptionId: subId,
					Location:       location,
				},
			}

			// Use PromptAiModel to let user select a model (scoped to chosen location)
			color.Cyan("Loading models for %s...", location)
			modelResp, err := azdClient.Prompt().PromptAiModel(ctx, &azdext.PromptAiModelRequest{
				AzureContext: azureContext,
				Filter: &azdext.AiModelFilterOptions{
					Locations: []string{location},
				},
				SelectOptions: &azdext.SelectOptions{
					Message: "Select an AI model to deploy",
				},
				Quota: &azdext.QuotaCheckOptions{
					MinRemainingCapacity: 1,
				},
			})
			if err != nil {
				return fmt.Errorf("selecting model: %w", err)
			}

			modelName := modelResp.Model.Name
			color.Cyan("\nResolving deployment for %s...", modelName)

			deployResp, err := azdClient.Prompt().PromptAiModelDeployment(ctx, &azdext.PromptAiModelDeploymentRequest{
				AzureContext: azureContext,
				ModelName:    modelName,
				Options: &azdext.AiModelDeploymentOptions{
					Locations: []string{location},
					// Skus:      []string{"GlobalStandard", "Standard"},
				},
				Quota: &azdext.QuotaCheckOptions{
					MinRemainingCapacity: 1,
				},
			})
			if err != nil {
				return fmt.Errorf("resolving deployment: %w", err)
			}

			d := deployResp.Deployment
			fmt.Println()
			color.HiWhite("Deployment Configuration:\n")
			fmt.Printf("  Model:      %s\n", color.CyanString(d.ModelName))
			fmt.Printf("  Format:     %s\n", d.Format)
			fmt.Printf("  Version:    %s\n", d.Version)
			fmt.Printf("  Location:   %s\n", d.Location)
			fmt.Printf("  SKU:        %s\n", d.Sku.Name)
			fmt.Printf("  UsageName:  %s\n", d.Sku.UsageName)
			fmt.Printf("  Capacity:   %d\n", d.Capacity)
			if d.RemainingQuota != nil {
				fmt.Printf("  Remaining:  %.0f\n", *d.RemainingQuota)
			}

			return nil
		},
	}
}
