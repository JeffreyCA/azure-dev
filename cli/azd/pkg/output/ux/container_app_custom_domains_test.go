// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package ux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainerAppCustomDomains_ToString(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		ccd := &ContainerAppCustomDomains{
			ServiceDomains: map[string][]string{},
		}

		result := ccd.ToString("")
		assert.Equal(t, "", result)
	})

	t.Run("single service with single domain", func(t *testing.T) {
		ccd := &ContainerAppCustomDomains{
			ServiceDomains: map[string][]string{
				"my-app": {"example.com"},
			},
		}

		result := ccd.ToString("")
		assert.Contains(t, result, "my-app:")
		assert.Contains(t, result, "- example.com")
	})

	t.Run("single service with multiple domains", func(t *testing.T) {
		ccd := &ContainerAppCustomDomains{
			ServiceDomains: map[string][]string{
				"my-app": {"example.com", "sub.example.com"},
			},
		}

		result := ccd.ToString("")
		assert.Contains(t, result, "my-app:")
		assert.Contains(t, result, "- example.com")
		assert.Contains(t, result, "- sub.example.com")
	})

	t.Run("multiple services with domains", func(t *testing.T) {
		ccd := &ContainerAppCustomDomains{
			ServiceDomains: map[string][]string{
				"app1": {"example.com"},
				"app2": {"other.com", "api.other.com"},
			},
		}

		result := ccd.ToString("")
		assert.Contains(t, result, "app1:")
		assert.Contains(t, result, "app2:")
		assert.Contains(t, result, "- example.com")
		assert.Contains(t, result, "- other.com")
		assert.Contains(t, result, "- api.other.com")
	})
}
