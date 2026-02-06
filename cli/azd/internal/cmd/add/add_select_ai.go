// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package add

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/azureutil"
	"github.com/azure/azure-dev/cli/azd/pkg/infra/provisioning"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/output/ux"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
)

func (a *AddAction) selectSearch(
	console input.Console,
	ctx context.Context,
	_ PromptOptions) (*project.ResourceConfig, error) {
	r := &project.ResourceConfig{}
	r.Type = project.ResourceTypeAiSearch
	return r, nil
}

func (a *AddAction) selectOpenAi(
	console input.Console,
	ctx context.Context,
	_ PromptOptions) (*project.ResourceConfig, error) {
	r := &project.ResourceConfig{}
	r.Type = project.ResourceTypeOpenAiModel
	return r, nil
}

func (a *AddAction) promptOpenAi(
	console input.Console,
	ctx context.Context,
	r *project.ResourceConfig,
	_ PromptOptions) (*project.ResourceConfig, error) {
	aiOption, err := console.Select(ctx, input.ConsoleOptions{
		Message: "Which type of Azure OpenAI service?",
		Options: []string{
			"Chat (GPT)",                   // 0 - chat
			"Embeddings (Document search)", // 1 - embeddings
		}})
	if err != nil {
		return nil, err
	}

	var allModels []ModelList
	for {
		err = provisioning.EnsureSubscriptionAndLocation(
			ctx, a.envManager, a.env, a.prompter, provisioning.EnsureSubscriptionAndLocationOptions{})
		if err != nil {
			return nil, err
		}

		console.ShowSpinner(
			ctx,
			fmt.Sprintf("Fetching available models in %s...", a.env.GetLocation()),
			input.Step)

		supportedModels, err := a.supportedModelsInLocation(ctx, a.env.GetSubscriptionId(), a.env.GetLocation())
		if err != nil {
			return nil, err
		}
		console.StopSpinner(ctx, "", input.Step)

		for _, model := range supportedModels {
			if model.Kind == "OpenAI" && slices.ContainsFunc(model.Model.Skus, func(sku ModelSku) bool {
				return sku.Name == "Standard"
			}) {
				switch aiOption {
				case 0:
					if model.Model.Name == "gpt-4o" || model.Model.Name == "gpt-4" {
						allModels = append(allModels, model)
					}
				case 1:
					if strings.HasPrefix(model.Model.Name, "text-embedding") {
						allModels = append(allModels, model)
					}
				}
			}

		}
		if len(allModels) > 0 {
			break
		}

		_, err = a.rm.FindResourceGroupForEnvironment(
			ctx, a.env.GetSubscriptionId(), a.env.Name())
		var notFoundError *azureutil.ResourceNotFoundError
		if errors.As(err, &notFoundError) { // not yet provisioned, we're safe here
			console.MessageUxItem(ctx, &ux.WarningMessage{
				Description: fmt.Sprintf("No models found in %s", a.env.GetLocation()),
			})
			confirm, err := console.Confirm(ctx, input.ConsoleOptions{
				Message: "Try a different location?",
			})
			if err != nil {
				return nil, err
			}
			if confirm {
				a.env.SetLocation("")
				continue
			}
		} else if err != nil {
			return nil, fmt.Errorf("finding resource group: %w", err)
		}

		return nil, fmt.Errorf("no models found in %s", a.env.GetLocation())
	}

	slices.SortFunc(allModels, func(a ModelList, b ModelList) int {
		if cmp := strings.Compare(strings.ToLower(a.Model.Name), strings.ToLower(b.Model.Name)); cmp != 0 {
			return cmp
		}

		return strings.Compare(strings.ToLower(b.Model.Version), strings.ToLower(a.Model.Version))
	})

	displayModels := make([]string, 0, len(allModels))
	models := make([]Model, 0, len(allModels))
	for _, model := range allModels {
		models = append(models, model.Model)
		displayModels = append(displayModels, fmt.Sprintf("%s\t%s", model.Model.Name, model.Model.Version))
	}

	if console.IsSpinnerInteractive() {
		displayModels, err = output.TabAlign(displayModels, 5)
		if err != nil {
			return nil, fmt.Errorf("writing models: %w", err)
		}
	}

	sel, err := console.Select(ctx, input.ConsoleOptions{
		Message: "Select the model",
		Options: displayModels,
	})
	if err != nil {
		return nil, err
	}

	r.Props = project.AIModelProps{
		Model: project.AIModelPropsModel{
			Name:    models[sel].Name,
			Version: models[sel].Version,
		},
	}

	return r, nil
}

