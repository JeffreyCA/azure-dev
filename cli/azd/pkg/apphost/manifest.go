// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package apphost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/azure/azure-dev/cli/azd/pkg/custommaps"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/dotnet"
	"github.com/psanford/memfs"
)

type Manifest struct {
	Schema    string               `json:"$schema"`
	Resources map[string]*Resource `json:"resources"`
	// BicepFiles holds any bicep files generated by Aspire next to the manifest file.
	BicepFiles *memfs.FS `json:"-"`
}

type Resource struct {
	// Type is present on all resource types
	Type string `json:"type"`

	// Path is present on a project.v0 resource and is the path to the project file, and on a dockerfile.v0
	// resource and is the path to the Dockerfile (including the "Dockerfile" filename).
	// For a bicep.v0 resource, it is the path to the bicep file.
	Path *string `json:"path,omitempty"`

	// Context is present on a dockerfile.v0 resource and is the path to the context directory.
	Context *string `json:"context,omitempty"`

	// BuildArgs is present on a dockerfile.v0 resource and is the --build-arg for building the docker image.
	BuildArgs map[string]string `json:"buildArgs,omitempty"`

	// Args is optionally present on project.v0 and dockerfile.v0 resources and are the arguments to pass to the container.
	Args []string `json:"args,omitempty"`

	// Parent is present on a resource which is a child of another. It is the name of the parent resource. For example, a
	// postgres.database.v0 is a child of a postgres.server.v0, and so it would have a parent of which is the name of
	// the server resource.
	Parent *string `json:"parent,omitempty"`

	// Image is present on a container.v0 resource and is the image to use for the container.
	Image *string `json:"image,omitempty"`

	// Bindings is present on container.v0, project.v0 and dockerfile.v0 resources, and is a map of binding names to
	// binding details.
	Bindings custommaps.WithOrder[Binding] `json:"bindings,omitempty"`

	// Env is present on project.v0, container.v0 and dockerfile.v0 resources, and is a map of environment variable
	// names to value  expressions. The value expressions are simple expressions like "{redis.connectionString}" or
	// "{postgres.port}" to allow referencing properties of other resources. The set of properties supported in these
	// expressions depends on the type of resource you are referencing.
	Env map[string]string `json:"env,omitempty"`

	// Queues is optionally present on a azure.servicebus.v0 resource, and is a list of queue names to create.
	Queues *[]string `json:"queues,omitempty"`

	// Topics is optionally present on a azure.servicebus.v0 resource, and is a list of topic names to create.
	Topics *[]string `json:"topics,omitempty"`

	// Some resources just represent connections to existing resources that need not be provisioned.  These resources have
	// a "connectionString" property which is the connection string that should be used during binding.
	ConnectionString *string `json:"connectionString,omitempty"`

	// Dapr is present on dapr.v0 resources.
	Dapr *DaprResourceMetadata `json:"dapr,omitempty"`

	// DaprComponent is present on dapr.component.v0 resources.
	DaprComponent *DaprComponentResourceMetadata `json:"daprComponent,omitempty"`

	// Inputs is present on resources that need inputs from during the provisioning process (e.g asking for an API key, or
	// a password for a database).
	Inputs map[string]Input `json:"inputs,omitempty"`

	// For a bicep.v0 resource, defines the input parameters for the bicep file.
	Params map[string]any `json:"params,omitempty"`

	// parameter.v0 uses value field to define the value of the parameter.
	Value string

	// container.v0 uses volumes field to define the volumes of the container.
	Volumes []*Volume `json:"volumes,omitempty"`

	// The entrypoint to use for the container image when executed.
	Entrypoint string `json:"entrypoint,omitempty"`

	// An object that captures properties that control the building of a container image.
	Build *ContainerV1Build `json:"build,omitempty"`

	// container.v0 uses bind mounts field to define the volumes with initial data of the container.
	BindMounts []*BindMount `json:"bindMounts,omitempty"`

	// project.v1 and container.v1 uses deployment when the AppHost owns the ACA bicep definitions.
	Deployment *DeploymentMetadata `json:"deployment,omitempty"`

	// Present on bicep modules to control the scope of the module.
	Scope *BicepModuleScope `json:"scope,omitempty"`
}

