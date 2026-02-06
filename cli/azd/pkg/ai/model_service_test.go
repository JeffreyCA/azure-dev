// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package ai

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices"
	"github.com/stretchr/testify/require"
)

func TestConvertSDKModel(t *testing.T) {
	sdkModel := &armcognitiveservices.Model{
		Kind: to.Ptr("OpenAI"),
		Model: &armcognitiveservices.AccountModel{
			Name:             to.Ptr("gpt-4o"),
			Version:          to.Ptr("2024-05-13"),
			Format:           to.Ptr("OpenAI"),
			IsDefaultVersion: to.Ptr(true),
			LifecycleStatus:  to.Ptr(armcognitiveservices.ModelLifecycleStatusGenerallyAvailable),
			Capabilities: map[string]*string{
				"chat": to.Ptr("true"),
			},
			SKUs: []*armcognitiveservices.ModelSKU{
				{
					Name:      to.Ptr("GlobalStandard"),
					UsageName: to.Ptr("OpenAI.Standard.gpt-4o"),
					Capacity: &armcognitiveservices.CapacityConfig{
						Default: to.Ptr(int32(10)),
						Maximum: to.Ptr(int32(100)),
						Minimum: to.Ptr(int32(1)),
						Step:    to.Ptr(int32(1)),
					},
				},
			},
		},
	}

	mv := convertSDKModel(sdkModel)

	require.Equal(t, "2024-05-13", mv.Version)
	require.Equal(t, "OpenAI", mv.Format)
	require.Equal(t, "OpenAI", mv.Kind)
	require.True(t, mv.IsDefaultVersion)
	require.Equal(t, "GenerallyAvailable", mv.LifecycleStatus)
	require.Equal(t, "true", mv.Capabilities["chat"])
	require.Len(t, mv.Skus, 1)
	require.Equal(t, "GlobalStandard", mv.Skus[0].Name)
	require.Equal(t, "OpenAI.Standard.gpt-4o", mv.Skus[0].UsageName)
	require.Equal(t, int32(10), mv.Skus[0].Capacity.Default)
	require.Equal(t, int32(100), mv.Skus[0].Capacity.Maximum)
}

func TestFilterModels(t *testing.T) {
	modelMap := map[string]*Model{
		"gpt-4o": {
			Name: "gpt-4o",
			DetailsByLocation: map[string][]ModelVersion{
				"eastus": {
					{
						Version: "2024-05-13", Format: "OpenAI", Kind: "OpenAI",
						Capabilities: map[string]string{"chat": "true"},
					},
				},
			},
		},
		"text-embedding-ada-002": {
			Name: "text-embedding-ada-002",
			DetailsByLocation: map[string][]ModelVersion{
				"westus": {
					{
						Version: "2", Format: "OpenAI", Kind: "AIServices",
						Capabilities: map[string]string{"embeddings": "true"},
					},
				},
			},
		},
	}

	t.Run("NoFilter", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{})
		require.Len(t, result, 2)
	})

	t.Run("FilterByCapability", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{Capabilities: []string{"chat"}})
		require.Len(t, result, 1)
		require.Equal(t, "gpt-4o", result[0].Name)
	})

	t.Run("FilterByKind", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{Kinds: []string{"AIServices"}})
		require.Len(t, result, 1)
		require.Equal(t, "text-embedding-ada-002", result[0].Name)
	})

	t.Run("FilterByLocation", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{Locations: []string{"eastus"}})
		require.Len(t, result, 1)
		require.Equal(t, "gpt-4o", result[0].Name)
	})

	t.Run("FilterByMultipleCriteria", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{
			Kinds:        []string{"OpenAI"},
			Capabilities: []string{"chat"},
		})
		require.Len(t, result, 1)
		require.Equal(t, "gpt-4o", result[0].Name)
	})

	t.Run("FilterNoMatch", func(t *testing.T) {
		result := filterModels(modelMap, &FilterOptions{
			Kinds: []string{"OpenAI"},
			Capabilities: []string{"embeddings"},
		})
		require.Len(t, result, 0)
	})
}

func TestHasDefaultVersion(t *testing.T) {
	t.Run("WithDefault", func(t *testing.T) {
		model := &Model{
			DetailsByLocation: map[string][]ModelVersion{
				"eastus": {{IsDefaultVersion: true}},
			},
		}
		require.True(t, hasDefaultVersion(model))
	})

	t.Run("WithoutDefault", func(t *testing.T) {
		model := &Model{
			DetailsByLocation: map[string][]ModelVersion{
				"eastus": {{IsDefaultVersion: false}},
			},
		}
		require.False(t, hasDefaultVersion(model))
	})
}
