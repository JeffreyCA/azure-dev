// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

type aiSharedFlags struct {
	subscriptionID string
}

type aiCatalogFlags struct {
	aiSharedFlags
	locations    []string
	kinds        []string
	formats      []string
	statuses     []string
	capabilities []string
}

type aiUsagesFlags struct {
	aiSharedFlags
	location   string
	namePrefix string
}

type aiQuotaFlags struct {
	aiSharedFlags
	locations    []string
	requirements []string
}

type aiPromptFlags struct {
	aiSharedFlags
	location     string
	requirements []string
	kinds        []string
	formats      []string
	statuses     []string
	capabilities []string
}

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
	flags := &aiCatalogFlags{}

	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "List AI model catalog entries exposed by azd.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				subscriptionID, err := resolveSubscriptionID(ctx, azdClient, flags.subscriptionID)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().ListModelCatalog(ctx, &azdext.AiListModelCatalogRequest{
					SubscriptionId: subscriptionID,
					Locations:      flags.locations,
					Kinds:          flags.kinds,
					Formats:        flags.formats,
					Statuses:       flags.statuses,
					Capabilities:   flags.capabilities,
				})
				if err != nil {
					return err
				}

				if len(resp.Models) == 0 {
					fmt.Println("No AI model catalog entries found.")
					return nil
				}

				for _, model := range resp.Models {
					fmt.Printf("Model: %s\n", model.Name)
					for _, location := range model.Locations {
						for _, version := range location.Versions {
							defaultVersion := ""
							if version.IsDefaultVersion {
								defaultVersion = " (default)"
							}

							if len(version.Skus) == 0 {
								fmt.Printf(
									"  - version=%s%s | location=%s | kind=%s | format=%s | status=%s\n",
									version.Version,
									defaultVersion,
									location.Location,
									version.Kind,
									version.Format,
									version.Status,
								)
								continue
							}

							for _, sku := range version.Skus {
								fmt.Printf(
									"  - version=%s%s | sku=%s | usage=%s | default_capacity=%d | location=%s\n",
									version.Version,
									defaultVersion,
									sku.Name,
									sku.UsageName,
									sku.CapacityDefault,
									location.Location,
								)
							}
						}
					}
					fmt.Println()
				}

				return nil
			})
		},
	}

	cmd.Flags().StringVar(
		&flags.subscriptionID,
		"subscription-id",
		"",
		"Azure subscription ID (defaults from current azd context)",
	)
	cmd.Flags().StringSliceVar(&flags.locations, "location", nil, "Filter by location (repeatable)")
	cmd.Flags().StringSliceVar(&flags.kinds, "kind", nil, "Filter by model kind (repeatable)")
	cmd.Flags().StringSliceVar(&flags.formats, "format", nil, "Filter by model format (repeatable)")
	cmd.Flags().StringSliceVar(&flags.statuses, "status", nil, "Filter by model lifecycle status (repeatable)")
	cmd.Flags().StringSliceVar(&flags.capabilities, "capability", nil, "Filter by model capability (repeatable)")

	return cmd
}

func newAiUsagesCommand() *cobra.Command {
	flags := &aiUsagesFlags{}

	cmd := &cobra.Command{
		Use:   "usages",
		Short: "List AI quota usage for a location.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				subscriptionID, err := resolveSubscriptionID(ctx, azdClient, flags.subscriptionID)
				if err != nil {
					return err
				}

				location, err := resolveLocation(ctx, azdClient, subscriptionID, flags.location)
				if err != nil {
					return err
				}

				resp, err := azdClient.Ai().ListUsages(ctx, &azdext.AiListUsagesRequest{
					SubscriptionId: subscriptionID,
					Location:       location,
					NamePrefix:     flags.namePrefix,
				})
				if err != nil {
					return err
				}

				if len(resp.Usages) == 0 {
					fmt.Println("No AI usage records found.")
					return nil
				}

				fmt.Printf("%-45s %-10s %-10s %-10s\n", "USAGE NAME", "CURRENT", "LIMIT", "REMAINING")
				for _, usage := range resp.Usages {
					fmt.Printf(
						"%-45s %-10.0f %-10.0f %-10.0f\n",
						usage.Name,
						usage.Current,
						usage.Limit,
						usage.Remaining,
					)
				}

				return nil
			})
		},
	}

	cmd.Flags().StringVar(
		&flags.subscriptionID,
		"subscription-id",
		"",
		"Azure subscription ID (defaults from current azd context)",
	)
	cmd.Flags().StringVar(&flags.location, "location", "", "Azure location")
	cmd.Flags().StringVar(&flags.namePrefix, "name-prefix", "", "Optional usage name prefix filter")

	return cmd
}

