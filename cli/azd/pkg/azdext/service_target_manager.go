// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/google/uuid"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// ProgressReporter defines a function type for reporting progress updates from extensions
type ProgressReporter func(message string)

// ServiceTargetProvider defines the interface for service target logic.
type ServiceTargetProvider interface {
	Name(ctx context.Context) (string, error)
	Initialize(ctx context.Context, projectPath string, options *ServiceTargetOptions) error
	State(ctx context.Context, options *ServiceTargetStateOptions) (*ServiceTargetStateResult, error)
	GetTargetResource(ctx context.Context, subscriptionId string, serviceConfig *ServiceTargetConfig) (*TargetResource, error)
	Package(ctx context.Context, serviceConfig *ServiceTargetConfig, frameworkPackage *ServicePackageResult, progress ProgressReporter) (*ServicePackageResult, error)
	Publish(ctx context.Context, serviceConfig *ServiceTargetConfig, servicePackage *ServicePackageResult, targetResource *TargetResource, progress ProgressReporter) (*ServicePublishResult, error)
	Deploy(ctx context.Context, serviceConfig *ServiceTargetConfig, servicePackage *ServicePackageResult, servicePublish *ServicePublishResult, targetResource *TargetResource, progress ProgressReporter) (*ServiceDeployResult, error)
	Endpoints(ctx context.Context, serviceConfig *ServiceTargetConfig, targetResource *TargetResource) ([]string, error)
}

// SupportsCoreServiceTargetDelegation allows a provider to receive a core delegation helper.
type SupportsCoreServiceTargetDelegation interface {
	SetCoreDelegate(CoreServiceTargetDelegate)
}

// CoreServiceTargetDelegate exposes helpers for delegating to built-in azd service targets.
type CoreServiceTargetDelegate interface {
	Package(ctx context.Context, builtinKind string, serviceConfig *ServiceTargetConfig, frameworkPackage *ServicePackageResult) (*ServicePackageResult, error)
	Publish(ctx context.Context, builtinKind string, serviceConfig *ServiceTargetConfig, servicePackage *ServicePackageResult, targetResource *TargetResource) (*ServicePublishResult, error)
}

// ServiceTargetManager handles registration and provisioning request forwarding for a provider.
type ServiceTargetManager struct {
	client *AzdClient
	stream ServiceTargetService_StreamClient
}

// NewServiceTargetManager creates a new ServiceTargetManager for an AzdClient.
func NewServiceTargetManager(client *AzdClient) *ServiceTargetManager {
	return &ServiceTargetManager{
		client: client,
	}
}

// Register registers the provider with the server, waits for the response, then starts background handling of provisioning requests.
func (m *ServiceTargetManager) Register(ctx context.Context, provider ServiceTargetProvider, hostType string, displayName string) error {
	stream, err := m.client.ServiceTarget().Stream(ctx)
	if err != nil {
		return err
	}

	m.stream = stream

	registerReq := &ServiceTargetMessage{
		RequestId: uuid.NewString(),
		MessageType: &ServiceTargetMessage_RegisterServiceTargetRequest{
			RegisterServiceTargetRequest: &RegisterServiceTargetRequest{
				Host: hostType,
				// DisplayName: displayName,
			},
		},
	}
	if err := m.stream.Send(registerReq); err != nil {
		return err
	}

	msg, err := m.stream.Recv()
	if errors.Is(err, io.EOF) {
		log.Println("Stream closed by client")
		return nil
	}
	if err != nil {
		return err
	}

	regResponse := msg.GetRegisterServiceTargetResponse()
	if regResponse != nil {
		if delegator, ok := provider.(SupportsCoreServiceTargetDelegation); ok {
			delegator.SetCoreDelegate(newCoreServiceTargetDelegate(m.client.CoreServiceTarget()))
		}

		go m.handleServiceTargetStream(ctx, provider)
		return nil
	}

	return status.Errorf(codes.FailedPrecondition, "expected RegisterProviderResponse, got %T", msg.GetMessageType())
}

func newCoreServiceTargetDelegate(client CoreServiceTargetServiceClient) CoreServiceTargetDelegate {
	return &coreServiceTargetDelegate{client: client}
}

type coreServiceTargetDelegate struct {
	client CoreServiceTargetServiceClient
}

