// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/alpha"
	"github.com/azure/azure-dev/cli/azd/pkg/async"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
)

// ServiceIterationHelper handles the common service iteration and packaging logic
// for service operations like deploy and publish.
type ServiceIterationHelper struct {
	serviceManager project.ServiceManager
	importManager  *project.ImportManager
	console        input.Console
}

// ServiceIterationConfig contains configuration for service iteration operations
type ServiceIterationConfig struct {
	ProjectConfig     *project.ProjectConfig
	TargetServiceName string
	FromPackage       string
	AllServices       bool
	ServiceFilter     func(*project.ServiceConfig) bool
	OperationName     string // "Deploying" or "Publishing"
}

// ServiceOperationFunc is called for each service that should be processed.
// It receives the service config and package result, and returns the operation result.
type ServiceOperationFunc func(
	ctx context.Context,
	svc *project.ServiceConfig,
	packageResult *project.ServicePackageResult,
) (interface{}, error)

// NewServiceIterationHelper creates a new instance of ServiceIterationHelper
func NewServiceIterationHelper(
	serviceManager project.ServiceManager,
	importManager *project.ImportManager,
	console input.Console,
) *ServiceIterationHelper {
	return &ServiceIterationHelper{
		serviceManager: serviceManager,
		importManager:  importManager,
		console:        console,
	}
}

// IterateServices handles the common logic of iterating through services and calling an operation function for each.
// It returns a map of service names to their operation results.
func (h *ServiceIterationHelper) IterateServices(
	ctx context.Context,
	config ServiceIterationConfig,
	operationFunc ServiceOperationFunc,
) (map[string]interface{}, error) {
	results := map[string]interface{}{}

	// Get stable services
	stableServices, err := h.importManager.ServiceStable(ctx, config.ProjectConfig)
	if err != nil {
		return nil, err
	}

	for _, svc := range stableServices {
		stepMessage := fmt.Sprintf("%s service %s", config.OperationName, svc.Name)
		h.console.ShowSpinner(ctx, stepMessage, input.Step)

		// Skip this service if both cases are true:
		// 1. The user specified a service name
		// 2. This service is not the one the user specified
		if config.TargetServiceName != "" && config.TargetServiceName != svc.Name {
			h.console.StopSpinner(ctx, stepMessage, input.StepSkipped)
			continue
		}

		// Apply operation-specific service filtering
		if config.ServiceFilter != nil && !config.ServiceFilter(svc) {
			h.console.StopSpinner(ctx, stepMessage, input.StepSkipped)
			continue
		}

		// Check for alpha features
		if alphaFeatureId, isAlphaFeature := alpha.IsFeatureKey(string(svc.Host)); isAlphaFeature {
			h.console.WarnForFeature(ctx, alphaFeatureId)
		}

		var packageResult *project.ServicePackageResult
		if config.FromPackage != "" {
			// --from-package set, skip packaging
			packageResult = &project.ServicePackageResult{
				PackagePath: config.FromPackage,
			}
		} else {
			// --from-package not set, automatically package the application
			packageResult, err = async.RunWithProgress(
				func(packageProgress project.ServiceProgress) {
					progressMessage := fmt.Sprintf(
						"%s service %s (%s)",
						config.OperationName,
						svc.Name,
						packageProgress.Message,
					)
					h.console.ShowSpinner(ctx, progressMessage, input.Step)
				},
				func(progress *async.Progress[project.ServiceProgress]) (*project.ServicePackageResult, error) {
					return h.serviceManager.Package(ctx, svc, nil, progress, nil)
				},
			)

			if err != nil {
				h.console.StopSpinner(ctx, stepMessage, input.StepFailed)
				return nil, err
			}
		}

		// Execute the operation-specific function
		result, err := operationFunc(ctx, svc, packageResult)

		// Clean up temporary packages
		if config.FromPackage == "" && strings.HasPrefix(packageResult.PackagePath, os.TempDir()) {
			if err := os.RemoveAll(packageResult.PackagePath); err != nil {
				log.Printf("failed to remove temporary package: %s : %s", packageResult.PackagePath, err)
			}
		}

		h.console.StopSpinner(ctx, stepMessage, input.GetStepResultFormat(err))
		if err != nil {
			return nil, err
		}

		results[svc.Name] = result

		// Report operation outputs
		switch typedResult := result.(type) {
		case *project.ServiceDeployResult:
			h.console.MessageUxItem(ctx, typedResult)
		case *project.ServicePublishResult:
			h.console.MessageUxItem(ctx, typedResult)
		}
	}

	return results, nil
}
