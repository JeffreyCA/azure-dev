// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

// Ensure AgentServiceTargetProvider implements ServiceTargetProvider interface
var _ azdext.ServiceTargetProvider = &AgentServiceTargetProvider{}

// AgentServiceTargetProvider is a minimal implementation of ServiceTargetProvider for demonstration
type AgentServiceTargetProvider struct {
	azdClient   *azdext.AzdClient
	projectPath string
	options     *azdext.ServiceTargetOptions
	logger      *log.Logger
}

// NewAgentServiceTargetProvider creates a new AgentServiceTargetProvider instance
func NewAgentServiceTargetProvider(azdClient *azdext.AzdClient) azdext.ServiceTargetProvider {
	// Create log file in a temp directory
	logDir := filepath.Join("/workspaces/azure-dev/cli/azd/extensions/microsoft.agents", "logs")
	os.MkdirAll(logDir, 0755)

	logFile, err := os.OpenFile(
		filepath.Join(logDir, "service_target_agent.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		// Fallback to stdout if file creation fails
		logFile = os.Stdout
	}

	logger := log.New(logFile, "[AgentServiceTarget] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("AgentServiceTargetProvider created, logging to: %s", logFile.Name())

	return &AgentServiceTargetProvider{
		azdClient: azdClient,
		logger:    logger,
	}
}

// Name returns the name of this service target provider
func (p *AgentServiceTargetProvider) Name(ctx context.Context) (string, error) {
	p.logger.Println("Name() called")
	return "agent", nil
}

// Initialize initializes the service target provider with project path and options
func (p *AgentServiceTargetProvider) Initialize(ctx context.Context, projectPath string, options *azdext.ServiceTargetOptions) error {
	p.logger.Printf("Initialize() called with projectPath: %s", projectPath)
	p.projectPath = projectPath
	p.options = options
	return nil
}

// State returns the current state of the service target
func (p *AgentServiceTargetProvider) State(ctx context.Context, options *azdext.ServiceTargetStateOptions) (*azdext.ServiceTargetStateResult, error) {
	p.logger.Println("State() called")

	// Return a minimal state result
	state := &azdext.ServiceTargetState{
		Outputs:   make(map[string]*azdext.ServiceTargetOutputParameter),
		Resources: []*azdext.ServiceTargetResource{},
	}

	return &azdext.ServiceTargetStateResult{
		State: state,
	}, nil
}

// GetTargetResource returns a custom target resource for the agent service
func (p *AgentServiceTargetProvider) GetTargetResource(ctx context.Context, subscriptionId string, serviceConfig *azdext.ServiceTargetConfig) (*azdext.TargetResource, error) {
	p.logger.Printf("GetTargetResource() called for service: %s", serviceConfig.Name)

	// This is a sample implementation that creates a mock target resource
	// In a real implementation, this would contain the custom logic for resolving
	// the target resource based on the extension's specific requirements

	// For demonstration, create a mock Container App target resource
	targetResource := &azdext.TargetResource{
		SubscriptionId:    subscriptionId,
		ResourceGroupName: "rg-agent-demo",
		ResourceName:      "ca-" + serviceConfig.Name + "-agent",
		ResourceType:      "Microsoft.App/containerApps",
	}

	p.logger.Printf("Returning target resource: %+v", targetResource)
	return targetResource, nil
}

// Deploy performs the deployment operation for the agent service
func (p *AgentServiceTargetProvider) Deploy(ctx context.Context, serviceConfig *azdext.ServiceTargetConfig, servicePackage *azdext.ServiceTargetPackageResult, targetResource *azdext.TargetResource) (*azdext.ServiceTargetDeployResult, error) {
	p.logger.Printf("Deploy() called for service: %s", serviceConfig.Name)
	p.logger.Printf("Package path: %s", servicePackage.PackagePath)
	p.logger.Printf("Target resource: %s", targetResource.ResourceName)

	// This is a sample implementation that simulates a deployment
	// In a real implementation, this would contain the custom logic for deploying
	// the service to the target resource (e.g., uploading container image, updating configuration, etc.)

	// Construct resource ID
	resourceId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s",
		targetResource.SubscriptionId,
		targetResource.ResourceGroupName,
		targetResource.ResourceType,
		targetResource.ResourceName)

	// Mock endpoint generation
	endpoints := []string{
		fmt.Sprintf("https://%s.%s.azurecontainerapps.io", targetResource.ResourceName, "region"),
	}

	// Return deployment result
	deployResult := &azdext.ServiceTargetDeployResult{
		Package: &azdext.ServiceTargetPackageResult{
			PackagePath: servicePackage.PackagePath,
			Details:     servicePackage.Details,
		},
		TargetResourceId: resourceId,
		Kind:             "agent",
		Endpoints:        endpoints,
		Details:          "Agent service deployed successfully using custom extension logic",
	}

	p.logger.Printf("Returning deploy result: %+v", deployResult)
	return deployResult, nil
}
