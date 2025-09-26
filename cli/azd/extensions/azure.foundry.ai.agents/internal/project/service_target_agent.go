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
	azdClient     *azdext.AzdClient
	serviceConfig *azdext.ServiceTargetConfig
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

// Initialize initializes the service target provider with service configuration
func (p *AgentServiceTargetProvider) Initialize(ctx context.Context, serviceConfig *azdext.ServiceTargetConfig) error {
	name := ""
	if serviceConfig != nil {
		name = serviceConfig.GetName()
	}
	agentPrintf("[AgentServiceTarget] Initialize() called for service: %s", name)
	p.serviceConfig = serviceConfig
	return nil
}

// Endpoints returns endpoints exposed by the agent service
func (p *AgentServiceTargetProvider) Endpoints(
	ctx context.Context,
	serviceConfig *azdext.ServiceTargetConfig,
	targetResource *azdext.TargetResource,
) ([]string, error) {
	agentPrintf("[AgentServiceTarget] Endpoints() called for service: %s", serviceConfig.Name)
	return []string{
		fmt.Sprintf("https://%s.%s.azurecontainerapps.io/api", targetResource.ResourceName, "region"),
	}, nil
}

// GetTargetResource returns a custom target resource for the agent service
func (p *AgentServiceTargetProvider) GetTargetResource(ctx context.Context, subscriptionId string, serviceConfig *azdext.ServiceTargetConfig) (*azdext.TargetResource, error) {
	agentPrintf("[AgentServiceTarget] GetTargetResource() called for service: %s", serviceConfig.Name)

	targetResource := &azdext.TargetResource{
		SubscriptionId:    subscriptionId,
		ResourceGroupName: "rg-agent-demo",
		ResourceName:      "ca-" + serviceConfig.Name + "-agent",
		ResourceType:      "Microsoft.App/containerApps",
		Metadata: map[string]string{
			"agentId":   "asst_xYZ",
			"agentName": "Agent 007",
		},
	}

	agentPrintf("[AgentServiceTarget] Returning target resource: %+v", targetResource)
	return targetResource, nil
}

// Package performs packaging for the agent service
func (p *AgentServiceTargetProvider) Package(
	ctx context.Context,
	serviceConfig *azdext.ServiceTargetConfig,
	frameworkPackage *azdext.ServicePackageResult,
	progress azdext.ProgressReporter,
) (*azdext.ServicePackageResult, error) {
	agentPrintf("[AgentServiceTarget] Package() called for service: %s", serviceConfig.Name)
	progress("Validating framework package output")
	time.Sleep(400 * time.Millisecond)
	progress("Preparing agent package artifacts")
	time.Sleep(600 * time.Millisecond)

	// packagePath := frameworkPackage.GetPackagePath()
	// if packagePath == "" {
	// 	packagePath = fmt.Sprintf("/tmp/%s-agent.zip", serviceConfig.Name)
	// }
	packagePath := "agent-aca/app:azd-deploy-1758834482"

	return &azdext.ServicePackageResult{
		PackagePath: packagePath,
		Details: map[string]string{
			"packagedBy": "agent-extension",
			"timestamp":  time.Now().Format(time.RFC3339),
		},
	}, nil
}

// Publish performs the publish operation for the agent service
func (p *AgentServiceTargetProvider) Publish(
	ctx context.Context,
	serviceConfig *azdext.ServiceTargetConfig,
	servicePackage *azdext.ServicePackageResult,
	targetResource *azdext.TargetResource,
	progress azdext.ProgressReporter,
) (*azdext.ServicePublishResult, error) {
	agentPrintf("[AgentServiceTarget] Publish() called for service: %s", serviceConfig.Name)
	progress("Pushing artifacts to agent registry")
	time.Sleep(700 * time.Millisecond)
	progress("Configuring publish metadata")
	time.Sleep(500 * time.Millisecond)

	return &azdext.ServicePublishResult{
		Details: map[string]string{
			"remoteImage": fmt.Sprintf("contoso.azurecr.io/%s-agent:latest", serviceConfig.Name),
			"packagePath": servicePackage.GetPackagePath(),
		},
	}, nil
}

// Deploy performs the deployment operation for the agent service
func (p *AgentServiceTargetProvider) Deploy(
	ctx context.Context,
	serviceConfig *azdext.ServiceTargetConfig,
	servicePackage *azdext.ServicePackageResult,
	servicePublish *azdext.ServicePublishResult,
	targetResource *azdext.TargetResource,
	progress azdext.ProgressReporter,
) (*azdext.ServiceDeployResult, error) {
	agentPrintf("[AgentServiceTarget] Deploy() called for service: %s", serviceConfig.Name)
	agentPrintf("[AgentServiceTarget] Package path: %s", servicePackage.PackagePath)
	agentPrintf("[AgentServiceTarget] Target resource: %s", targetResource.ResourceName)
	if servicePublish != nil {
		agentPrintf("[AgentServiceTarget] Publish details: %+v", servicePublish.Details)
	}

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

	// Resolve endpoints
	endpoints, err := p.Endpoints(ctx, serviceConfig, targetResource)
	if err != nil {
		return nil, err
	}

	// Return deployment result
	deployResult := &azdext.ServiceDeployResult{
		TargetResourceId: resourceId,
		Kind:             "agent",
		Endpoints:        endpoints,
		Details: map[string]string{
			"message": "Agent service deployed successfully using custom extension logic",
		},
	}

	agentPrintf("\n\n[AgentServiceTarget] Returning deploy result: %+v", deployResult)
	return deployResult, nil
}