// BicepModuleScope is the scope of a bicep module.
type BicepModuleScope struct {
	ResourceGroup *string `json:"resourceGroup,omitempty"`
}

type DeploymentMetadata struct {
	// Type is the type of deployment. For now, only bicep.v0 is supported.
	Type string `json:"type"`

	// Path is present for a bicep.v0 deployment type, and the path to the bicep file.
	Path *string `json:"path,omitempty"`

	// For a bicep.v0 deployment type, defines the input parameters for the bicep file.
	Params map[string]any `json:"params,omitempty"`
}

type ContainerV1Build struct {
	// The path to the context directory for the container build.
	// Can be relative of absolute. If relative it is relative to the location of the manifest file.
	Context string `json:"context"`

	// The path to the Dockerfile. Can be relative or absolute. If relative it is relative to the manifest file.
	Dockerfile string `json:"dockerfile"`

	// Args is optionally present on project.v0 and dockerfile.v0 resources and are the arguments to pass to the container.
	Args map[string]string `json:"args,omitempty"`

	// A list of build arguments which are used during container build."
	Secrets map[string]ContainerV1BuildSecrets `json:"secrets,omitempty"`
}

type ContainerV1BuildSecrets struct {
	// "env" (will come with value) or "file" (will come with source).
	Type string `json:"type"`
	// If provided use as the value for the environment variable when docker build is run.
	Value *string `json:"value,omitempty"`
	// Path to secret file. If relative, the path is relative to the manifest file.
	Source *string `json:"source,omitempty"`
}

type DaprResourceMetadata struct {
	AppId                  *string `json:"appId,omitempty"`
	Application            *string `json:"application,omitempty"`
	AppPort                *int    `json:"appPort,omitempty"`
	AppProtocol            *string `json:"appProtocol,omitempty"`
	DaprHttpMaxRequestSize *int    `json:"daprHttpMaxRequestSize,omitempty"`
	DaprHttpReadBufferSize *int    `json:"daprHttpReadBufferSize,omitempty"`
	EnableApiLogging       *bool   `json:"enableApiLogging,omitempty"`
	LogLevel               *string `json:"logLevel,omitempty"`
}

type DaprComponentResourceMetadata struct {
	Type *string `json:"type"`
}

type Reference struct {
	Bindings []string `json:"bindings,omitempty"`
}

type Binding struct {
	TargetPort *int   `json:"targetPort,omitempty"`
	Port       *int   `json:"port,omitempty"`
	Scheme     string `json:"scheme"`
	Protocol   string `json:"protocol"`
	Transport  string `json:"transport"`
	External   bool   `json:"external"`
}

