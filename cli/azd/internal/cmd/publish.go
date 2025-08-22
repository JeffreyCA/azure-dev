// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/alpha"
	"github.com/azure/azure-dev/cli/azd/pkg/async"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/cloud"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/environment/azdcontext"
	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/output/ux"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// PublishFlags defines flags for the publish command
type PublishFlags struct {
	All         bool
	fromPackage string
	*internal.EnvFlag
}

// Bind registers flags for publish
func (p *PublishFlags) Bind(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	p.bindCommon(local, global)
}

// bindCommon registers common flags
func (p *PublishFlags) bindCommon(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	p.EnvFlag = &internal.EnvFlag{}
	p.EnvFlag.Bind(local, global)
	local.BoolVar(
		&p.All,
		"all",
		false,
		"Publishes all services listed in "+azdcontext.ProjectFileName,
	)
	local.StringVar(
		&p.fromPackage,
		"from-package",
		"",
		"Publishes the packaged service located at the provided path.",
	)
}

// NewPublishFlags creates PublishFlags from command
func NewPublishFlags(cmd *cobra.Command, global *internal.GlobalCommandOptions) *PublishFlags {
	flags := &PublishFlags{}
	flags.Bind(cmd.Flags(), global)
	return flags
}

// NewPublishCmd creates the publish cobra command
func NewPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <service>",
		Short: "Publish your project code to Azure.",
	}
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

// PublishAction implements publish logic (stubbed)
type PublishAction struct {
	flags               *PublishFlags
	args                []string
	projectConfig       *project.ProjectConfig
	azdCtx              *azdcontext.AzdContext
	env                 *environment.Environment
	projectManager      project.ProjectManager
	serviceManager      project.ServiceManager
	resourceManager     project.ResourceManager
	accountManager      account.Manager
	azCli               *azapi.AzureClient
	portalUrlBase       string
	formatter           output.Formatter
	writer              io.Writer
	console             input.Console
	commandRunner       exec.CommandRunner
	alphaFeatureManager *alpha.FeatureManager
	importManager       *project.ImportManager
}

// NewPublishAction constructs a PublishAction with all dependencies
func NewPublishAction(
	flags *PublishFlags,
	args []string,
	projectConfig *project.ProjectConfig,
	projectManager project.ProjectManager,
	serviceManager project.ServiceManager,
	resourceManager project.ResourceManager,
	azdCtx *azdcontext.AzdContext,
	environment *environment.Environment,
	accountManager account.Manager,
	cloud *cloud.Cloud,
	azCli *azapi.AzureClient,
	commandRunner exec.CommandRunner,
	console input.Console,
	formatter output.Formatter,
	writer io.Writer,
	alphaFeatureManager *alpha.FeatureManager,
	importManager *project.ImportManager,
) actions.Action {
	return &PublishAction{
		flags:               flags,
		args:                args,
		projectConfig:       projectConfig,
		projectManager:      projectManager,
		serviceManager:      serviceManager,
		resourceManager:     resourceManager,
		azdCtx:              azdCtx,
		env:                 environment,
		accountManager:      accountManager,
		azCli:               azCli,
		portalUrlBase:       cloud.PortalUrlBase,
		commandRunner:       commandRunner,
		console:             console,
		formatter:           formatter,
		writer:              writer,
		alphaFeatureManager: alphaFeatureManager,
		importManager:       importManager,
	}
}

// Run executes the publish command (stub)
func (pa *PublishAction) Run(ctx context.Context) (*actions.ActionResult, error) {

	var targetServiceName string
	if len(pa.args) == 1 {
		targetServiceName = pa.args[0]
	}

	if pa.env.GetSubscriptionId() == "" {
		return nil, errors.New(
			"infrastructure has not been provisioned. Run `azd provision`",
		)
	}

	targetServiceName, err := getTargetServiceName(
		ctx,
		pa.projectManager,
		pa.importManager,
		pa.projectConfig,
		string(project.ServiceEventDeploy),
		targetServiceName,
		false,
	)
	if err != nil {
		return nil, err
	}

	if pa.flags.All && pa.flags.fromPackage != "" {
		return nil, errors.New(
			"'--from-package' cannot be specified when '--all' is set. Specify a specific service by passing a <service>")
	}

	if targetServiceName == "" && pa.flags.fromPackage != "" {
		return nil, errors.New(
			//nolint:lll
			"'--from-package' cannot be specified when deploying all services. Specify a specific service by passing a <service>",
		)
	}

	if err := pa.projectManager.Initialize(ctx, pa.projectConfig); err != nil {
		return nil, err
	}

	if err := pa.projectManager.EnsureServiceTargetTools(ctx, pa.projectConfig, func(svc *project.ServiceConfig) bool {
		return targetServiceName == "" || svc.Name == targetServiceName
	}); err != nil {
		return nil, err
	}

	// Command title
	pa.console.MessageUxItem(ctx, &ux.MessageTitle{
		Title: "Publishing services (azd publish)",
	})

	startTime := time.Now()

	// Use the shared service iteration helper
	helper := NewServiceIterationHelper(pa.serviceManager, pa.importManager, pa.console)
	_, err = helper.IterateServices(ctx, ServiceIterationConfig{
		ProjectConfig:     pa.projectConfig,
		TargetServiceName: targetServiceName,
		FromPackage:       pa.flags.fromPackage,
		AllServices:       pa.flags.All,
		OperationName:     "Publishing",
		ServiceFilter: func(svc *project.ServiceConfig) bool {
			// Publish only supports ContainerAppTarget services
			return svc.Host == project.ContainerAppTarget
		},
	}, func(
		ctx context.Context,
		svc *project.ServiceConfig,
		packageResult *project.ServicePackageResult,
	) (interface{}, error) {
		// Execute the publish operation
		return async.RunWithProgress(
			func(publishProgress project.ServiceProgress) {
				progressMessage := fmt.Sprintf("Publishing service %s (%s)", svc.Name, publishProgress.Message)
				pa.console.ShowSpinner(ctx, progressMessage, input.Step)
			},
			func(progress *async.Progress[project.ServiceProgress]) (interface{}, error) {
				return pa.serviceManager.Publish(ctx, svc, packageResult, progress)
			},
		)
	})

	if err != nil {
		return nil, err
	}

	return &actions.ActionResult{
		Message: &actions.ResultMessage{
			Header: fmt.Sprintf(
				"Your application was published to Azure Container Registry in %s.",
				ux.DurationAsText(since(startTime)),
			),
		},
	}, nil
}

// GetCmdPublishHelpDescription returns help description
func GetCmdPublishHelpDescription(*cobra.Command) string {
	return generateCmdHelpDescription("Publish application to Azure.", nil)
}

// GetCmdPublishHelpFooter returns help footer samples
func GetCmdPublishHelpFooter(*cobra.Command) string {
	return generateCmdHelpSamplesBlock(map[string]string{
		"Publish all services in the current project to Azure.": output.WithHighLightFormat(
			"azd publish --all",
		),
		"Publish the service named 'api' to Azure.": output.WithHighLightFormat(
			"azd publish api",
		),
		"Publish the service named 'api' from a previously generated package.": output.WithHighLightFormat(
			"azd publish api --from-package <package-path>",
		),
	})
}
