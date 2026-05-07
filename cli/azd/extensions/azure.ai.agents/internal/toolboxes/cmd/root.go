// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/spf13/cobra"
)

func NewToolboxesRootCommand(extCtx *azdext.ExtensionContext) *cobra.Command {
	extCtx = ensureExtensionContext(extCtx)

	cmd := &cobra.Command{
		Use:   "toolboxes",
		Short: "Manage AI toolboxes.",
	}

	cmd.AddCommand(newVersionCommand())

	cmd.AddCommand(newCreateCommand(extCtx))

	return cmd
}