type Volume struct {
	Name     string `json:"name,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

type BindMount struct {
	Name     string `json:"-"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

type Input struct {
	Type    string        `json:"type"`
	Secret  bool          `json:"secret"`
	Default *InputDefault `json:"default,omitempty"`
	// When the input is used to set a bicep module scope, the scope is set here.
	// This allows generation to add azdMetadata to the bicep parameter.
	scope *string
}

type InputDefaultGenerate struct {
	MinLength  *uint `json:"minLength,omitempty"`
	Lower      *bool `json:"lower,omitempty"`
	Upper      *bool `json:"upper,omitempty"`
	Numeric    *bool `json:"numeric,omitempty"`
	Special    *bool `json:"special,omitempty"`
	MinLower   *uint `json:"minLower,omitempty"`
	MinUpper   *uint `json:"minUpper,omitempty"`
	MinNumeric *uint `json:"minNumeric,omitempty"`
	MinSpecial *uint `json:"minSpecial,omitempty"`
}

type InputDefault struct {
	Generate *InputDefaultGenerate `json:"generate,omitempty"`
	Value    *string               `json:"value,omitempty"`
}

// ManifestFromAppHost returns the Manifest from the given app host.
func ManifestFromAppHost(
	ctx context.Context, appHostProject string, dotnetCli *dotnet.Cli, dotnetEnv string,
) (*Manifest, error) {
	tempDir, err := os.MkdirTemp("", "azd-provision")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory for apphost-manifest.json: %w", err)
	}
	defer os.RemoveAll(tempDir)

	manifestPath := filepath.Join(tempDir, "apphost-manifest.json")

	if err := dotnetCli.PublishAppHostManifest(ctx, appHostProject, manifestPath, dotnetEnv); err != nil {
		return nil, fmt.Errorf("generating app host manifest: %w", err)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("unmarshalling manifest: %w", err)
	}

	// Make all paths absolute, to simplify logic for consumers.
	// Note that since we created a temp dir, and `dotnet run --publisher` returns relative paths to the temp dir,
	// the resulting path may be a symlinked path that isn't safe for Rel comparisons with the azd root directory.
	manifestDir := filepath.Dir(manifestPath)

	// The manifest writer writes paths relative to the manifest file. When we use a fixed manifest, the manifest is
	// located SxS with the appHostProject.
	if enabled, err := strconv.ParseBool(os.Getenv("AZD_DEBUG_DOTNET_APPHOST_USE_FIXED_MANIFEST")); err == nil && enabled {
		manifestDir = filepath.Dir(appHostProject)
	}

	manifest.BicepFiles = memfs.New()

	for resourceName, res := range manifest.Resources {
		if res.Path != nil {
			if res.Type == "azure.bicep.v0" || res.Type == "azure.bicep.v1" {
				e := manifest.BicepFiles.MkdirAll(resourceName, osutil.PermissionDirectory)
				if e != nil {
					return nil, e
				}
				// try reading as a generated bicep adding the tem-manifest dir
				content, e := os.ReadFile(filepath.Join(manifestDir, *res.Path))
				if e != nil {
					// second try reading as relative (external bicep reference)
					content, e = os.ReadFile(*res.Path)
					if e != nil {
						return nil, fmt.Errorf("did not find bicep at generated path or at: %s. Error: %w", *res.Path, e)
					}
				}
				*res.Path = filepath.Join(resourceName, filepath.Base(*res.Path))
				e = manifest.BicepFiles.WriteFile(*res.Path, content, osutil.PermissionFile)
				if e != nil {
					return nil, e
				}
				// move on to the next resource
				continue
			}

			if !filepath.IsAbs(*res.Path) {
				*res.Path = filepath.Join(manifestDir, *res.Path)
			}
		}

		if res.Deployment != nil {
			if res.Deployment.Type != "azure.bicep.v0" && res.Deployment.Type != "azure.bicep.v1" {
				return nil, fmt.Errorf(
					"unexpected deployment type %q. Supported types: [azure.bicep.v0, azure.bicep.v1]", res.Deployment.Type)
			}
			// use a folder with the name of the resource
			e := manifest.BicepFiles.MkdirAll(resourceName, osutil.PermissionDirectory)
			if e != nil {
				return nil, e
			}
			content, e := os.ReadFile(filepath.Join(manifestDir, *res.Deployment.Path))
			if e != nil {
				return nil, fmt.Errorf("reading bicep file from deployment property: %w", e)
			}
			*res.Deployment.Path = filepath.Join(resourceName, filepath.Base(*res.Deployment.Path))
			e = manifest.BicepFiles.WriteFile(*res.Deployment.Path, content, osutil.PermissionFile)
			if e != nil {
				return nil, e
			}
		}

		if res.Type == "dockerfile.v0" {
			if !filepath.IsAbs(*res.Context) {
				*res.Context = filepath.Join(manifestDir, *res.Context)
			}
		}
		if res.BindMounts != nil {
			for _, bindMount := range res.BindMounts {
				if !filepath.IsAbs(bindMount.Source) {
					bindMount.Source = filepath.Join(manifestDir, bindMount.Source)
				}
			}
		}
		if res.Type == "container.v1" {
			if res.Build != nil {
				if !filepath.IsAbs(res.Build.Dockerfile) {
					res.Build.Dockerfile = filepath.Join(manifestDir, res.Build.Dockerfile)
				}
				if !filepath.IsAbs(res.Build.Context) {
					res.Build.Context = filepath.Join(manifestDir, res.Build.Context)
				}
				for _, secret := range res.Build.Secrets {
					if secret.Source != nil && !filepath.IsAbs(*secret.Source) {
						*secret.Source = filepath.Join(manifestDir, *secret.Source)
					}
				}
			}
		}
	}

	return &manifest, nil
}
