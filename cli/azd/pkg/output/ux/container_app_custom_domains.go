// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package ux

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/output"
)

// ContainerAppCustomDomains defines a UX item for displaying Container App custom domain deletions
// grouped by Container App service name.
type ContainerAppCustomDomains struct {
	// Map of Container App service name to list of custom domains that will be deleted
	ServiceDomains map[string][]string
}

func (ccd *ContainerAppCustomDomains) ToString(currentIndentation string) string {
	if len(ccd.ServiceDomains) == 0 {
		return ""
	}

	var lines []string
	for serviceName, domains := range ccd.ServiceDomains {
		// Service name in bold red
		lines = append(lines, currentIndentation+fmt.Sprintf("%s (Container App):", serviceName))

		// Domain names with red bullet points and indentation
		for _, domain := range domains {
			lines = append(lines, currentIndentation+"  "+output.WithErrorFormat("- %s", domain))
		}

		// Add empty line between services
		lines = append(lines, "")
	}

	// Remove the trailing empty line
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func (ccd *ContainerAppCustomDomains) MarshalJSON() ([]byte, error) {
	// Flatten the data for JSON output
	var domains []map[string]interface{}
	for serviceName, domainList := range ccd.ServiceDomains {
		for _, domain := range domainList {
			domains = append(domains, map[string]interface{}{
				"serviceName": serviceName,
				"domain":      domain,
				"operation":   "Delete",
			})
		}
	}

	return json.Marshal(output.EventForMessage(fmt.Sprintf("Container App custom domains to be deleted: %v", domains)))
}
