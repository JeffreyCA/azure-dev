// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/stretchr/testify/require"
)

func TestToProtoServiceConfig(t *testing.T) {
	t.Parallel()

	projectConfig := &ProjectConfig{
		Name:              "contoso",
		ResourceGroupName: osutil.NewExpandableString("rg-${AZURE_ENV_NAME}"),
	}

	serviceConfig := &ServiceConfig{
		Name:              "api",
		Host:              ContainerAppTarget,
		Project:           projectConfig,
		ResourceGroupName: osutil.NewExpandableString("rg-${SERVICE_NAME}"),
	}

	protoConfig, err := (&ExternalServiceTarget{}).toProtoServiceConfig(serviceConfig)
	require.NoError(t, err)
	require.NotNil(t, protoConfig)
	require.Equal(t, "api", protoConfig.Name)
	require.Equal(t, string(ContainerAppTarget), protoConfig.Host)
	require.Equal(t, "contoso", protoConfig.ProjectName)
	require.NotNil(t, protoConfig.ResourceGroupName)
	require.Equal(t, "rg-${SERVICE_NAME}", protoConfig.ResourceGroupName.Template)
	require.False(t, protoConfig.ResourceGroupName.IsEmpty)
	require.NotNil(t, protoConfig.ProjectResourceGroupName)
	require.Equal(t, "rg-${AZURE_ENV_NAME}", protoConfig.ProjectResourceGroupName.Template)
	require.False(t, protoConfig.ProjectResourceGroupName.IsEmpty)
}

func TestToProtoServiceConfig_EmptyResourceGroups(t *testing.T) {
	t.Parallel()

	serviceConfig := &ServiceConfig{
		Name: "api",
		Host: ContainerAppTarget,
		Project: &ProjectConfig{
			Name: "contoso",
		},
	}

	protoConfig, err := (&ExternalServiceTarget{}).toProtoServiceConfig(serviceConfig)
	require.NoError(t, err)
	require.True(t, protoConfig.ResourceGroupName.IsEmpty)
	require.True(t, protoConfig.ProjectResourceGroupName.IsEmpty)
}
