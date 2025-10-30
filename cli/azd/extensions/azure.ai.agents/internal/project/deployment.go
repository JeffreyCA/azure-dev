// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package project

// Deployment represents a single cognitive service account deployment
type Deployment struct {
	// Specify the name of cognitive service account deployment.
	Name string `json:"name"`

	// Required. Properties of Cognitive Services account deployment model.
	Model DeploymentModel `json:"model"`

	// The resource model definition representing SKU.
	Sku DeploymentSku `json:"sku"`
}

// DeploymentModel represents the model configuration for a cognitive services deployment
type DeploymentModel struct {
	// Required. The name of Cognitive Services account deployment model.
	Name string `json:"name"`

	// Required. The format of Cognitive Services account deployment model.
	Format string `json:"format"`

	// Required. The version of Cognitive Services account deployment model.
	Version string `json:"version"`
}

// DeploymentSku represents the resource model definition representing SKU
type DeploymentSku struct {
	// Required. The name of the resource model definition representing SKU.
	Name string `json:"name"`

	// The capacity of the resource model definition representing SKU.
	Capacity int `json:"capacity"`
}
