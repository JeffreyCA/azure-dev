// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package provisioning

import (
	"github.com/azure/azure-dev/cli/azd/pkg/containerapps"
)

// DeploymentPreview defines the general structure for a deployment preview regardless of the deployment provider.
type DeploymentPreview struct {
	Status     string
	Properties *DeploymentPreviewProperties
}

// DeploymentPreviewProperties holds the changes for the deployment preview.
type DeploymentPreviewProperties struct {
	Changes []*DeploymentPreviewChange
}

// DeploymentPreviewChange represents a change to one Azure resource.
type DeploymentPreviewChange struct {
	ChangeType        ChangeType
	ResourceId        Resource
	ResourceType      string
	Name              string
	UnsupportedReason string
	Before            interface{}
	After             interface{}
	Delta             []DeploymentPreviewPropertyChange
}

// DeploymentPreviewPropertyChange includes the details and properties from a resource change.
type DeploymentPreviewPropertyChange struct {
	ChangeType PropertyChangeType
	Path       string
	Before     interface{}
	After      interface{}
	Children   []DeploymentPreviewPropertyChange
}

// ChangeType defines a type for the valid changes for an Azure resource.
type ChangeType string

const (
	ChangeTypeCreate      ChangeType = "Create"
	ChangeTypeDelete      ChangeType = "Delete"
	ChangeTypeDeploy      ChangeType = "Deploy"
	ChangeTypeIgnore      ChangeType = "Ignore"
	ChangeTypeModify      ChangeType = "Modify"
	ChangeTypeNoChange    ChangeType = "NoChange"
	ChangeTypeUnsupported ChangeType = "Unsupported"
)

// PropertyChangeType defines a type for the valid properties of a change.
type PropertyChangeType string

const (
	PropertyChangeTypeArray    PropertyChangeType = "Array"
	PropertyChangeTypeCreate   PropertyChangeType = "Create"
	PropertyChangeTypeDelete   PropertyChangeType = "Delete"
	PropertyChangeTypeModify   PropertyChangeType = "Modify"
	PropertyChangeTypeNoEffect PropertyChangeType = "NoEffect"
)

// GetDeletedAcaCustomDomains extracts custom domain names from a WhatIf delta
func GetDeletedAcaCustomDomains(delta DeploymentPreviewPropertyChange) []string {
	if delta.Path != containerapps.PathConfigurationIngressCustomDomains {
		return nil
	}

	var domains []string

	switch delta.ChangeType {
	case PropertyChangeTypeDelete:
		// Case 1: Entire custom domains array is being deleted
		beforeArray, ok := delta.Before.([]any)
		if !ok {
			return nil
		}

		for _, item := range beforeArray {
			if domainObj, ok := item.(map[string]any); ok {
				if domainName, ok := domainObj["name"].(string); ok {
					domains = append(domains, domainName)
				}
			}
		}

	case PropertyChangeTypeArray:
		// Case 2: Array is being modified, check children for deletions
		for _, child := range delta.Children {
			if child.ChangeType == PropertyChangeTypeDelete {
				if beforeObj, ok := child.Before.(map[string]any); ok {
					if domainName, ok := beforeObj["name"].(string); ok {
						domains = append(domains, domainName)
					}
				}
			}
		}
	}

	return domains
}