func newAiQuotaCommand() *cobra.Command {
	flags := &aiQuotaFlags{}

	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Find locations that satisfy AI quota requirements.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				subscriptionID, err := resolveSubscriptionID(ctx, azdClient, flags.subscriptionID)
				if err != nil {
					return err
				}

				requirements, err := resolveQuotaRequirements(ctx, azdClient, flags.requirements)
				if err != nil {
					return err
				}

				req, err := buildAiFindLocationsWithQuotaRequest(subscriptionID, flags.locations, requirements)
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

				if len(resp.Results) > 0 {
					fmt.Println("Diagnostics:")
				}
				for _, result := range resp.Results {
					if result.Error != "" {
						fmt.Printf("- %s: error=%s\n", result.Location, result.Error)
						continue
					}

					status := "matched"
					if !result.Matched {
						status = "unmatched"
					}
					fmt.Printf("- %s: %s\n", result.Location, status)
					for _, requirement := range result.Requirements {
						meetsQuota := requirement.AvailableCapacity >= float64(requirement.RequiredCapacity)
						check := "fail"
						if meetsQuota {
							check = "ok"
						}
						fmt.Printf(
							"  * %s required=%d available=%.0f (%s)\n",
							requirement.UsageName,
							requirement.RequiredCapacity,
							requirement.AvailableCapacity,
							check,
						)
					}
				}

				return nil
			})
		},
	}

	cmd.Flags().StringVar(
		&flags.subscriptionID,
		"subscription-id",
		"",
		"Azure subscription ID (defaults from current azd context)",
	)
	cmd.Flags().StringSliceVar(&flags.locations, "location", nil, "Candidate location allow-list (repeatable)")
	cmd.Flags().StringSliceVar(
		&flags.requirements,
		"require",
		nil,
		"Quota requirement in format usageName[,capacity] (repeatable)",
	)

	return cmd
}

func newAiPromptCommand() *cobra.Command {
	flags := &aiPromptFlags{}

	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Run AI location/model prompt helpers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWithAzdClient(cmd, func(ctx context.Context, azdClient *azdext.AzdClient) error {
				subscriptionID, err := resolveSubscriptionID(ctx, azdClient, flags.subscriptionID)
				if err != nil {
					return err
				}

				scope, err := resolveAzureScope(ctx, azdClient, subscriptionID, flags.location)
				if err != nil {
					return err
				}

				locationPromptReq, err := buildPromptAiLocationRequest(scope, nil, flags.requirements)
				if err != nil {
					return err
				}

				locationResp, err := azdClient.Prompt().PromptAiLocation(ctx, locationPromptReq)
				if err != nil {
					return err
				}

				modelResp, err := azdClient.Prompt().PromptAiModel(ctx, &azdext.PromptAiModelRequest{
					AzureContext: &azdext.AzureContext{
						Scope: &azdext.AzureScope{
							TenantId:       scope.TenantId,
							SubscriptionId: scope.SubscriptionId,
							Location:       locationResp.GetLocation().GetName(),
						},
					},
					Location:     locationResp.GetLocation().GetName(),
					Kinds:        flags.kinds,
					Formats:      flags.formats,
					Statuses:     flags.statuses,
					Capabilities: flags.capabilities,
				})
				if err != nil {
					return err
				}

				model := modelResp.GetModel()
				if model == nil {
					return fmt.Errorf("no AI model selected")
				}

				fmt.Println("Selection:")
				fmt.Printf("  location: %s\n", locationResp.GetLocation().GetName())
				fmt.Printf("  model: %s\n", model.GetName())
				fmt.Printf("  version: %s\n", model.GetVersion())
				fmt.Printf("  sku: %s\n", model.GetSku().GetName())
				fmt.Printf("  usage_name: %s\n", model.GetSku().GetUsageName())
				fmt.Printf("  capacity_default: %d\n", model.GetSku().GetCapacityDefault())

				return nil
			})
		},
	}

	cmd.Flags().StringVar(
		&flags.subscriptionID,
		"subscription-id",
		"",
		"Azure subscription ID (defaults from current azd context)",
	)
	cmd.Flags().StringVar(&flags.location, "location", "", "Optional starting location")
	cmd.Flags().StringSliceVar(
		&flags.requirements,
		"require",
		nil,
		"Quota requirement in format usageName[,capacity] (repeatable)",
	)
	cmd.Flags().StringSliceVar(&flags.kinds, "kind", nil, "Filter by model kind (repeatable)")
	cmd.Flags().StringSliceVar(&flags.formats, "format", nil, "Filter by model format (repeatable)")
	cmd.Flags().StringSliceVar(&flags.statuses, "status", nil, "Filter by model status (repeatable)")
	cmd.Flags().StringSliceVar(&flags.capabilities, "capability", nil, "Filter by model capability (repeatable)")

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

