// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package ai

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/internal/mapper"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

func init() {
	registerAiModelMappings()
}

// registerAiModelMappings registers all AI model type conversions with the mapper.
func registerAiModelMappings() {
	// *ai.Model -> *azdext.AiModel
	mapper.MustRegister(func(ctx context.Context, src *Model) (*azdext.AiModel, error) {
		detailsMap := make(map[string]*azdext.AiModelLocationDetails, len(src.DetailsByLocation))
		for location, versions := range src.DetailsByLocation {
			protoVersions := make([]*azdext.AiModelVersion, len(versions))
			for i, v := range versions {
				var protoVersion *azdext.AiModelVersion
				if err := mapper.Convert(&v, &protoVersion); err != nil {
					return nil, err
				}
				protoVersions[i] = protoVersion
			}
			detailsMap[location] = &azdext.AiModelLocationDetails{
				Versions: protoVersions,
			}
		}
		return &azdext.AiModel{
			Name:              src.Name,
			DetailsByLocation: detailsMap,
		}, nil
	})

	// *ai.ModelVersion -> *azdext.AiModelVersion
	mapper.MustRegister(func(ctx context.Context, src *ModelVersion) (*azdext.AiModelVersion, error) {
		protoSkus := make([]*azdext.AiModelSku, len(src.Skus))
		for i, sku := range src.Skus {
			protoSkus[i] = &azdext.AiModelSku{
				Name:      sku.Name,
				UsageName: sku.UsageName,
				Capacity: &azdext.AiModelSkuCapacity{
					DefaultValue: sku.Capacity.Default,
					Maximum:      sku.Capacity.Maximum,
					Minimum:      sku.Capacity.Minimum,
					Step:         sku.Capacity.Step,
				},
			}
		}
		return &azdext.AiModelVersion{
			Version:          src.Version,
			Format:           src.Format,
			Kind:             src.Kind,
			IsDefaultVersion: src.IsDefaultVersion,
			LifecycleStatus:  src.LifecycleStatus,
			Capabilities:     src.Capabilities,
			Skus:             protoSkus,
		}, nil
	})

	// []*ai.Model -> []*azdext.AiModel (slice conversion)
	mapper.MustRegister(func(ctx context.Context, src []*Model) ([]*azdext.AiModel, error) {
		result := make([]*azdext.AiModel, len(src))
		for i, m := range src {
			var proto *azdext.AiModel
			if err := mapper.Convert(m, &proto); err != nil {
				return nil, err
			}
			result[i] = proto
		}
		return result, nil
	})

	// *azdext.AiModelFilterOptions -> *ai.FilterOptions
	mapper.MustRegister(func(ctx context.Context, src *azdext.AiModelFilterOptions) (*FilterOptions, error) {
		if src == nil {
			return nil, nil
		}
		return &FilterOptions{
			Capabilities: src.Capabilities,
			Statuses:     src.Statuses,
			Formats:      src.Formats,
			Kinds:        src.Kinds,
			Locations:    src.Locations,
		}, nil
	})

	// *ai.Usage -> *azdext.AiUsage
	mapper.MustRegister(func(ctx context.Context, src *Usage) (*azdext.AiUsage, error) {
		return &azdext.AiUsage{
			Name:         src.Name,
			CurrentValue: src.CurrentValue,
			Limit:        src.Limit,
		}, nil
	})

	// []ai.Usage -> []*azdext.AiUsage (slice conversion)
	mapper.MustRegister(func(ctx context.Context, src []Usage) ([]*azdext.AiUsage, error) {
		result := make([]*azdext.AiUsage, len(src))
		for i, u := range src {
			var proto *azdext.AiUsage
			if err := mapper.Convert(&u, &proto); err != nil {
				return nil, err
			}
			result[i] = proto
		}
		return result, nil
	})

	// *ai.ModelDeployment -> *azdext.GetModelDeploymentResponse
	mapper.MustRegister(func(ctx context.Context, src *ModelDeployment) (*azdext.GetModelDeploymentResponse, error) {
		return &azdext.GetModelDeploymentResponse{
			Name:     src.Name,
			Format:   src.Format,
			Version:  src.Version,
			Location: src.Location,
			Sku: &azdext.AiModelDeploymentSku{
				Name:      src.Sku.Name,
				UsageName: src.Sku.UsageName,
				Capacity:  src.Sku.Capacity,
			},
		}, nil
	})

	// []*azdext.QuotaRequirement -> []ai.QuotaRequirement
	mapper.MustRegister(func(ctx context.Context, src []*azdext.QuotaRequirement) ([]QuotaRequirement, error) {
		result := make([]QuotaRequirement, len(src))
		for i, r := range src {
			result[i] = QuotaRequirement{
				UsageName: r.UsageName,
				Capacity:  r.Capacity,
			}
		}
		return result, nil
	})
}
