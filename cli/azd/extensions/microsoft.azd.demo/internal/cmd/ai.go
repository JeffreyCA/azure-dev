// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import "github.com/spf13/cobra"

func newAiCommand() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "Examples of AI extension framework capabilities.",
	}

	aiCmd.AddCommand(newAiCatalogCommand())
	aiCmd.AddCommand(newAiUsagesCommand())
	aiCmd.AddCommand(newAiDeploymentCommand())
	aiCmd.AddCommand(newAiQuotaCommand())

	return aiCmd
}