func (d *coreServiceTargetDelegate) Package(
	ctx context.Context,
	builtinKind string,
	serviceConfig *ServiceTargetConfig,
	frameworkPackage *ServicePackageResult,
) (*ServicePackageResult, error) {
	if serviceConfig == nil {
		return nil, fmt.Errorf("service configuration is required")
	}

	kind := builtinKind
	if kind == "" {
		kind = serviceConfig.GetHost()
	}
	if kind == "" {
		return nil, fmt.Errorf("builtin service target kind is required")
	}

	request := &CoreServiceTargetPackageRequest{
		BuiltinKind: kind,
		PackageRequest: &ServiceTargetPackageRequest{
			ServiceConfig:    serviceConfig,
			FrameworkPackage: frameworkPackage,
		},
	}

	response, err := d.client.Package(ctx, request)
	if err != nil {
		return nil, err
	}

	return response.GetPackageResult(), nil
}

func (d *coreServiceTargetDelegate) Publish(
	ctx context.Context,
	builtinKind string,
	serviceConfig *ServiceTargetConfig,
	servicePackage *ServicePackageResult,
	targetResource *TargetResource,
) (*ServicePublishResult, error) {
	if serviceConfig == nil {
		return nil, fmt.Errorf("service configuration is required")
	}

	kind := builtinKind
	if kind == "" {
		kind = serviceConfig.GetHost()
	}
	if kind == "" {
		return nil, fmt.Errorf("builtin service target kind is required")
	}

	request := &CoreServiceTargetPublishRequest{
		BuiltinKind: kind,
		PublishRequest: &ServiceTargetPublishRequest{
			ServiceConfig:  serviceConfig,
			ServicePackage: servicePackage,
			TargetResource: targetResource,
		},
	}

	response, err := d.client.Publish(ctx, request)
	if err != nil {
		return nil, err
	}

	return response.GetPublishResult(), nil
}

func (m *ServiceTargetManager) handleServiceTargetStream(ctx context.Context, provider ServiceTargetProvider) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled by caller, exiting service target stream")
			return
		default:
			msg, err := m.stream.Recv()
			if err != nil {
				log.Printf("service target stream closed: %v", err)
				return
			}
			go func(msg *ServiceTargetMessage) {
				resp := buildServiceTargetResponseMsg(ctx, provider, msg, m.stream)
				if resp != nil {
					if err := m.stream.Send(resp); err != nil {
						log.Printf("failed to send service target response: %v", err)
					}
				}
			}(msg)
		}
	}
}

