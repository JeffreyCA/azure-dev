// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func newAiModelsCommand() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Demonstrates AI model service capabilities.",
	}

	aiCmd.AddCommand(newAiListModelsCommand())
	aiCmd.AddCommand(newAiListVersionsCommand())
	aiCmd.AddCommand(newAiListSkusCommand())
	aiCmd.AddCommand(newAiGetDeploymentCommand())
	aiCmd.AddCommand(newAiListUsagesCommand())
	aiCmd.AddCommand(newAiLocationsWithQuotaCommand())
	aiCmd.AddCommand(newAiSkuLocationsCommand())
	aiCmd.AddCommand(newAiPromptModelCommand())
	aiCmd.AddCommand(newAiPromptDeploymentCommand())

	return aiCmd
}

// initClient creates an AZD client and returns the context with access token.
func initClient(cmd *cobra.Command) (context.Context, *azdext.AzdClient, error) {
	ctx := azdext.WithAccessToken(cmd.Context())

	azdClient, err := azdext.NewAzdClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create azd client: %w", err)
	}

	if err := azdext.WaitForDebugger(ctx, azdClient); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, azdext.ErrDebuggerAborted) {
			azdClient.Close()
			return nil, nil, nil
		}
		azdClient.Close()
		return nil, nil, fmt.Errorf("failed waiting for debugger: %w", err)
	}

	return ctx, azdClient, nil
}

// promptSubscription prompts for a subscription and returns its ID.
func promptSubscription(ctx context.Context, azdClient *azdext.AzdClient) (string, error) {
	resp, err := azdClient.Prompt().PromptSubscription(ctx, &azdext.PromptSubscriptionRequest{
		Message: "Select a subscription for AI model lookup",
	})
	if err != nil {
		return "", err
	}
	return resp.Subscription.Id, nil
}

// --- List Models ---

func newAiListModelsCommand() *cobra.Command {
	var location string
	var kinds []string
	var capabilities []string

	cmd := &cobra.Command{
		Use:   "list-models",
		Short: "List available AI models (AiModelService.ListModels).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			req := &azdext.ListModelsRequest{
				SubscriptionId: subscriptionId,
				Filter: &azdext.AiModelFilterOptions{
					Kinds:        kinds,
					Capabilities: capabilities,
				},
			}
			if location != "" {
				req.Location = &location
			}

			fmt.Println()
			color.Cyan("Fetching AI models...")
			resp, err := azdClient.AiModel().ListModels(ctx, req)
			if err != nil {
				return fmt.Errorf("listing models: %w", err)
			}

			fmt.Printf("\nFound %s models:\n\n", color.HiWhiteString("%d", len(resp.Models)))
			for _, model := range resp.Models {
				locations := make([]string, 0, len(model.DetailsByLocation))
				for loc := range model.DetailsByLocation {
					locations = append(locations, loc)
				}
				fmt.Printf("  %s  (%d locations)\n",
					color.HiCyanString(model.Name),
					len(locations))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&location, "location", "", "Filter by location (empty = all locations)")
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "Filter by kind (e.g. OpenAI, AIServices)")
	cmd.Flags().StringSliceVar(&capabilities, "capability", nil, "Filter by capability (e.g. chat, embeddings)")

	return cmd
}

// --- List Model Versions ---

func newAiListVersionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-versions",
		Short: "List versions for a model (AiModelService.ListModelVersions).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			locationResp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subscriptionId},
				},
			})
			if err != nil {
				return err
			}

			modelNameResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:  "Enter model name (e.g. gpt-4o)",
					Required: true,
				},
			})
			if err != nil {
				return err
			}

			resp, err := azdClient.AiModel().ListModelVersions(ctx, &azdext.ListModelVersionsRequest{
				SubscriptionId: subscriptionId,
				ModelName:      modelNameResp.Value,
				Location:       locationResp.Location.Name,
			})
			if err != nil {
				return fmt.Errorf("listing versions: %w", err)
			}

			fmt.Printf("\nVersions for %s in %s:\n",
				color.HiCyanString(modelNameResp.Value),
				color.HiWhiteString(locationResp.Location.Name))
			for _, v := range resp.Versions {
				marker := ""
				if v == resp.DefaultVersion {
					marker = color.HiGreenString(" (default)")
				}
				fmt.Printf("  • %s%s\n", v, marker)
			}
			return nil
		},
	}
}

// --- List Model SKUs ---

func newAiListSkusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-skus",
		Short: "List SKUs for a model+version (AiModelService.ListModelSkus).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			locationResp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subscriptionId},
				},
			})
			if err != nil {
				return err
			}

			modelNameResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:      "Enter model name",
					Required:     true,
					DefaultValue: "gpt-4o",
				},
			})
			if err != nil {
				return err
			}

			versionResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:  "Enter model version",
					Required: true,
				},
			})
			if err != nil {
				return err
			}

			resp, err := azdClient.AiModel().ListModelSkus(ctx, &azdext.ListModelSkusRequest{
				SubscriptionId: subscriptionId,
				ModelName:      modelNameResp.Value,
				Location:       locationResp.Location.Name,
				Version:        versionResp.Value,
			})
			if err != nil {
				return fmt.Errorf("listing SKUs: %w", err)
			}

			fmt.Printf("\nSKUs for %s v%s in %s:\n",
				color.HiCyanString(modelNameResp.Value),
				versionResp.Value,
				color.HiWhiteString(locationResp.Location.Name))
			for _, sku := range resp.Skus {
				fmt.Printf("  • %s\n", sku)
			}
			return nil
		},
	}
}

// --- Get Model Deployment ---

func newAiGetDeploymentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get-deployment",
		Short: "Resolve deployment config for a model (AiModelService.GetModelDeployment).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			modelNameResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:      "Enter model name",
					Required:     true,
					DefaultValue: "gpt-4o",
				},
			})
			if err != nil {
				return err
			}

			fmt.Println()
			color.Cyan("Resolving deployment configuration...")
			resp, err := azdClient.AiModel().GetModelDeployment(ctx, &azdext.GetModelDeploymentRequest{
				SubscriptionId: subscriptionId,
				ModelName:      modelNameResp.Value,
				PreferredSkus:  []string{"GlobalStandard", "Standard"},
			})
			if err != nil {
				return fmt.Errorf("getting deployment: %w", err)
			}

			fmt.Println()
			color.Cyan("Resolved deployment configuration:")
			printKeyValue("Model", resp.Name)
			printKeyValue("Format", resp.Format)
			printKeyValue("Version", resp.Version)
			printKeyValue("Location", resp.Location)
			printKeyValue("SKU", resp.Sku.Name)
			printKeyValue("Usage Name", resp.Sku.UsageName)
			printKeyValue("Capacity", fmt.Sprintf("%d", resp.Sku.Capacity))
			return nil
		},
	}
}

// --- List Usages ---

func newAiListUsagesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-usages",
		Short: "List AI quota/usage in a location (AiModelService.ListUsages).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			locationResp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subscriptionId},
				},
			})
			if err != nil {
				return err
			}

			fmt.Println()
			color.Cyan("Fetching quota/usage for %s...", locationResp.Location.Name)
			resp, err := azdClient.AiModel().ListUsages(ctx, &azdext.ListUsagesRequest{
				SubscriptionId: subscriptionId,
				Location:       locationResp.Location.Name,
			})
			if err != nil {
				return fmt.Errorf("listing usages: %w", err)
			}

			fmt.Printf("\nQuota/Usage in %s (%d entries):\n\n",
				color.HiWhiteString(locationResp.Location.DisplayName), len(resp.Usages))
			for _, u := range resp.Usages {
				remaining := u.Limit - u.CurrentValue
				usageColor := color.HiGreenString
				if remaining <= 0 {
					usageColor = color.HiRedString
				} else if remaining < u.Limit*0.2 {
					usageColor = color.HiYellowString
				}
				fmt.Printf("  %-50s  %s / %.0f  (remaining: %s)\n",
					u.Name,
					fmt.Sprintf("%.0f", u.CurrentValue),
					u.Limit,
					usageColor("%.0f", remaining))
			}
			return nil
		},
	}
}

// --- Locations With Quota ---

func newAiLocationsWithQuotaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "locations-with-quota",
		Short: "Find locations with sufficient quota (AiModelService.ListLocationsWithQuota).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			usageNameResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:      "Enter usage name to check (e.g. OpenAI.Standard.gpt-4o)",
					Required:     true,
					DefaultValue: "OpenAI.Standard.gpt-4o",
				},
			})
			if err != nil {
				return err
			}

			capacityResp, err := azdClient.Prompt().Prompt(ctx, &azdext.PromptRequest{
				Options: &azdext.PromptOptions{
					Message:      "Minimum required capacity",
					Required:     true,
					DefaultValue: "10",
				},
			})
			if err != nil {
				return err
			}

			var capacity float64
			fmt.Sscanf(capacityResp.Value, "%f", &capacity)

			fmt.Println()
			color.Cyan("Checking quota across locations (this may take a moment)...")
			resp, err := azdClient.AiModel().ListLocationsWithQuota(ctx, &azdext.ListLocationsWithQuotaRequest{
				SubscriptionId: subscriptionId,
				Requirements: []*azdext.QuotaRequirement{
					{UsageName: usageNameResp.Value, Capacity: capacity},
				},
			})
			if err != nil {
				return fmt.Errorf("checking quota: %w", err)
			}

			if len(resp.Locations) == 0 {
				color.Yellow("\nNo locations found with sufficient quota for %s (%.0f)",
					usageNameResp.Value, capacity)
			} else {
				fmt.Printf("\n%s locations with sufficient quota:\n\n",
					color.HiWhiteString("%d", len(resp.Locations)))
				for _, loc := range resp.Locations {
					fmt.Printf("  • %s\n", color.HiGreenString(loc))
				}
			}
			return nil
		},
	}
}

