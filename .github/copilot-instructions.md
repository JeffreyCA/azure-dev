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

## Changelog Updates for Releases

When preparing a new release changelog, update `cli/azd/CHANGELOG.md` and `cli/version.txt` following this process:

#### Step 1: Prepare Version Header
If there's an existing section with heading `## 1.x.x-beta.1 (Unreleased)`, rename it to the version being released. Do not create a new "Unreleased" section.

#### Step 2: Gather Raw Commits
Run this command to get commits since the last release:
```bash
git --no-pager log --oneline --pretty=format:"%h %C(dim white)(%ad)%C(reset) %s %C(dim white)[%an <%ae>]%C(reset)" --date=short -N
```
Start with N=20 and gather commits up to the first commit with message like "Release changelog for v1.17.0 (#5263)". If you don't see the cutoff commit, increase N until you find it.

#### Step 3: Create Tracking Table
For each commit, create a tracking table with these columns:
| Commit Hash | Date | PR# | Commit Message | Author Name | Author Email | GitHub Handle | Category | Final Entry | Include? |

#### Step 4: Process Each Commit Iteratively
For each commit in your tracking table:

1. **Extract PR Number**: Look for `(#XXXX)` pattern in commit message
2. **Fetch PR Context**: 
   - Fetch the PR webpage: `https://github.com/Azure/azure-dev/pull/PR#`
   - Read the PR description and any linked GitHub issues
   - Fetch any linked issue webpages referenced in the PR (e.g., "Fixes #XXXX")
3. **Identify GitHub Handle**: Convert email to GitHub handle (may require PR lookup)
4. **Determine Category**:
   - **Features Added**: New functionality, enhancements
   - **Breaking Changes**: Changes that may break existing functionality (rare)
   - **Bugs Fixed**: Bug fixes and corrections
   - **Other Changes**: Dependencies, internal improvements, non-user-facing changes
5. **Check External Contributor**: Compare GitHub handle against `.github/CODEOWNERS`
6. **Write Final Entry**: Follow this format:
   ```md
   - [[PR#]](https://github.com/Azure/azure-dev/pull/PR#) User-friendly description. [GitHub Handle]
   ```
   For external contributors, append: " Thanks @handle for the contribution!"
7. **Mark Include Decision**: Exclude non-customer-facing changes (build pipeline, internal tooling, etc.)

#### Step 5: Organize by Category
Group all entries marked "Include? = Yes" by their category.

#### Step 6: Validate and Refine
- Ensure entries are concise and user-focused
- Start descriptions with action verbs (Add, Fix, Update, etc.)
- Verify all PR links are correctly formatted

### Writing Style
- Keep entries brief but informative
- Describe impact to end users, not implementation details
- Start with verbs (Add, Fix, Update, Support, etc.)

### Exclusion Criteria
Exclude commits that are:
- CI/CD configuration changes
- Internal tooling updates
- Test-only changes
- Documentation-only updates (unless user-facing)
