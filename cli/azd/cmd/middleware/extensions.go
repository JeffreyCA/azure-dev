// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package middleware

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal/grpcserver"
	"github.com/azure/azure-dev/cli/azd/pkg/extensions"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/fatih/color"
)

type ExtensionsMiddleware struct {
	extensionManager *extensions.Manager
	extensionRunner  *extensions.Runner
	serviceLocator   ioc.ServiceLocator
	console          input.Console
	options          *Options
}

func NewExtensionsMiddleware(
	options *Options,
	serviceLocator ioc.ServiceLocator,
	extensionsManager *extensions.Manager,
	extensionRunner *extensions.Runner,
	console input.Console,
) Middleware {
	return &ExtensionsMiddleware{
		options:          options,
		serviceLocator:   serviceLocator,
		extensionManager: extensionsManager,
		extensionRunner:  extensionRunner,
		console:          console,
	}
}

func (m *ExtensionsMiddleware) Run(ctx context.Context, next NextFn) (*actions.ActionResult, error) {
	// Extensions were already started in the root parent command
	if m.options.IsChildAction(ctx) {
		return next(ctx)
	}

	installedExtensions, err := m.extensionManager.ListInstalled()
	if err != nil {
		return nil, err
	}

	requireLifecycleEvents := false
	extensionList := []*extensions.Extension{}

	// Find extensions that require lifecycle events
	for _, extension := range installedExtensions {
		if slices.Contains(extension.Capabilities, extensions.LifecycleEventsCapability) {
			extensionList = append(extensionList, extension)
			requireLifecycleEvents = true
		}
	}

	if !requireLifecycleEvents {
		return next(ctx)
	}

	var grpcServer *grpcserver.Server
	if err := m.serviceLocator.Resolve(&grpcServer); err != nil {
		return nil, err
	}

	serverInfo, err := grpcServer.Start()
	if err != nil {
		return nil, err
	}

	extensionContexts := make([]context.CancelFunc, len(extensionList))

	// Track extension goroutines for proper cleanup
	var extensionWg sync.WaitGroup

	// Set up cleanup with proper goroutine synchronization
	defer func() {
		// 1. First signal extensions to shutdown gracefully
		for _, cancel := range extensionContexts {
			if cancel != nil {
				cancel()
			}
		}

		// 2. Wait for extension invoke goroutines to complete (with timeout)
		done := make(chan struct{})
		go func() {
			extensionWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Extensions terminated gracefully
		case <-time.After(1 * time.Second):
			// Extension shutdown timeout reached
		}

		// 3. Then stop the gRPC server
		if err := grpcServer.Stop(); err != nil {
			log.Printf("failed to stop gRPC server: %v\n", err)
		}
	}()

	forceColor := !color.NoColor

	var wg sync.WaitGroup

	for i, extension := range extensionList {
		jwtToken, err := grpcserver.GenerateExtensionToken(extension, serverInfo)
		if err != nil {
			return nil, err
		}

		// Create extension context before starting goroutine to avoid race
		extensionCtx, extensionCancel := context.WithCancel(ctx)
		extensionContexts[i] = extensionCancel

		wg.Add(1)
		go func(extension *extensions.Extension, jwtToken string, extensionCtx context.Context) {
			defer wg.Done()

			// Invoke the extension in a separate goroutine so that we can proceed to waiting for readiness.
			extensionWg.Add(1)
			go func() {
				defer extensionWg.Done()

				allEnv := []string{
					fmt.Sprintf("AZD_SERVER=%s", serverInfo.Address),
					fmt.Sprintf("AZD_ACCESS_TOKEN=%s", jwtToken),
				}

				if forceColor {
					allEnv = append(allEnv, "FORCE_COLOR=1")
				}

				options := &extensions.InvokeOptions{
					Args:   []string{"listen"},
					Env:    allEnv,
					StdIn:  extension.StdIn(),
					StdOut: extension.StdOut(),
					StdErr: extension.StdErr(),
				}

				if _, err := m.extensionRunner.Invoke(extensionCtx, extension, options); err != nil {
					extension.Fail(err)
				}
			}()

			// Wait for the extension to signal readiness or failure.
			readyCtx, cancel := context.WithTimeout(extensionCtx, 2*time.Second)
			defer cancel()
			if err := extension.WaitUntilReady(readyCtx); err != nil {
				log.Printf("extension '%s' failed to become ready: %v\n", extension.Id, err)
			}
		}(extension, jwtToken, extensionCtx)
	}

	// Wait for all extensions to reach a terminal state (ready or failed)
	wg.Wait()

	return next(ctx)
}
