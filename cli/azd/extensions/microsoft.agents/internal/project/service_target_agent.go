// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"fmt"
	"time"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

// Reference implementation

// Ensure AgentServiceTargetProvider implements ServiceTargetProvider interface
var _ azdext.ServiceTargetProvider = &AgentServiceTargetProvider{}

// AgentServiceTargetProvider is a minimal implementation of ServiceTargetProvider for demonstration
type AgentServiceTargetProvider struct {
	azdClient   *azdext.AzdClient
	projectPath string
	options     *azdext.ServiceTargetOptions
}

func agentPrintf(format string, args ...any) {
	fmt.Printf("\n"+format+"\n", args...)
}

// NewAgentServiceTargetProvider creates a new AgentServiceTargetProvider instance
func NewAgentServiceTargetProvider(azdClient *azdext.AzdClient) azdext.ServiceTargetProvider {
	agentPrintf("[AgentServiceTarget] AgentServiceTargetProvider created")

	return &AgentServiceTargetProvider{
		azdClient: azdClient,
	}
}

// Name returns the name of this service target provider
func (p *AgentServiceTargetProvider) Name(ctx context.Context) (string, error) {
	agentPrintf("[AgentServiceTarget] Name() called")
	return "agent", nil
}

// Initialize initializes the service target provider with project path and options
func (p *AgentServiceTargetProvider) Initialize(ctx context.Context, projectPath string, options *azdext.ServiceTargetOptions) error {
	agentPrintf("[AgentServiceTarget] Initialize() called with projectPath: %s", projectPath)
	p.projectPath = projectPath
	p.options = options
	return nil
}

// State returns the current state of the service target
func (p *AgentServiceTargetProvider) State(ctx context.Context, options *azdext.ServiceTargetStateOptions) (*azdext.ServiceTargetStateResult, error) {
	agentPrintf("[AgentServiceTarget] State() called")

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
	agentPrintf("[AgentServiceTarget] GetTargetResource() called for service: %s", serviceConfig.Name)

	// This is a sample implementation that creates a mock target resource
	// In a real implementation, this would contain the custom logic for resolving
	// the target resource based on the extension's specific requirements

	// For demonstration, create a mock Container App target resource
	targetResource := &azdext.TargetResource{
		SubscriptionId:    subscriptionId,
		ResourceGroupName: "rg-agent-demo",
		ResourceName:      "ca-" + serviceConfig.Name + "-agent",
		ResourceType:      "Microsoft.App/containerApps",
		Metadata: map[string]string{
			"agentId":       "agent-" + serviceConfig.Name,
			"agentEndpoint": fmt.Sprintf("https://%s.agents.ai.azure.com", serviceConfig.Name),
		},
	}

	agentPrintf("[AgentServiceTarget] Returning target resource: %+v", targetResource)
	return targetResource, nil
}

// Deploy performs the deployment operation for the agent service
func (p *AgentServiceTargetProvider) Deploy(ctx context.Context, serviceConfig *azdext.ServiceTargetConfig, servicePackage *azdext.ServiceTargetPackageResult, targetResource *azdext.TargetResource, progress azdext.ProgressReporter) (*azdext.ServiceTargetDeployResult, error) {
	agentPrintf("[AgentServiceTarget] Deploy() called for service: %s", serviceConfig.Name)
	agentPrintf("[AgentServiceTarget] Package path: %s", servicePackage.PackagePath)
	agentPrintf("[AgentServiceTarget] Target resource: %s", targetResource.ResourceName)

	// This is a sample implementation that simulates a deployment with progress updates
	// In a real implementation, this would contain the custom logic for deploying
	// the service to the target resource (e.g., uploading container image, updating configuration, etc.)

	// Step 1: Validate configuration
	progress("Validating service configuration")
	time.Sleep(500 * time.Millisecond) // Simulate work

	// Step 2: Prepare deployment artifacts
	progress("Preparing deployment artifacts")
	time.Sleep(800 * time.Millisecond) // Simulate work

	// Step 3: Upload artifacts
	progress("Uploading artifacts to Azure")
	time.Sleep(1200 * time.Millisecond) // Simulate work

	// Step 4: Configure target resource
	progress("Configuring target resource")
	time.Sleep(600 * time.Millisecond) // Simulate work

	// Step 5: Deploy to target
	progress("Deploying service to target resource")
	time.Sleep(1000 * time.Millisecond) // Simulate work

	// Step 6: Verify deployment
	progress("Verifying deployment health")
	time.Sleep(400 * time.Millisecond) // Simulate work

	// Construct resource ID
	resourceId := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/%s/%s",
		targetResource.SubscriptionId,
		targetResource.ResourceGroupName,
		targetResource.ResourceType,
		targetResource.ResourceName)

	// Return deployment result
	deployResult := &azdext.ServiceTargetDeployResult{
		Package: &azdext.ServiceTargetPackageResult{
			PackagePath: servicePackage.PackagePath,
			Details:     servicePackage.Details,
		},
		TargetResourceId: resourceId,
		Kind:             "agent",
		Endpoints: []string{
			fmt.Sprintf("https://%s.%s.azurecontainerapps.io", targetResource.ResourceName, "region"),
			// "https://foo.bar.azurecontainerapps.io",
		},
		Details: "Agent service deployed successfully using custom extension logic",
	}

	agentPrintf("\n\n[AgentServiceTarget] Returning deploy result: %+v", deployResult)
	return deployResult, nil
}
