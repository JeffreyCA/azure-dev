// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/prompt"
	"github.com/google/uuid"
)

// ExternalTargetResourceProvider implements target resource resolution by delegating to external extensions via gRPC.
// It maintains a map of service target kinds to their corresponding extensions and streams.
type ExternalTargetResourceProvider struct {
	console    input.Console
	prompters  prompt.Prompter
	extensions map[ServiceTargetKind]*extensionInfo
	mu         sync.RWMutex
}

type extensionInfo struct {
	extension     *extensions.Extension
	stream        azdext.ServiceTargetService_StreamServer
	responseChans sync.Map
}

// NewExternalTargetResourceProvider creates a new external target resource provider.
func NewExternalTargetResourceProvider(
	console input.Console,
	prompters prompt.Prompter,
) *ExternalTargetResourceProvider {
	return &ExternalTargetResourceProvider{
		console:    console,
		prompters:  prompters,
		extensions: make(map[ServiceTargetKind]*extensionInfo),
	}
}

// RegisterExtension registers an extension for a specific service target kind.
// This is called when an extension registers a service target provider via gRPC.
func (p *ExternalTargetResourceProvider) RegisterExtension(
	kind ServiceTargetKind,
	extension *extensions.Extension,
	stream azdext.ServiceTargetService_StreamServer,
) {
	p.mu.Lock()
	defer p.mu.Unlock()

	info := &extensionInfo{
		extension: extension,
		stream:    stream,
	}

	// Start response dispatcher for this extension
	go p.startResponseDispatcher(info)

	p.extensions[kind] = info
}

// UnregisterExtension removes an extension registration for a service target kind.
func (p *ExternalTargetResourceProvider) UnregisterExtension(kind ServiceTargetKind) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.extensions, kind)
}

// GetTargetResource resolves the target resource by delegating to the appropriate external extension.
func (p *ExternalTargetResourceProvider) GetTargetResource(
	ctx context.Context,
	subscriptionId string,
	serviceConfig *ServiceConfig,
) (*environment.TargetResource, error) {
	p.mu.RLock()
	info, exists := p.extensions[serviceConfig.Host]
	p.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no external extension registered for service target kind: %s", serviceConfig.Host)
	}

	requestId := uuid.NewString()

	// Convert ServiceConfig to proto ServiceTargetConfig
	protoServiceConfig := &azdext.ServiceTargetConfig{
		Name:        serviceConfig.Name,
		Host:        string(serviceConfig.Host),
		ProjectName: serviceConfig.Project.Name,
	}

	// Convert resource group name templates
	if !serviceConfig.ResourceGroupName.Empty() {
		// Use YAML marshaling to get the raw template string
		templateValue, _ := serviceConfig.ResourceGroupName.MarshalYAML()
		protoServiceConfig.ResourceGroupName = &azdext.ResourceGroupNameTemplate{
			Template: templateValue.(string),
			IsEmpty:  false,
		}
	} else {
		protoServiceConfig.ResourceGroupName = &azdext.ResourceGroupNameTemplate{
			IsEmpty: true,
		}
	}

	if !serviceConfig.Project.ResourceGroupName.Empty() {
		// Use YAML marshaling to get the raw template string
		templateValue, _ := serviceConfig.Project.ResourceGroupName.MarshalYAML()
		protoServiceConfig.ProjectResourceGroupName = &azdext.ResourceGroupNameTemplate{
			Template: templateValue.(string),
			IsEmpty:  false,
		}
	} else {
		protoServiceConfig.ProjectResourceGroupName = &azdext.ResourceGroupNameTemplate{
			IsEmpty: true,
		}
	}

	req := &azdext.ServiceTargetMessage{
		RequestId: requestId,
		MessageType: &azdext.ServiceTargetMessage_GetTargetResourceRequest{
			GetTargetResourceRequest: &azdext.GetTargetResourceRequest{
				SubscriptionId: subscriptionId,
				ServiceConfig:  protoServiceConfig,
			},
		},
	}

	resp, err := p.sendAndWait(ctx, info, req, func(r *azdext.ServiceTargetMessage) bool {
		return r.GetGetTargetResourceResponse() != nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get target resource from extension: %w", err)
	}

	result := resp.GetGetTargetResourceResponse()
	if result == nil || result.TargetResource == nil {
		return nil, errors.New("extension returned empty target resource response")
	}

	// Convert proto TargetResource back to environment.TargetResource
	targetResource := environment.NewTargetResource(
		result.TargetResource.SubscriptionId,
		result.TargetResource.ResourceGroupName,
		result.TargetResource.ResourceName,
		result.TargetResource.ResourceType,
	)

	return targetResource, nil
}

// CanHandle returns true if there's an external extension registered for the service target kind.
func (p *ExternalTargetResourceProvider) CanHandle(serviceConfig *ServiceConfig) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.extensions[serviceConfig.Host]
	return exists
}

// Helper methods for gRPC communication

// sendAndWait sends a request and waits for the matching response
func (p *ExternalTargetResourceProvider) sendAndWait(
	ctx context.Context,
	info *extensionInfo,
	req *azdext.ServiceTargetMessage,
	match func(*azdext.ServiceTargetMessage) bool,
) (*azdext.ServiceTargetMessage, error) {
	ch := make(chan *azdext.ServiceTargetMessage, 1)
	info.responseChans.Store(req.RequestId, ch)
	defer info.responseChans.Delete(req.RequestId)

	if err := info.stream.Send(req); err != nil {
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

// startResponseDispatcher runs a goroutine to receive and dispatch responses for an extension
func (p *ExternalTargetResourceProvider) startResponseDispatcher(info *extensionInfo) {
	for {
		resp, err := info.stream.Recv()
		if err != nil {
			// Stream is closed, propagate error to all waiting calls
			info.responseChans.Range(func(key, value any) bool {
				ch := value.(chan *azdext.ServiceTargetMessage)
				close(ch)
				return true
			})
			return
		}

		if ch, ok := info.responseChans.Load(resp.RequestId); ok {
			ch.(chan *azdext.ServiceTargetMessage) <- resp
		}
	}
}
