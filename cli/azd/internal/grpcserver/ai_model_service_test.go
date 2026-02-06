// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"testing"

	"github.com/azure/azure-dev/cli/azd/internal/mapper"
	"github.com/azure/azure-dev/cli/azd/pkg/ai"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/require"
)

func TestMapperConvertFilter(t *testing.T) {
	// Ensure mapper registrations from pkg/ai/mapper_registry.go init() are loaded
	t.Run("NilFilter", func(t *testing.T) {
		var result *ai.FilterOptions
		err := mapper.Convert((*azdext.AiModelFilterOptions)(nil), &result)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("WithValues", func(t *testing.T) {
		proto := &azdext.AiModelFilterOptions{
			Capabilities: []string{"chat"},
			Kinds:        []string{"OpenAI"},
			Formats:      []string{"OpenAI"},
			Statuses:     []string{"GenerallyAvailable"},
			Locations:    []string{"eastus"},
		}
		var result *ai.FilterOptions
		err := mapper.Convert(proto, &result)
		require.NoError(t, err)
		require.Equal(t, []string{"chat"}, result.Capabilities)
		require.Equal(t, []string{"OpenAI"}, result.Kinds)
		require.Equal(t, []string{"OpenAI"}, result.Formats)
		require.Equal(t, []string{"GenerallyAvailable"}, result.Statuses)
		require.Equal(t, []string{"eastus"}, result.Locations)
	})
}

func TestMapperConvertModels(t *testing.T) {
	models := []*ai.Model{
		{
			Name: "gpt-4o",
			DetailsByLocation: map[string][]ai.ModelVersion{
				"eastus": {
					{
						Version:          "2024-05-13",
						Format:           "OpenAI",
						Kind:             "OpenAI",
						IsDefaultVersion: true,
						LifecycleStatus:  "GenerallyAvailable",
						Capabilities:     map[string]string{"chat": "true"},
						Skus: []ai.ModelSku{
							{
								Name:      "GlobalStandard",
								UsageName: "OpenAI.Standard.gpt-4o",
								Capacity: ai.ModelSkuCapacity{
									Default: 10,
									Maximum: 100,
									Minimum: 1,
									Step:    1,
								},
							},
						},
					},
				},
			},
		},
	}

	var result []*azdext.AiModel
	err := mapper.Convert(models, &result)
	require.NoError(t, err)

	require.Len(t, result, 1)
	require.Equal(t, "gpt-4o", result[0].Name)
	require.Contains(t, result[0].DetailsByLocation, "eastus")

	details := result[0].DetailsByLocation["eastus"]
	require.Len(t, details.Versions, 1)

	v := details.Versions[0]
	require.Equal(t, "2024-05-13", v.Version)
	require.Equal(t, "OpenAI", v.Format)
	require.Equal(t, "OpenAI", v.Kind)
	require.True(t, v.IsDefaultVersion)
	require.Equal(t, "GenerallyAvailable", v.LifecycleStatus)
	require.Equal(t, "true", v.Capabilities["chat"])

	require.Len(t, v.Skus, 1)
	require.Equal(t, "GlobalStandard", v.Skus[0].Name)
	require.Equal(t, "OpenAI.Standard.gpt-4o", v.Skus[0].UsageName)
	require.Equal(t, int32(10), v.Skus[0].Capacity.DefaultValue)
	require.Equal(t, int32(100), v.Skus[0].Capacity.Maximum)
}
