// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

func newAiDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Interactively select a single AI deployment configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				scope, err := promptSubscriptionScope(ctx, azdClient)
				if err != nil {
					return err
				}

				selectionMode := azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_LOCATION_FIRST
				modelFirstResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Focus deployment selection on model first?",
						DefaultValue: boolPtr(false),
					},
				})
				if err != nil {
					return err
				}
				if modelFirstResp.GetValue() {
					selectionMode = azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_MODEL_FIRST
				}

				allowedLocations := []string{}
				requirements := []*azdext.AiUsageRequirement{}
				includeQuotaResp, err := azdClient.Prompt().Confirm(ctx, &azdext.ConfirmRequest{
					Options: &azdext.ConfirmOptions{
						Message:      "Include quota requirements while selecting deployment?",
						DefaultValue: boolPtr(false),
					},
				})
				if err != nil {
					return err
				}
				if includeQuotaResp.GetValue() {
					selectedLocation, err := promptLocationForScope(ctx, azdClient, scope)
					if err != nil {
						return err
					}

					usageResp, err := azdClient.Ai().ListUsages(ctx, &azdext.AiListUsagesRequest{
						SubscriptionId: scope.SubscriptionId,
						Location:       selectedLocation,
					})
					if err != nil {
						return err
					}
					if len(usageResp.GetUsages()) == 0 {
						return fmt.Errorf("no usage meters found in location '%s'", selectedLocation)
					}

					requirements, err = promptQuotaRequirements(ctx, azdClient, usageResp.GetUsages())
					if err != nil {
						return err
					}

					if selectionMode == azdext.AiDeploymentSelectionMode_AI_DEPLOYMENT_SELECTION_MODE_LOCATION_FIRST {
						allowedLocations = []string{selectedLocation}
					}
				}

				resp, err := azdClient.Prompt().PromptAiDeployment(ctx, &azdext.PromptAiDeploymentRequest{
					AzureContext: &azdext.AzureContext{
						Scope: scope,
					},
					AllowedLocations: allowedLocations,
					Requirements:     requirements,
					LocationMessage:  "Select an Azure location for AI deployment:",
					ModelMessage:     "Select an AI model:",
					SelectionMode:    selectionMode,
				})
				if err != nil {
					return err
				}

				model := resp.GetModel()
				if model == nil {
					return fmt.Errorf("no AI deployment selection returned")
				}

				fmt.Println("Selected deployment:")
				printAiModelSelection(model)

				return nil
			})
		},
	}

	return cmd
}