func resolveSubscriptionID(ctx context.Context, azdClient *azdext.AzdClient, provided string) (string, error) {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided), nil
	}

	deploymentCtx, err := azdClient.Deployment().GetDeploymentContext(ctx, &azdext.EmptyRequest{})
	if err == nil && deploymentCtx.GetAzureContext() != nil && deploymentCtx.GetAzureContext().GetScope() != nil {
		if deploymentCtx.GetAzureContext().GetScope().GetSubscriptionId() != "" {
			return deploymentCtx.GetAzureContext().GetScope().GetSubscriptionId(), nil
		}
	}

	scope, err := resolveCurrentScopeFromEnvironment(ctx, azdClient)
	if err != nil {
		return "", err
	}

	if scope.SubscriptionId == "" {
		subscriptionResponse, err := azdClient.Prompt().PromptSubscription(ctx, &azdext.PromptSubscriptionRequest{
			Message: "Select an Azure subscription for this command:",
		})
		if err != nil {
			return "", err
		}
		if subscriptionResponse.GetSubscription() == nil || subscriptionResponse.GetSubscription().GetId() == "" {
			return "", fmt.Errorf("subscription id is required")
		}

		return subscriptionResponse.GetSubscription().GetId(), nil
	}

	return scope.SubscriptionId, nil
}

func resolveLocation(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	subscriptionID string,
	provided string,
) (string, error) {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided), nil
	}

	scope, err := resolveAzureScope(ctx, azdClient, subscriptionID, "")
	if err != nil {
		return "", err
	}
	if scope.GetLocation() != "" {
		return scope.GetLocation(), nil
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

func resolveQuotaRequirements(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	provided []string,
) ([]string, error) {
	if len(provided) > 0 {
		return provided, nil
	}

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

func resolveAzureScope(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	subscriptionID string,
	locationOverride string,
) (*azdext.AzureScope, error) {
	scope, err := resolveCurrentScopeFromEnvironment(ctx, azdClient)
	if err != nil {
		return nil, err
	}

	scope.SubscriptionId = subscriptionID
	if strings.TrimSpace(locationOverride) != "" {
		scope.Location = strings.TrimSpace(locationOverride)
	}

	if scope.TenantId == "" {
		tenantResponse, err := azdClient.Account().LookupTenant(ctx, &azdext.LookupTenantRequest{
			SubscriptionId: scope.SubscriptionId,
		})
		if err == nil {
			scope.TenantId = tenantResponse.TenantId
		}
	}

	return scope, nil
}

func resolveCurrentScopeFromEnvironment(ctx context.Context, azdClient *azdext.AzdClient) (*azdext.AzureScope, error) {
	scope := &azdext.AzureScope{}

	currentEnv, err := azdClient.Environment().GetCurrent(ctx, &azdext.EmptyRequest{})
	if err != nil || currentEnv.GetEnvironment() == nil {
		return scope, nil
	}

	envValues, err := azdClient.Environment().GetValues(ctx, &azdext.GetEnvironmentRequest{
		Name: currentEnv.Environment.Name,
	})
	if err != nil {
		return scope, nil
	}

	for _, kv := range envValues.KeyValues {
		switch kv.Key {
		case "AZURE_SUBSCRIPTION_ID":
			scope.SubscriptionId = kv.Value
		case "AZURE_LOCATION":
			scope.Location = kv.Value
		case "AZURE_TENANT_ID":
			scope.TenantId = kv.Value
		}
	}

	return scope, nil
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
