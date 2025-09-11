// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

// Ensure AgentServiceTargetProvider implements ServiceTargetProvider interface
var _ azdext.ServiceTargetProvider = &AgentServiceTargetProvider{}

// AgentServiceTargetProvider is a minimal implementation of ServiceTargetProvider for demonstration
type AgentServiceTargetProvider struct {
	azdClient   *azdext.AzdClient
	projectPath string
	options     *azdext.ServiceTargetOptions
}

// NewAgentServiceTargetProvider creates a new AgentServiceTargetProvider instance
func NewAgentServiceTargetProvider(azdClient *azdext.AzdClient) azdext.ServiceTargetProvider {
	return &AgentServiceTargetProvider{
		azdClient: azdClient,
	}
}

// Name returns the name of this service target provider
func (p *AgentServiceTargetProvider) Name(ctx context.Context) (string, error) {
	fmt.Println("AgentServiceTargetProvider.Name called")
	return "agent", nil
}

// Initialize initializes the service target provider with project path and options
func (p *AgentServiceTargetProvider) Initialize(ctx context.Context, projectPath string, options *azdext.ServiceTargetOptions) error {
	fmt.Printf("AgentServiceTargetProvider.Initialize called with projectPath: %s\n", projectPath)
	p.projectPath = projectPath
	p.options = options
	return nil
}

// State returns the current state of the service target
func (p *AgentServiceTargetProvider) State(ctx context.Context, options *azdext.ServiceTargetStateOptions) (*azdext.ServiceTargetStateResult, error) {
	fmt.Println("AgentServiceTargetProvider.State called")

	// Return a minimal state result
	state := &azdext.ServiceTargetState{
		Outputs:   make(map[string]*azdext.ServiceTargetOutputParameter),
		Resources: []*azdext.ServiceTargetResource{},
	}

	return &azdext.ServiceTargetStateResult{
		State: state,
	}, nil
}