func buildServiceTargetResponseMsg(ctx context.Context, provider ServiceTargetProvider, msg *ServiceTargetMessage, stream ServiceTargetService_StreamClient) *ServiceTargetMessage {
	var resp *ServiceTargetMessage
	switch r := msg.MessageType.(type) {
	// case *ServiceTargetMessage_NameRequest:
	// 	name, _ := provider.Name(ctx)
	// 	resp = &ServiceTargetMessage{
	// 		RequestId: msg.RequestId,
	// 		MessageType: &ServiceTargetMessage_NameResponse{
	// 			NameResponse: &ServiceTargetNameResponse{Name: name},
	// 		},
	// 	}
	// case *ServiceTargetMessage_InitializeRequest:
	// 	err := provider.Initialize(ctx, r.InitializeRequest.ProjectPath, r.InitializeRequest.Options)
	// 	resp = &ServiceTargetMessage{
	// 		RequestId: msg.RequestId,
	// 		MessageType: &ServiceTargetMessage_InitializeResponse{
	// 			InitializeResponse: &ServiceTargetInitializeResponse{},
	// 		},
	// 	}
	// 	if err != nil {
	// 		resp.Error = &ServiceTargetErrorMessage{
	// 			Message: err.Error(),
	// 		}
	// 	}
	// case *ServiceTargetMessage_StateRequest:
	// 	result, err := provider.State(ctx, r.StateRequest.Options)
	// 	resp = &ServiceTargetMessage{
	// 		RequestId: msg.RequestId,
	// 		MessageType: &ServiceTargetMessage_StateResponse{
	// 			StateResponse: &ServiceTargetStateResponse{StateResult: result},
	// 		},
	// 	}
	// 	if err != nil {
	// 		resp.Error = &ServiceTargetErrorMessage{
	// 			Message: err.Error(),
	// 		}
	// 	}
	case *ServiceTargetMessage_GetTargetResourceRequest:
		result, err := provider.GetTargetResource(ctx, r.GetTargetResourceRequest.SubscriptionId, r.GetTargetResourceRequest.ServiceConfig)
		resp = &ServiceTargetMessage{
			RequestId: msg.RequestId,
			MessageType: &ServiceTargetMessage_GetTargetResourceResponse{
				GetTargetResourceResponse: &GetTargetResourceResponse{TargetResource: result},
			},
		}
		if err != nil {
			resp.Error = &ServiceTargetErrorMessage{
				Message: err.Error(),
			}
		}
	case *ServiceTargetMessage_PackageRequest:
		progressReporter := func(message string) {
			progressMsg := &ServiceTargetMessage{
				RequestId: msg.RequestId,
				MessageType: &ServiceTargetMessage_ProgressMessage{
					ProgressMessage: &ServiceTargetProgressMessage{
						RequestId: msg.RequestId,
						Message:   message,
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}
			if err := stream.Send(progressMsg); err != nil {
				log.Printf("failed to send progress message: %v", err)
			}
		}

		result, err := provider.Package(ctx, r.PackageRequest.ServiceConfig, r.PackageRequest.FrameworkPackage, progressReporter)
		resp = &ServiceTargetMessage{
			RequestId: msg.RequestId,
			MessageType: &ServiceTargetMessage_PackageResponse{
				PackageResponse: &ServiceTargetPackageResponse{PackageResult: result},
			},
		}
		if err != nil {
			resp.Error = &ServiceTargetErrorMessage{
				Message: err.Error(),
			}
		}
	case *ServiceTargetMessage_PublishRequest:
		progressReporter := func(message string) {
			progressMsg := &ServiceTargetMessage{
				RequestId: msg.RequestId,
				MessageType: &ServiceTargetMessage_ProgressMessage{
					ProgressMessage: &ServiceTargetProgressMessage{
						RequestId: msg.RequestId,
						Message:   message,
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}
			if err := stream.Send(progressMsg); err != nil {
				log.Printf("failed to send progress message: %v", err)
			}
		}

		result, err := provider.Publish(ctx, r.PublishRequest.ServiceConfig, r.PublishRequest.ServicePackage, r.PublishRequest.TargetResource, progressReporter)
		resp = &ServiceTargetMessage{
			RequestId: msg.RequestId,
			MessageType: &ServiceTargetMessage_PublishResponse{
				PublishResponse: &ServiceTargetPublishResponse{PublishResult: result},
			},
		}
		if err != nil {
			resp.Error = &ServiceTargetErrorMessage{
				Message: err.Error(),
			}
		}
	case *ServiceTargetMessage_DeployRequest:
		// Create a progress reporter that sends progress messages back to core
		progressReporter := func(message string) {
			progressMsg := &ServiceTargetMessage{
				RequestId: msg.RequestId,
				MessageType: &ServiceTargetMessage_ProgressMessage{
					ProgressMessage: &ServiceTargetProgressMessage{
						RequestId: msg.RequestId,
						Message:   message,
						Timestamp: time.Now().UnixMilli(),
					},
				},
			}
			if err := stream.Send(progressMsg); err != nil {
				log.Printf("failed to send progress message: %v", err)
			}
		}

		result, err := provider.Deploy(
			ctx,
			r.DeployRequest.ServiceConfig,
			r.DeployRequest.ServicePackage,
			r.DeployRequest.ServicePublish,
			r.DeployRequest.TargetResource,
			progressReporter,
		)
		resp = &ServiceTargetMessage{
			RequestId: msg.RequestId,
			MessageType: &ServiceTargetMessage_DeployResponse{
				DeployResponse: &ServiceTargetDeployResponse{DeployResult: result},
			},
		}
		if err != nil {
			resp.Error = &ServiceTargetErrorMessage{
				Message: err.Error(),
			}
		}
	case *ServiceTargetMessage_EndpointsRequest:
		endpoints, err := provider.Endpoints(ctx, r.EndpointsRequest.ServiceConfig, r.EndpointsRequest.TargetResource)
		resp = &ServiceTargetMessage{
			RequestId: msg.RequestId,
			MessageType: &ServiceTargetMessage_EndpointsResponse{
				EndpointsResponse: &ServiceTargetEndpointsResponse{Endpoints: endpoints},
			},
		}
		if err != nil {
			resp.Error = &ServiceTargetErrorMessage{
				Message: err.Error(),
			}
		}
	default:
		log.Printf("Unknown or unhandled service target message type: %T", r)
	}
	return resp
}
