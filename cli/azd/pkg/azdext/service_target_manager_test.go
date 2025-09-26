// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azdext

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingServiceTargetProvider struct {
	initErr       error
	serviceConfig *ServiceTargetConfig
}

func (p *recordingServiceTargetProvider) Initialize(ctx context.Context, serviceConfig *ServiceTargetConfig) error {
	p.serviceConfig = serviceConfig
	return p.initErr
}

func (p *recordingServiceTargetProvider) Endpoints(ctx context.Context, serviceConfig *ServiceTargetConfig, targetResource *TargetResource) ([]string, error) {
	return nil, nil
}

func (p *recordingServiceTargetProvider) GetTargetResource(ctx context.Context, subscriptionId string, serviceConfig *ServiceTargetConfig) (*TargetResource, error) {
	return nil, nil
}

func (p *recordingServiceTargetProvider) Package(ctx context.Context, serviceConfig *ServiceTargetConfig, frameworkPackage *ServicePackageResult, progress ProgressReporter) (*ServicePackageResult, error) {
	return nil, nil
}

func (p *recordingServiceTargetProvider) Publish(ctx context.Context, serviceConfig *ServiceTargetConfig, servicePackage *ServicePackageResult, targetResource *TargetResource, progress ProgressReporter) (*ServicePublishResult, error) {
	return nil, nil
}

func (p *recordingServiceTargetProvider) Deploy(ctx context.Context, serviceConfig *ServiceTargetConfig, servicePackage *ServicePackageResult, servicePublish *ServicePublishResult, targetResource *TargetResource, progress ProgressReporter) (*ServiceDeployResult, error) {
	return nil, nil
}

func TestBuildServiceTargetResponseMsg_Initialize(t *testing.T) {
	t.Parallel()

	testErr := errors.New("init failed")

	tests := []struct {
		name        string
		initErr     error
		expectError bool
	}{
		{
			name:        "success",
			initErr:     nil,
			expectError: false,
		},
		{
			name:        "error",
			initErr:     testErr,
			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			provider := &recordingServiceTargetProvider{initErr: tt.initErr}
			msg := &ServiceTargetMessage{
				RequestId: "req",
				MessageType: &ServiceTargetMessage_InitializeRequest{
					InitializeRequest: &ServiceTargetInitializeRequest{
						ServiceConfig: &ServiceTargetConfig{Name: "api", Host: "containerapp"},
					},
				},
			}

			resp := buildServiceTargetResponseMsg(context.Background(), provider, msg, nil)
			require.NotNil(t, resp)
			require.Equal(t, msg.RequestId, resp.RequestId)
			require.NotNil(t, resp.GetInitializeResponse())
			require.NotNil(t, provider.serviceConfig)
			require.Equal(t, "api", provider.serviceConfig.GetName())
			require.Equal(t, "containerapp", provider.serviceConfig.GetHost())

			if tt.expectError {
				require.NotNil(t, resp.Error)
				require.Equal(t, tt.initErr.Error(), resp.Error.Message)
			} else {
				require.Nil(t, resp.Error)
			}
		})
	}
}
