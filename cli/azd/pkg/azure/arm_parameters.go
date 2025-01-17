// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package azure

import "encoding/json"

// ArmParameters is a map of arm template parameters to their configured values.
type ArmParameters map[string]ArmParameter

// ArmParametersFile is the model type for a `.parameters.json` file. It fits the schema outlined here:
// https://schema.management.azure.com/schemas/2019-04-01/deploymentParameters.json
type ArmParameterFile struct {
	Schema         string        `json:"$schema"`
	ContentVersion string        `json:"contentVersion"`
	Parameters     ArmParameters `json:"parameters"`
}


type ArmParameter interface{}

// type ArmParameter struct {
// 	Value     any           `json:"value,omitempty"`
// 	Reference *ArmParameterKeyvaultReference `json:"reference,omitempty"`
// }

// ArmParameterValue wraps the configured value for the parameter.
type ArmParameterValue struct {
	Value any `json:"value"`
}

// ArmParameterKeyvaultReference wraps the key vault reference for the parameter.
type ArmParameterKeyvaultReference struct {
	Reference KeyvaultReference `json:"reference"`
}

// KeyvaultReference represents the key vault reference structure.
type KeyvaultReference struct {
	KeyVault   KeyVault `json:"keyVault"`
	SecretName string   `json:"secretName"`
	SecretVersion string `json:"secretVersion,omitempty"`
}

// KeyVault represents the key vault id.
type KeyVault struct {
	ID string `json:"id"`
}

func (a *ArmParameters) UnmarshalJSON(data []byte) error {
    raw := map[string]json.RawMessage{}
    if err := json.Unmarshal(data, &raw); err != nil {
        return err
    }

    result := make(ArmParameters)
    for k, v := range raw {
        var probe map[string]interface{}
        if err := json.Unmarshal(v, &probe); err != nil {
            return err
        }

        if _, ok := probe["value"]; ok {
            var val ArmParameterValue
            if err := json.Unmarshal(v, &val); err != nil {
                return err
            }
            result[k] = val
        } else if _, ok := probe["reference"]; ok {
            var ref ArmParameterKeyvaultReference
            if err := json.Unmarshal(v, &ref); err != nil {
                return err
            }
            result[k] = ref
        }
    }
    *a = result
    return nil
}
