# GitHub Copilot Instructions

This is the Azure Developer CLI - a Go-based CLI tool for managing Azure application development workflows. It handles infrastructure provisioning, application deployment, environment management, and project lifecycle automation. Please follow these guidelines when contributing.

## Code Standards

### Required Before Each Commit
- Run `gofmt -s -w .` before committing any changes to ensure proper code formatting
- Run `golangci-lint run ./...` to check for linting issues
- Run `cspell lint "**/*.go" --relative --config ./.vscode/cspell.yaml` to check spelling
- New Go files must include the standard copyright header:
  ```go
  // Copyright (c) Microsoft Corporation. All rights reserved.
  // Licensed under the MIT License.
  ```

**Note: All formatting, linting, and spelling commands should be run from the `cli/azd` directory.**

### Development Flow
**Prerequisites:** [Go](https://go.dev/dl/) 1.24

**Build `azd` binary:**
```bash
cd cli/azd
go build
```

**Test:**
- Quick tests: `go test ./... -short`
- All tests: `go test ./...`

## Repository Structure
- `cli/azd/`: Main CLI application entry point and command definitions
- `cli/azd/cmd/`: CLI command implementations using Cobra framework
- `cli/azd/internal/`: Internal CLI packages - application detection, scaffolding, telemetry, tracing, error handling, and CLI-specific utilities not intended for external use
- `cli/azd/pkg/`: Reusable business logic - Azure service integrations, environment/project management, infrastructure provisioning, deployment targets, and cross-cutting platform concerns
- `cli/azd/test/`: Test helpers and mocks
- `templates/`: Project template definitions and scaffolding resources
- `schemas/`: JSON schemas for azure.yaml validation
- `ext/`: Extensions for VS Code, Azure DevOps, and Dev Containers
- `eng/`: Engineering scripts, CI/CD pipelines, and build automation

## Key Guidelines
1. Follow Go best practices and idiomatic patterns
1. Maintain existing code structure and organization, unless refactoring is necessary or directly requested
1. Use dependency injection patterns where appropriate
1. Write unit tests for new functionality. Use table-driven unit tests when possible.
1. Review and update relevant documentation in `cli/azd/docs/` when making major changes or adding new features

## Testing Approach
- Unit tests in `*_test.go` files alongside source code
- Integration tests in `cli/azd/test/functional/` for end-to-end workflows
- Mock external services using interfaces and dependency injection
- Use testify for assertions and test organization
- Test both success and error scenarios extensively
- Update test snapshots in `cli/azd/cmd/testdata/*.snap` when changing CLI help output by setting `UPDATE_SNAPSHOTS=true` before running `go test`
