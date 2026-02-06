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
					usageMeters, err := resolveUsageMetersForPrompt(ctx, azdClient, scope.SubscriptionId, nil)
					if err != nil {
						return err
					}

					requirements, err = promptQuotaRequirements(ctx, azdClient, usageMeters)
					if err != nil {
						return err
					}
				}

				resp, err := azdClient.Prompt().PromptAiDeployment(ctx, &azdext.PromptAiDeploymentRequest{
					AzureContext: &azdext.AzureContext{
						Scope: scope,
					},
					Requirements:    requirements,
					LocationMessage: "Select an Azure location for AI deployment:",
					ModelMessage:    "Select an AI model deployment configuration:",
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
