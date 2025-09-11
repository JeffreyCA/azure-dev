// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"errors"
	"sync"

	"github.com/azure/azure-dev/cli/azd/pkg/async"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/prompt"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
)

type ExternalServiceTarget struct {
	extension  *extensions.Extension
	targetName string
	targetKind ServiceTargetKind
	console    input.Console
	prompters  prompt.Prompter

	stream        azdext.ServiceTargetService_StreamServer
	responseChans sync.Map
}

// NewExternalServiceTarget creates a new external service target
func NewExternalServiceTarget(
	name string,
	kind ServiceTargetKind,
	extension *extensions.Extension,
	stream azdext.ServiceTargetService_StreamServer,
	console input.Console,
	prompters prompt.Prompter,
) ServiceTarget {
	target := &ExternalServiceTarget{
		extension:  extension,
		targetName: name,
		targetKind: kind,
		console:    console,
		prompters:  prompters,
		stream:     stream,
	}

	target.startResponseDispatcher()

	return target
}

// Initialize initializes the service target for the specified service configuration.
// This allows service targets to opt-in to service lifecycle events
func (est *ExternalServiceTarget) Initialize(ctx context.Context, serviceConfig *ServiceConfig) error {
	// For now, return no-op since ServiceTarget proto doesn't have all the fields we need
	// TODO: Implement gRPC call when ServiceTarget proto supports service configuration
	return nil
}

// RequiredExternalTools returns the tools needed to run the deploy operation for this target.
func (est *ExternalServiceTarget) RequiredExternalTools(ctx context.Context, serviceConfig *ServiceConfig) []tools.ExternalTool {
	// No-op implementation - return empty slice
	return []tools.ExternalTool{}
}

// Package prepares artifacts for deployment
func (est *ExternalServiceTarget) Package(
	ctx context.Context,
	serviceConfig *ServiceConfig,
	frameworkPackageOutput *ServicePackageResult,
	progress *async.Progress[ServiceProgress],
) (*ServicePackageResult, error) {
	// No-op implementation - ServiceTarget proto doesn't support Package operation yet
	// TODO: Implement gRPC call when ServiceTarget proto supports Package
	return frameworkPackageOutput, nil
}

// Deploy deploys the given deployment artifact to the target resource
func (est *ExternalServiceTarget) Deploy(
	ctx context.Context,
	serviceConfig *ServiceConfig,
	servicePackage *ServicePackageResult,
	targetResource *environment.TargetResource,
	progress *async.Progress[ServiceProgress],
) (*ServiceDeployResult, error) {
	// No-op implementation - ServiceTarget proto doesn't support Deploy operation yet
	// TODO: Implement gRPC call when ServiceTarget proto supports Deploy
	// Construct resource ID from available methods
	resourceId := "/subscriptions/" + targetResource.SubscriptionId() + "/resourceGroups/" + targetResource.ResourceGroupName() + "/providers/" + targetResource.ResourceType() + "/" + targetResource.ResourceName()
	return &ServiceDeployResult{
		Package:          servicePackage,
		TargetResourceId: resourceId,
		Kind:             est.targetKind,
		Endpoints:        []string{},
		Details:          "External service target (no-op)",
	}, nil
}

// Endpoints gets the endpoints a service exposes.
func (est *ExternalServiceTarget) Endpoints(
	ctx context.Context,
	serviceConfig *ServiceConfig,
	targetResource *environment.TargetResource,
) ([]string, error) {
	// No-op implementation - return empty endpoints
	return []string{}, nil
}

// Private methods for gRPC communication

// helper to send a request and wait for the matching response
func (est *ExternalServiceTarget) sendAndWait(ctx context.Context, req *azdext.ServiceTargetMessage, match func(*azdext.ServiceTargetMessage) bool) (*azdext.ServiceTargetMessage, error) {
	ch := make(chan *azdext.ServiceTargetMessage, 1)
	est.responseChans.Store(req.RequestId, ch)
	defer est.responseChans.Delete(req.RequestId)

	if err := est.stream.Send(req); err != nil {
		return nil, err
	}

	for {
		select {
		case resp := <-ch:
			if match(resp) {
				if resp.Error != nil && resp.Error.Message != "" {
					return nil, errors.New(resp.Error.Message)
				}
				return resp, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// goroutine to receive and dispatch responses
func (est *ExternalServiceTarget) startResponseDispatcher() {
	go func() {
		for {
			resp, err := est.stream.Recv()
			if err != nil {
				// propagate error to all waiting calls
				est.responseChans.Range(func(key, value any) bool {
					ch := value.(chan *azdext.ServiceTargetMessage)
					close(ch)
					return true
				})
				return
			}
			if ch, ok := est.responseChans.Load(resp.RequestId); ok {
				ch.(chan *azdext.ServiceTargetMessage) <- resp
			}
		}
	}()
}

func (est *ExternalServiceTarget) wireConsole() func() {
	stdOut := est.extension.StdOut()
	stdErr := est.extension.StdErr()
	stdOut.AddWriter(est.console.Handles().Stdout)
	stdErr.AddWriter(est.console.Handles().Stderr)

	return func() {
		stdOut.RemoveWriter(est.console.Handles().Stdout)
		stdErr.RemoveWriter(est.console.Handles().Stderr)
	}
}