// --- SKU Locations ---

func newAiSkuLocationsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sku-locations",
		Short: "List locations for an AI resource SKU (AiModelService.ListSkuLocations).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			fmt.Println()
			color.Cyan("Fetching AI Services (S0/Standard) locations...")
			resp, err := azdClient.AiModel().ListSkuLocations(ctx, &azdext.ListSkuLocationsRequest{
				SubscriptionId: subscriptionId,
				Kind:           "AIServices",
				SkuName:        "S0",
				Tier:           "Standard",
				ResourceType:   "accounts",
			})
			if err != nil {
				return fmt.Errorf("listing SKU locations: %w", err)
			}

			fmt.Printf("\nAI Services (S0/Standard) available in %s locations:\n\n",
				color.HiWhiteString("%d", len(resp.Locations)))
			for _, loc := range resp.Locations {
				fmt.Printf("  • %s\n", loc)
			}
			return nil
		},
	}
}

// --- Prompt AI Model (PromptService.PromptAiModel) ---

func newAiPromptModelCommand() *cobra.Command {
	var kinds []string

	cmd := &cobra.Command{
		Use:   "prompt-model",
		Short: "Interactive model selection (PromptService.PromptAiModel).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			locationResp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subscriptionId},
				},
			})
			if err != nil {
				return err
			}

			resp, err := azdClient.Prompt().PromptAiModel(ctx, &azdext.PromptAiModelRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{
						SubscriptionId: subscriptionId,
						Location:       locationResp.Location.Name,
					},
				},
				Filter: &azdext.AiModelFilterOptions{
					Kinds: kinds,
				},
			})
			if err != nil {
				return fmt.Errorf("prompting model: %w", err)
			}

			fmt.Println()
			color.Cyan("Selected model:")
			printKeyValue("Model", resp.ModelName)
			printKeyValue("Location", resp.Location)
			if resp.LocationChanged {
				color.Yellow("  (location was changed during selection)")
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "Filter by kind (e.g. OpenAI, AIServices)")

	return cmd
}

// --- Prompt AI Model Deployment (PromptService.PromptAiModelDeployment) ---

func newAiPromptDeploymentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt-deployment",
		Short: "Interactive deployment config wizard (PromptService.PromptAiModelDeployment).",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, azdClient, err := initClient(cmd)
			if err != nil || azdClient == nil {
				return err
			}
			defer azdClient.Close()

			subscriptionId, err := promptSubscription(ctx, azdClient)
			if err != nil {
				return err
			}

			locationResp, err := azdClient.Prompt().PromptLocation(ctx, &azdext.PromptLocationRequest{
				AzureContext: &azdext.AzureContext{
					Scope: &azdext.AzureScope{SubscriptionId: subscriptionId},
				},
			})
			if err != nil {
				return err
			}

			azureContext := &azdext.AzureContext{
				Scope: &azdext.AzureScope{
					SubscriptionId: subscriptionId,
					Location:       locationResp.Location.Name,
				},
			}

			// First select a model using PromptAiModel
			modelResp, err := azdClient.Prompt().PromptAiModel(ctx, &azdext.PromptAiModelRequest{
				AzureContext: azureContext,
			})
			if err != nil {
				return fmt.Errorf("prompting model: %w", err)
			}

			// Then configure deployment using PromptAiModelDeployment
			deployResp, err := azdClient.Prompt().PromptAiModelDeployment(ctx, &azdext.PromptAiModelDeploymentRequest{
				AzureContext:  azureContext,
				ModelName:     modelResp.ModelName,
				PreferredSkus: []string{"GlobalStandard", "DataZoneStandard", "Standard"},
			})
			if err != nil {
				return fmt.Errorf("prompting deployment: %w", err)
			}

			fmt.Println()
			color.Cyan("Deployment configuration:")
			printKeyValue("Model", deployResp.ModelName)
			printKeyValue("Format", deployResp.Format)
			printKeyValue("Version", deployResp.Version)
			printKeyValue("SKU", deployResp.SkuName)
			printKeyValue("Usage Name", deployResp.SkuUsageName)
			printKeyValue("Capacity", fmt.Sprintf("%d", deployResp.Capacity))
			return nil
		},
	}
}

func printKeyValue(key, value string) {
	if value == "" {
		value = "N/A"
	}
	fmt.Printf("  %s: %s\n",
		color.HiWhiteString("%-12s", key),
		color.HiBlackString(strings.TrimSpace(value)))
}
