// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
)

type aiQuotaModelSelection struct {
	ModelName string
	Version   string
	Sku       *azdext.AiModelSku
	Locations []string
}

type aiQuotaVersionChoice struct {
	Version          string
	IsDefaultVersion bool
	Kind             string
	Format           string
	Status           string
	Capabilities     []string
	Skus             []*aiQuotaSkuChoice
}

type aiQuotaSkuChoice struct {
	Sku       *azdext.AiModelSku
	Locations []string
}

func promptQuotaModelSelection(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	models []*azdext.AiModelCatalogItem,
) (*aiQuotaModelSelection, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no AI model catalog entries are available for selection")
	}

	modelOptions := slices.Clone(models)
	slices.SortFunc(modelOptions, func(a *azdext.AiModelCatalogItem, b *azdext.AiModelCatalogItem) int {
		return strings.Compare(strings.ToLower(a.GetName()), strings.ToLower(b.GetName()))
	})

	selectedModel := modelOptions[0]
	if len(modelOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(modelOptions))
		for _, model := range modelOptions {
			choices = append(choices, &azdext.SelectChoice{
				Label: model.GetName(),
				Value: model.GetName(),
			})
		}

		modelIndex, err := promptSelectIndex(ctx, azdClient, "Select an AI model:", choices)
		if err != nil {
			return nil, err
		}

		selectedModel = modelOptions[modelIndex]
	}

	versionOptions := buildAiQuotaVersionChoices(selectedModel)
	if len(versionOptions) == 0 {
		return nil, fmt.Errorf("no model versions found for '%s'", selectedModel.GetName())
	}

	selectedVersion := versionOptions[0]
	if len(versionOptions) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(versionOptions))
		for _, version := range versionOptions {
			label := version.Version
			if version.IsDefaultVersion {
				label = fmt.Sprintf("%s (default)", label)
			}
			if version.Kind != "" || version.Format != "" {
				label = fmt.Sprintf("%s [%s/%s]", label, version.Kind, version.Format)
			}
			if version.Status != "" {
				label = fmt.Sprintf("%s (%s)", label, version.Status)
			}

			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: version.Version,
			})
		}

		versionIndex, err := promptSelectIndex(ctx, azdClient, "Select a model version:", choices)
		if err != nil {
			return nil, err
		}

		selectedVersion = versionOptions[versionIndex]
	}

	if len(selectedVersion.Skus) == 0 {
		return nil, fmt.Errorf(
			"no model SKUs found for '%s' version '%s'",
			selectedModel.GetName(),
			selectedVersion.Version,
		)
	}

	selectedSku := selectedVersion.Skus[0]
	if len(selectedVersion.Skus) > 1 {
		choices := make([]*azdext.SelectChoice, 0, len(selectedVersion.Skus))
		for _, sku := range selectedVersion.Skus {
			label := fmt.Sprintf(
				"%s (usage=%s, default capacity=%d, %d locations)",
				sku.Sku.GetName(),
				sku.Sku.GetUsageName(),
				sku.Sku.GetCapacityDefault(),
				len(sku.Locations),
			)

			choices = append(choices, &azdext.SelectChoice{
				Label: label,
				Value: sku.Sku.GetName(),
			})
		}

		skuIndex, err := promptSelectIndex(ctx, azdClient, "Select a model SKU:", choices)
		if err != nil {
			return nil, err
		}

		selectedSku = selectedVersion.Skus[skuIndex]
	}

	return &aiQuotaModelSelection{
		ModelName: selectedModel.GetName(),
		Version:   selectedVersion.Version,
		Sku:       selectedSku.Sku,
		Locations: selectedSku.Locations,
	}, nil
}