func (a *AddAction) supportedModelsInLocation(ctx context.Context, subId, location string) ([]ModelList, error) {
	catalog, err := a.azureClient.ListAiModelCatalog(ctx, subId, azapi.AiModelCatalogFilters{
		Locations: []string{location},
	})
	if err != nil {
		return nil, fmt.Errorf("getting models: %w", err)
	}

	modelList := []ModelList{}
	for _, model := range catalog {
		for _, modelLocation := range model.Locations {
			if !strings.EqualFold(modelLocation.Location, location) {
				continue
			}

			for _, version := range modelLocation.Versions {
				skus := make([]ModelSku, 0, len(version.Skus))
				for _, sku := range version.Skus {
					skus = append(skus, ModelSku{
						Name:      sku.Name,
						UsageName: sku.UsageName,
						Capacity: ModelSkuCapacity{
							Maximum: sku.CapacityMaximum,
							Minimum: sku.CapacityMinimum,
							Step:    sku.CapacityStep,
							Default: sku.CapacityDefault,
						},
					})
				}

				modelList = append(modelList, ModelList{
					Kind: version.Kind,
					Model: Model{
						Name:             model.Name,
						Skus:             skus,
						Version:          version.Version,
						Format:           version.Format,
						IsDefaultVersion: version.IsDefaultVersion,
					},
				})
			}
		}
	}

	return modelList, nil
}

type ModelList struct {
	Kind  string `json:"kind"`
	Model Model  `json:"model"`
}

type Model struct {
	Name             string     `json:"name"`
	Skus             []ModelSku `json:"skus"`
	Version          string     `json:"version"`
	Format           string     `json:"format"`
	IsDefaultVersion bool       `json:"isDefaultVersion"`
}

type ModelSku struct {
	Name      string           `json:"name"`
	UsageName string           `json:"usageName"`
	Capacity  ModelSkuCapacity `json:"capacity"`
}

type ModelSkuCapacity struct {
	Maximum int32 `json:"maximum"`
	Minimum int32 `json:"minimum"`
	Step    int32 `json:"step"`
	Default int32 `json:"default"`
}

func (a *AddAction) selectAiModel(
	console input.Console,
	ctx context.Context,
	p PromptOptions) (*project.ResourceConfig, error) {
	r := &project.ResourceConfig{}
	r.Type = project.ResourceTypeAiProject
	return r, nil
}

func (a *AddAction) promptAiModel(
	console input.Console,
	ctx context.Context,
	r *project.ResourceConfig,
	p PromptOptions) (*project.ResourceConfig, error) {
	// check if there are models in the project already
	aiProject := project.AiFoundryModelProps{}
	for _, resource := range p.PrjConfig.Resources {
		if resource.Type == project.ResourceTypeAiProject && resource.Name == "ai-project" {
			em, castOk := resource.Props.(project.AiFoundryModelProps)
			if !castOk {
				return nil, fmt.Errorf("invalid resource properties")
			}
			r.Name = resource.Name
			aiProject = em
			r.Props = aiProject
			break
		}
	}

	modelCatalog, err := a.aiDeploymentCatalog(ctx, a.env.GetSubscriptionId(), aiProject.Models)
	if err != nil {
		return nil, err
	}

	modelNameSelection, m, err := selectFromMap(ctx, console, "Which model do you want to use?", modelCatalog, nil)
	if err != nil {
		return nil, err
	}
	_, k, err := selectFromMap(ctx, console, "Which deployment kind do you want to use?", m.Kinds, nil)
	if err != nil {
		return nil, err
	}

	modelVersionSelection, modelDefinition, err := selectFromMap(
		ctx, console, "Which model version do you want to use?", k.Versions, nil /*defVersion*/)
	if err != nil {
		return nil, err
	}
	skuSelection, err := selectFromSkus(ctx, console, "Select model SKU", modelDefinition.Model.Skus)
	if err != nil {
		return nil, err
	}

	aiProject.Models = append(aiProject.Models, project.AiServicesModel{
		Name:    modelNameSelection,
		Version: modelVersionSelection,
		Format:  modelDefinition.Model.Format,
		Sku: project.AiServicesModelSku{
			Name:      skuSelection.Name,
			UsageName: skuSelection.UsageName,
			Capacity:  skuSelection.Capacity.Default,
		},
	})
	r.Props = aiProject
	return r, nil
}

func selectFromMap[T any](
	ctx context.Context, console input.Console, q string, m map[string]T, defaultOpt *string) (string, T, error) {
	mIterator := maps.Keys(m)
	var options []string
	var value T
	for option := range mIterator {
		options = append(options, option)
	}
	if len(options) == 1 {
		key := options[0]
		return key, m[key], nil
	}
	defOpt := options[0]
	if defaultOpt != nil {
		defOpt = *defaultOpt
	}
	slices.Sort(options)
	selectedIndex, err := console.Select(ctx, input.ConsoleOptions{
		Message:      q,
		Options:      options,
		DefaultValue: defOpt,
	})
	if err != nil {
		return "", value, err
	}
	key := options[selectedIndex]
	return key, m[key], nil
}

