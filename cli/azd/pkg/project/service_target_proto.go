// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

import (
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
)

// ToProtoServiceConfig converts a project ServiceConfig into the protobuf representation used by extensions.
func ToProtoServiceConfig(serviceConfig *ServiceConfig) (*azdext.ServiceTargetConfig, error) {
	if serviceConfig == nil {
		return nil, fmt.Errorf("service config is required")
	}

	if serviceConfig.Project == nil {
		return nil, fmt.Errorf("service config '%s' is not associated with a project", serviceConfig.Name)
	}

	protoConfig := &azdext.ServiceTargetConfig{
		Name:        serviceConfig.Name,
		Host:        string(serviceConfig.Host),
		ProjectName: serviceConfig.Project.Name,
	}

	if !serviceConfig.ResourceGroupName.Empty() {
		templateValue, err := serviceConfig.ResourceGroupName.MarshalYAML()
		if err != nil {
			return nil, fmt.Errorf("marshalling service resource group name: %w", err)
		}
		template, ok := templateValue.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected resource group template type %T", templateValue)
		}

		protoConfig.ResourceGroupName = &azdext.ResourceGroupNameTemplate{
			Template: template,
			IsEmpty:  false,
		}
	} else {
		protoConfig.ResourceGroupName = &azdext.ResourceGroupNameTemplate{IsEmpty: true}
	}

	if !serviceConfig.Project.ResourceGroupName.Empty() {
		templateValue, err := serviceConfig.Project.ResourceGroupName.MarshalYAML()
		if err != nil {
			return nil, fmt.Errorf("marshalling project resource group name: %w", err)
		}
		template, ok := templateValue.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected project resource group template type %T", templateValue)
		}

		protoConfig.ProjectResourceGroupName = &azdext.ResourceGroupNameTemplate{
			Template: template,
			IsEmpty:  false,
		}
	} else {
		protoConfig.ProjectResourceGroupName = &azdext.ResourceGroupNameTemplate{IsEmpty: true}
	}

	return protoConfig, nil
}

// ToProtoServicePackageResult converts a project ServicePackageResult to protobuf.
func ToProtoServicePackageResult(result *ServicePackageResult) *azdext.ServicePackageResult {
	if result == nil {
		return nil
	}

	details := detailsInterfaceToStringMap(result.Details)
	protoResult := &azdext.ServicePackageResult{PackagePath: result.PackagePath}
	if len(details) > 0 {
		protoResult.Details = copyStringMap(details)
	}

	return protoResult
}

// FromProtoServicePackageResult converts a protobuf ServicePackageResult back to the project representation.
func FromProtoServicePackageResult(protoResult *azdext.ServicePackageResult, fallback *ServicePackageResult) *ServicePackageResult {
	if protoResult == nil {
		return fallback
	}

	result := &ServicePackageResult{}
	if fallback != nil {
		result.PackagePath = fallback.PackagePath
		result.Details = fallback.Details
	}

	if protoResult.PackagePath != "" {
		result.PackagePath = protoResult.PackagePath
	}

	if len(protoResult.Details) > 0 {
		result.Details = stringMapToDetailsInterface(protoResult.Details)
	}

	return result
}

// ToProtoServicePublishResult converts a project ServicePublishResult to protobuf.
func ToProtoServicePublishResult(result *ServicePublishResult) *azdext.ServicePublishResult {
	if result == nil {
		return nil
	}

	details := detailsInterfaceToStringMap(result.Details)
	if len(details) == 0 {
		return nil
	}

	return &azdext.ServicePublishResult{Details: copyStringMap(details)}
}

// FromProtoServicePublishResult converts a protobuf ServicePublishResult to the project representation.
func FromProtoServicePublishResult(protoResult *azdext.ServicePublishResult) *ServicePublishResult {
	if protoResult == nil {
		return nil
	}

	return &ServicePublishResult{
		Details: stringMapToDetailsInterface(protoResult.Details),
	}
}

// ToProtoTargetResource converts an environment.TargetResource to protobuf.
func ToProtoTargetResource(target *environment.TargetResource) *azdext.TargetResource {
	if target == nil {
		return nil
	}

	protoTarget := &azdext.TargetResource{
		SubscriptionId:    target.SubscriptionId(),
		ResourceGroupName: target.ResourceGroupName(),
		ResourceName:      target.ResourceName(),
		ResourceType:      target.ResourceType(),
	}

	if metadata := target.Metadata(); metadata != nil {
		protoTarget.Metadata = copyStringMap(metadata)
	}

	return protoTarget
}

// FromProtoTargetResource converts a protobuf TargetResource to the environment representation.
func FromProtoTargetResource(protoTarget *azdext.TargetResource) *environment.TargetResource {
	if protoTarget == nil {
		return nil
	}

	target := environment.NewTargetResource(
		protoTarget.SubscriptionId,
		protoTarget.ResourceGroupName,
		protoTarget.ResourceName,
		protoTarget.ResourceType,
	)
	target.SetMetadata(protoTarget.GetMetadata())

	return target
}
