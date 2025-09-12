// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/google/uuid"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// ServiceTargetProvider defines the interface for service target logic.
type ServiceTargetProvider interface {
	Name(ctx context.Context) (string, error)
	Initialize(ctx context.Context, projectPath string, options *ServiceTargetOptions) error
	State(ctx context.Context, options *ServiceTargetStateOptions) (*ServiceTargetStateResult, error)
	GetTargetResource(ctx context.Context, subscriptionId string, serviceConfig *ServiceTargetConfig) (*TargetResource, error)
	Deploy(ctx context.Context, serviceConfig *ServiceTargetConfig, servicePackage *ServiceTargetPackageResult, targetResource *TargetResource) (*ServiceTargetDeployResult, error)
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
		go m.handleServiceTargetStream(ctx, provider)
		return nil
	}

	return status.Errorf(codes.FailedPrecondition, "expected RegisterProviderResponse, got %T", msg.GetMessageType())
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
				resp := buildServiceTargetResponseMsg(ctx, provider, msg)
				if resp != nil {
					if err := m.stream.Send(resp); err != nil {
						log.Printf("failed to send service target response: %v", err)
					}
				}
			}(msg)
		}
	}
}

func buildServiceTargetResponseMsg(ctx context.Context, provider ServiceTargetProvider, msg *ServiceTargetMessage) *ServiceTargetMessage {
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
	case *ServiceTargetMessage_DeployRequest:
		result, err := provider.Deploy(ctx, r.DeployRequest.ServiceConfig, r.DeployRequest.ServicePackage, r.DeployRequest.TargetResource)
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
	default:
		log.Printf("Unknown or unhandled service target message type: %T", r)
	}
	return resp
}