func selectFromSkus(ctx context.Context, console input.Console, q string, s []ModelSku) (ModelSku, error) {
	var sku ModelSku
	if len(s) == 0 {
		return sku, fmt.Errorf("no skus found")
	}
	if len(s) == 1 {
		return s[0], nil
	}
	var options []string
	for _, option := range s {
		options = append(options, option.Name)
	}
	selectedIndex, err := console.Select(ctx, input.ConsoleOptions{
		Message:      q,
		Options:      options,
		DefaultValue: options[0],
	})
	if err != nil {
		return sku, err
	}
	return s[selectedIndex], nil
}

func (a *AddAction) aiDeploymentCatalog(
	ctx context.Context, subId string, excludeModels []project.AiServicesModel) (map[string]ModelCatalogKind, error) {
	a.console.ShowSpinner(ctx, "Retrieving available models...", input.Step)
	catalog, err := a.azureClient.ListAiModelCatalog(ctx, subId, azapi.AiModelCatalogFilters{})
	a.console.StopSpinner(ctx, "", input.StepDone)
	if err != nil {
		return nil, fmt.Errorf("getting model catalog: %w", err)
	}

	combinedResults := map[string]ModelCatalogKind{}
	for _, model := range catalog {
		for _, modelLocation := range model.Locations {
			for _, version := range modelLocation.Versions {
				if version.Kind == "OpenAI" {
					// OpenAI kind is handled by `add openai`, where clients connect directly without an AI Project.
					continue
				}

				skus := make([]ModelSku, 0, len(version.Skus))
				for _, sku := range version.Skus {
					if sku.CapacityDefault <= 0 {
						continue
					}

					skus = append(skus, ModelSku{
						Name:      sku.Name,
						UsageName: sku.UsageName,
						Capacity: ModelSkuCapacity{
							Maximum: sku.CapacityMaximum,
							Minimum: sku.CapacityMinimum,
							Step:    sku.CapacityStep,
							Default: sku.CapacityDefault,
						},
					})
				}
				if len(skus) == 0 {
					continue
				}

				modelList := ModelList{
					Kind: version.Kind,
					Model: Model{
						Name:             model.Name,
						Skus:             skus,
						Version:          version.Version,
						Format:           version.Format,
						IsDefaultVersion: version.IsDefaultVersion,
					},
				}

				if slices.ContainsFunc(excludeModels, func(m project.AiServicesModel) bool {
					return modelList.Model.Name == m.Name &&
						modelList.Model.Format == m.Format &&
						modelList.Model.Version == m.Version &&
						slices.ContainsFunc(modelList.Model.Skus, func(sku ModelSku) bool { return sku.Name == m.Sku.Name })
				}) {
					continue
				}

				upsertModelCatalogEntry(combinedResults, modelList, modelLocation.Location)
			}
		}
	}

	return combinedResults, nil
}

func upsertModelCatalogEntry(
	catalog map[string]ModelCatalogKind,
	model ModelList,
	location string,
) {
	nameKey := model.Model.Name
	kindKey := model.Kind
	versionKey := model.Model.Version

	modelCatalogKind, exists := catalog[nameKey]
	if !exists {
		catalog[nameKey] = ModelCatalogKind{
			Kinds: map[string]ModelCatalogVersions{
				kindKey: {
					Versions: map[string]ModelCatalog{
						versionKey: {
							ModelList: model,
							Locations: []string{location},
						},
					},
				},
			},
		}
		return
	}

	modelCatalogVersions, kindExists := modelCatalogKind.Kinds[kindKey]
	if !kindExists {
		modelCatalogKind.Kinds[kindKey] = ModelCatalogVersions{
			Versions: map[string]ModelCatalog{
				versionKey: {
					ModelList: model,
					Locations: []string{location},
				},
			},
		}
		catalog[nameKey] = modelCatalogKind
		return
	}

	modelCatalogEntry, versionExists := modelCatalogVersions.Versions[versionKey]
	if !versionExists {
		modelCatalogVersions.Versions[versionKey] = ModelCatalog{
			ModelList: model,
			Locations: []string{location},
		}
		modelCatalogKind.Kinds[kindKey] = modelCatalogVersions
		catalog[nameKey] = modelCatalogKind
		return
	}

	if !slices.Contains(modelCatalogEntry.Locations, location) {
		modelCatalogEntry.Locations = append(modelCatalogEntry.Locations, location)
	}
	modelCatalogVersions.Versions[versionKey] = modelCatalogEntry
	modelCatalogKind.Kinds[kindKey] = modelCatalogVersions
	catalog[nameKey] = modelCatalogKind
}

type ModelCatalog struct {
	ModelList
	Locations []string
}

type ModelCatalogKind struct {
	Kinds map[string]ModelCatalogVersions
}

type ModelCatalogVersions struct {
	Versions map[string]ModelCatalog
}