func buildAiQuotaVersionChoices(model *azdext.AiModelCatalogItem) []*aiQuotaVersionChoice {
	type skuAccumulator struct {
		name            string
		usageName       string
		capacityDefault int32
		capacityMinimum int32
		capacityMaximum int32
		capacityStep    int32
		locations       map[string]string
	}

	type versionAccumulator struct {
		version           string
		isDefaultVersion  bool
		kind              string
		format            string
		status            string
		capabilitiesByKey map[string]string
		skus              map[string]*skuAccumulator
	}

	versionByKey := map[string]*versionAccumulator{}
	for _, modelLocation := range model.GetLocations() {
		locationName := strings.TrimSpace(modelLocation.GetLocation())
		if locationName == "" {
			continue
		}

		for _, version := range modelLocation.GetVersions() {
			versionKey := strings.ToLower(strings.TrimSpace(version.GetVersion()))
			if versionKey == "" {
				continue
			}

			versionEntry, has := versionByKey[versionKey]
			if !has {
				versionEntry = &versionAccumulator{
					version:           version.GetVersion(),
					isDefaultVersion:  version.GetIsDefaultVersion(),
					kind:              version.GetKind(),
					format:            version.GetFormat(),
					status:            version.GetStatus(),
					capabilitiesByKey: map[string]string{},
					skus:              map[string]*skuAccumulator{},
				}
				versionByKey[versionKey] = versionEntry
			} else {
				versionEntry.isDefaultVersion = versionEntry.isDefaultVersion || version.GetIsDefaultVersion()

				// Keep first non-empty metadata to avoid blank labels after merge.
				if versionEntry.kind == "" {
					versionEntry.kind = version.GetKind()
				}
				if versionEntry.format == "" {
					versionEntry.format = version.GetFormat()
				}
				if versionEntry.status == "" {
					versionEntry.status = version.GetStatus()
				}
			}

			for _, capability := range version.GetCapabilities() {
				normalized := strings.ToLower(strings.TrimSpace(capability))
				if normalized == "" {
					continue
				}
				if _, exists := versionEntry.capabilitiesByKey[normalized]; !exists {
					versionEntry.capabilitiesByKey[normalized] = strings.TrimSpace(capability)
				}
			}

			for _, sku := range version.GetSkus() {
				skuName := strings.TrimSpace(sku.GetName())
				if skuName == "" {
					continue
				}

				usageName := strings.TrimSpace(sku.GetUsageName())
				skuKey := strings.Join([]string{
					strings.ToLower(skuName),
					strings.ToLower(usageName),
					strconv.Itoa(int(sku.GetCapacityDefault())),
					strconv.Itoa(int(sku.GetCapacityMinimum())),
					strconv.Itoa(int(sku.GetCapacityMaximum())),
					strconv.Itoa(int(sku.GetCapacityStep())),
				}, "|")

				skuEntry, skuHas := versionEntry.skus[skuKey]
				if !skuHas {
					skuEntry = &skuAccumulator{
						name:            sku.GetName(),
						usageName:       sku.GetUsageName(),
						capacityDefault: sku.GetCapacityDefault(),
						capacityMinimum: sku.GetCapacityMinimum(),
						capacityMaximum: sku.GetCapacityMaximum(),
						capacityStep:    sku.GetCapacityStep(),
						locations:       map[string]string{},
					}
					versionEntry.skus[skuKey] = skuEntry
				}

				addCaseInsensitiveString(skuEntry.locations, locationName)
			}
		}
	}

	versions := make([]*aiQuotaVersionChoice, 0, len(versionByKey))
	for _, versionEntry := range versionByKey {
		skus := make([]*aiQuotaSkuChoice, 0, len(versionEntry.skus))
		for _, skuEntry := range versionEntry.skus {
			skus = append(skus, &aiQuotaSkuChoice{
				Sku: &azdext.AiModelSku{
					Name:            skuEntry.name,
					UsageName:       skuEntry.usageName,
					CapacityDefault: skuEntry.capacityDefault,
					CapacityMinimum: skuEntry.capacityMinimum,
					CapacityMaximum: skuEntry.capacityMaximum,
					CapacityStep:    skuEntry.capacityStep,
				},
				Locations: mapValuesSorted(skuEntry.locations),
			})
		}

		slices.SortFunc(skus, func(a *aiQuotaSkuChoice, b *aiQuotaSkuChoice) int {
			nameCompare := strings.Compare(strings.ToLower(a.Sku.GetName()), strings.ToLower(b.Sku.GetName()))
			if nameCompare != 0 {
				return nameCompare
			}

			return strings.Compare(strings.ToLower(a.Sku.GetUsageName()), strings.ToLower(b.Sku.GetUsageName()))
		})

		capabilities := make([]string, 0, len(versionEntry.capabilitiesByKey))
		for _, capability := range versionEntry.capabilitiesByKey {
			capabilities = append(capabilities, capability)
		}
		slices.SortFunc(capabilities, func(a, b string) int {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		})

		versions = append(versions, &aiQuotaVersionChoice{
			Version:          versionEntry.version,
			IsDefaultVersion: versionEntry.isDefaultVersion,
			Kind:             versionEntry.kind,
			Format:           versionEntry.format,
			Status:           versionEntry.status,
			Capabilities:     capabilities,
			Skus:             skus,
		})
	}

	slices.SortFunc(versions, func(a *aiQuotaVersionChoice, b *aiQuotaVersionChoice) int {
		if a.IsDefaultVersion != b.IsDefaultVersion {
			if a.IsDefaultVersion {
				return -1
			}
			return 1
		}

		versionCompare := strings.Compare(strings.ToLower(a.Version), strings.ToLower(b.Version))
		if versionCompare != 0 {
			return versionCompare
		}

		kindCompare := strings.Compare(strings.ToLower(a.Kind), strings.ToLower(b.Kind))
		if kindCompare != 0 {
			return kindCompare
		}

		return strings.Compare(strings.ToLower(a.Format), strings.ToLower(b.Format))
	})

	return versions
}

func addCaseInsensitiveString(values map[string]string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}

	key := strings.ToLower(trimmed)
	if _, has := values[key]; !has {
		values[key] = trimmed
	}
}

func mapValuesSorted(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}

	slices.SortFunc(result, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})

	return result
}

func promptSelectIndex(
	ctx context.Context,
	azdClient *azdext.AzdClient,
	message string,
	choices []*azdext.SelectChoice,
) (int, error) {
	if len(choices) == 0 {
		return 0, fmt.Errorf("no choices available for selection")
	}

	enableFiltering := true
	resp, err := azdClient.Prompt().Select(ctx, &azdext.SelectRequest{
		Options: &azdext.SelectOptions{
			Message:         message,
			Choices:         choices,
			EnableFiltering: &enableFiltering,
			DisplayCount:    int32(min(12, len(choices))),
		},
	})
	if err != nil {
		return 0, err
	}

	index := int(resp.GetValue())
	if index < 0 || index >= len(choices) {
		return 0, fmt.Errorf("invalid selection index: %d", index)
	}

	return index, nil
}
