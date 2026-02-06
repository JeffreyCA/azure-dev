// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

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

func boolPtr(value bool) *bool {
	return &value
}
