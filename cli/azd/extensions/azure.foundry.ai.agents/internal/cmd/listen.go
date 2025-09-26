// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"fmt"

	"azure.foundry.ai.agents/internal/project"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newListenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "listen",
		Short: "Starts the extension and listens for events.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create a new context that includes the AZD access token.
			ctx := azdext.WithAccessToken(cmd.Context())

			// Create a new AZD client.
			azdClient, err := azdext.NewAzdClient()
			if err != nil {
				return fmt.Errorf("failed to create azd client: %w", err)
			}
			defer azdClient.Close()

			provider := project.NewAgentServiceTargetProvider(azdClient)
			serviceTargetManager := azdext.NewServiceTargetManager(azdClient)

			if err := serviceTargetManager.Register(ctx, provider, "foundryagent"); err != nil {
				return fmt.Errorf("failed to register provider: %w", err)
			}

			// This is blocking
			if _, err := azdClient.Extension().Ready(ctx, &azdext.ReadyRequest{}); err != nil {
				// Treat connection shutdowns as graceful termination.
				if status.Code(err) != codes.Canceled && status.Code(err) != codes.Unavailable {
					return fmt.Errorf("failed to signal readiness: %w (type: %T, status: %v)", err, err, status.Code(err))
				}
			}

			return nil
		},
	}
}
