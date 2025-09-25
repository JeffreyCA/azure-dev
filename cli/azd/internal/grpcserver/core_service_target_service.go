// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package grpcserver

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/pkg/async"
	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CoreServiceTargetService implements azdext.CoreServiceTargetServiceServer.
type CoreServiceTargetService struct {
	azdext.UnimplementedCoreServiceTargetServiceServer
	container *ioc.NestedContainer
}

// NewCoreServiceTargetService creates a new CoreServiceTargetService instance.
func NewCoreServiceTargetService(container *ioc.NestedContainer) azdext.CoreServiceTargetServiceServer {
	return &CoreServiceTargetService{container: container}
}

// Package delegates package operations to built-in azd service targets.
func (s *CoreServiceTargetService) Package(
	ctx context.Context,
	req *azdext.CoreServiceTargetPackageRequest,
) (*azdext.CoreServiceTargetPackageResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	packageReq := req.GetPackageRequest()
	if packageReq == nil {
		return nil, status.Error(codes.InvalidArgument, "package_request is required")
	}

	scope, err := s.container.NewScope()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating scope: %v", err)
	}

	serviceTarget, serviceConfig, err := s.resolveServiceTarget(scope, req.BuiltinKind, packageReq.ServiceConfig)
	if err != nil {
		return nil, err
	}

	frameworkPackage := project.FromProtoServicePackageResult(packageReq.FrameworkPackage, nil)
	if frameworkPackage == nil {
		frameworkPackage = &project.ServicePackageResult{}
	}

	progress := async.NewProgress[project.ServiceProgress]()
	progressDone := drainServiceProgress(progress)
	defer progressDone()

	packageResult, err := serviceTarget.Package(ctx, serviceConfig, frameworkPackage, progress)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "package: %v", err)
	}

	protoResult := project.ToProtoServicePackageResult(packageResult)
	return &azdext.CoreServiceTargetPackageResponse{PackageResult: protoResult}, nil
}

// Publish delegates publish operations to built-in azd service targets.
func (s *CoreServiceTargetService) Publish(
	ctx context.Context,
	req *azdext.CoreServiceTargetPublishRequest,
) (*azdext.CoreServiceTargetPublishResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	publishReq := req.GetPublishRequest()
	if publishReq == nil {
		return nil, status.Error(codes.InvalidArgument, "publish_request is required")
	}

	scope, err := s.container.NewScope()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating scope: %v", err)
	}

	serviceTarget, serviceConfig, err := s.resolveServiceTarget(scope, req.BuiltinKind, publishReq.ServiceConfig)
	if err != nil {
		return nil, err
	}

	servicePackage := project.FromProtoServicePackageResult(publishReq.ServicePackage, nil)
	if servicePackage == nil {
		servicePackage = &project.ServicePackageResult{}
	}

	targetResource := project.FromProtoTargetResource(publishReq.TargetResource)
	if targetResource == nil {
		return nil, status.Error(codes.InvalidArgument, "target_resource is required")
	}

	progress := async.NewProgress[project.ServiceProgress]()
	progressDone := drainServiceProgress(progress)
	defer progressDone()

	publishResult, err := serviceTarget.Publish(
		ctx,
		serviceConfig,
		servicePackage,
		targetResource,
		progress,
		nil, // publish options are not currently supplied by extensions
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "publish: %v", err)
	}

	protoResult := project.ToProtoServicePublishResult(publishResult)
	return &azdext.CoreServiceTargetPublishResponse{PublishResult: protoResult}, nil
}

func (s *CoreServiceTargetService) resolveServiceTarget(
	scope *ioc.NestedContainer,
	builtinKind string,
	serviceConfigProto *azdext.ServiceTargetConfig,
) (project.ServiceTarget, *project.ServiceConfig, error) {
	if serviceConfigProto == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "service_config is required")
	}

	var projectConfig *project.ProjectConfig
	if err := scope.Resolve(&projectConfig); err != nil {
		return nil, nil, status.Errorf(codes.Internal, "resolving project config: %v", err)
	}

	if projectConfig == nil {
		return nil, nil, status.Error(codes.FailedPrecondition, "project configuration is unavailable")
	}

	serviceConfig, has := projectConfig.Services[serviceConfigProto.Name]
	if !has {
		return nil, nil, status.Errorf(codes.NotFound, "service '%s' not found", serviceConfigProto.Name)
	}

	hostKind := string(serviceConfig.Host)
	targetKind := hostKind
	if builtinKind != "" {
		targetKind = builtinKind
	}
	if targetKind == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "service target kind could not be determined")
	}

	var target project.ServiceTarget
	if err := scope.ResolveNamed(targetKind, &target); err != nil {
		return nil, nil, status.Errorf(
			codes.Internal,
			"resolving service target '%s': %v",
			targetKind,
			err,
		)
	}

	return target, serviceConfig, nil
}

func drainServiceProgress(progress *async.Progress[project.ServiceProgress]) func() {
	done := make(chan struct{})
	go func() {
		for range progress.Progress() {
			// Discard progress updates for delegated calls.
		}
		close(done)
	}()

	return func() {
		progress.Done()
		<-done
	}
}
