# GitHub Copilot instructions

This is the Azure Developer CLI - a Go-based CLI tool for managing Azure application development workflows. It handles infrastructure provisioning, application deployment, environment management, and project lifecycle automation. Please follow these guidelines when contributing.

## Code standards

### Required before each commit
- From `cli/azd/` directory, run `gofmt -s -w .` before committing any changes to ensure proper code formatting
- From `cli/azd/` directory, run `golangci-lint run ./...` to check for linting issues
- From `cli/azd/` directory, run `cspell lint "**/*.go" --relative --config ./.vscode/cspell.yaml --no-progress` to check spelling
- New Go files must include the standard copyright header:
  ```go
  // Copyright (c) Microsoft Corporation. All rights reserved.
  // Licensed under the MIT License.
  ```

### Development flow
**Prerequisites:** [Go](https://go.dev/dl/) 1.24

**Build `azd` binary:**
```bash
cd cli/azd
go build
```

**Test:**
- Quick tests: `go test ./... -short`
- All tests: `go test ./...`

## Repository structure
- `cli/azd/`: Main CLI application and command definitions
- `cli/azd/cmd/`: CLI command implementations (Cobra framework)
- `cli/azd/internal/`: Internal CLI packages (not for external use)
- `cli/azd/pkg/`: Reusable business logic (Azure services, deployment, infrastructure)
- `cli/azd/test/`: Test helpers and mocks
- `templates/`: Sample azd templates
- `schemas/`: JSON schemas for azure.yaml
- `ext/`: Extensions for VS Code, Azure DevOps, and Dev Containers
- `eng/`: Build scripts and CI/CD pipelines

## Key guidelines
1. Follow Go best practices and idiomatic patterns
1. Maintain existing code structure and organization, unless refactoring is necessary or directly requested
1. Use dependency injection patterns where appropriate
1. Write unit tests for new functionality. Use table-driven unit tests when possible.
1. Review and update relevant documentation in `cli/azd/docs/` when making major changes or adding new features

## Testing approach
- Unit tests in `*_test.go` files alongside source code
- Integration tests in `cli/azd/test/functional/` for end-to-end workflows
- Update test snapshots in `cli/azd/cmd/testdata/*.snap` when changing CLI help output by setting `UPDATE_SNAPSHOTS=true` before running `go test`

## Changelog updates for releases

When preparing a new release changelog, update `cli/azd/CHANGELOG.md` and `cli/version.txt`:

#### Step 1: Prepare version header
Rename any existing `## 1.x.x-beta.1 (Unreleased)` section to the version being released, and remove the `-beta.1` and `Unreleased` parts.

#### Step 2: Gather commits
**IMPORTANT**: Ensure you have the latest commits from main by first running the following `git fetch` commands before any `git log` commands:
```bash
git fetch --unshallow origin && git fetch origin main:refs/remotes/origin/main
```

**Determine cutoff commit**: Run this command to find the last changelog update commit:
```bash
git --no-pager log -n 3 --follow -p -- cli/azd/CHANGELOG.md
```
Look at the commit messages and diff output to identify the most recent commit that made actual changelog content updates. Ignore commits like "Increment CLI version after release".

**Gather commits since cutoff**: Run this command to get commits since the last release:
```bash
git --no-pager log --oneline --pretty=format:"%h %C(dim white)(%ad)%C(reset) %s" --date=short -20 origin/main
```

Increase `-20` if needed to find the cutoff commit. `git log` shows commits in reverse chronological order (newest first). You must identify the cutoff commit and exclude it along with any commits older than it.

#### Step 3: Create intermediate tracking table
For commits newer than the cutoff, create a table with these columns:
| Commit Hash | Date | PR# | Commit Message | GitHub Handle | Category | Final Entry | Include? |

#### Step 4: Process each commit iteratively
For each commit in your tracking table:

1. **Extract PR number**: Look for `(#XXXX)` pattern in commit message
2. **Fetch PR context**: 
    - Fetch the PR webpage: `https://github.com/Azure/azure-dev/pull/PR#`
    - Read the PR description and any linked GitHub issues
    - Fetch any linked issue webpages referenced in the PR (e.g., "Fixes #XXXX")
3. **Identify GitHub handle**: Convert email to GitHub handle (may require PR lookup)
4. **Determine category**: Features Added, Bugs Fixed, Breaking Changes, Other Changes
5. **Check external contributor**: Compare GitHub handle against `.github/CODEOWNERS`
6. **Write final entry**: Follow this format:
    ```md
    - [[PR#]](https://github.com/Azure/azure-dev/pull/PR#) User-friendly description.
    ```
    For external contributors, append: " Thanks @handle for the contribution!"
7. **Mark include decision**: Exclude non-customer-facing changes:
    - CI/CD changes
    - Internal tooling
    - Test-only changes
    - Dependency updates outside of `cli/azd`

#### Step 5: Organize by category
Group all entries marked "Include? = Yes" by their category.

#### Step 6: Validate and refine
- Ensure entries are concise and user-focused
- Start descriptions with action verbs (Add, Fix, Update, etc.)
- Verify all PR links are correctly formatted
- Remove categories with no entries
- Run cspell and add flagged GitHub handles to `vscode/cspell-github-user-aliases.txt` if needed

### Writing style
- Keep entries brief but informative, matching existing entries
- Describe impact to end users, not implementation details
- Start with verbs (Add, Fix, Update, etc.)
