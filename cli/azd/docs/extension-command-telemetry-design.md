# Evaluation: Capturing Extension Command Paths for Telemetry

## Problem Summary

When azd invokes an extension command, the telemetry currently only captures the extension ID and version (via `fields.ExtensionId` and `fields.ExtensionVersion`), but not the full command path within the extension (e.g., `azd demo context` vs `azd demo prompt`). The challenge is:

1. **Privacy**: azd cannot distinguish between command arguments (safe) and user-provided parameters (potentially PII)
2. **No Command Tree Knowledge**: azd core doesn't know what commands an extension exposes
3. **Language Agnostic**: Extensions can be written in Go, Python, .NET, JavaScript, etc.

### Related Issues and PRs

- Issue: [#6097 - tracing: capture command path for extensions](https://github.com/Azure/azure-dev/issues/6097)
- PR: [#6010 - tracing: add usage attributes for extension installs and runs](https://github.com/Azure/azure-dev/pull/6010)

## Approaches Evaluated

### Approach 1: Static Commands List in Extension Manifest

**Description**: Add a `commands` array to `extension.yaml` listing all known command paths.

```yaml
# extension.yaml
id: microsoft.azd.demo
commands:
  - path: "context"
    description: "Displays context"
  - path: "prompt"
    description: "Display prompt capabilities"
  - path: "mcp serve"
    description: "Start MCP server"
```

**Pros**:
- Simple to implement in azd core
- No runtime overhead
- Already available during install (parsed from extension.yaml inside zip)
- Extension authors explicitly opt-in to telemetry for specific commands
- Language agnostic

**Cons**:
- Manual maintenance burden for extension authors
- Can become stale if extension adds commands without updating manifest
- Harder to capture flag/argument metadata for future telemetry enhancements
- Doesn't scale well to complex command trees with many subcommands

**Implementation complexity**: Low

---

### Approach 2: Leverage Existing `examples.usages` Field

**Description**: Reuse the existing `examples.usages` field in extension.yaml to extract known command paths.

**Pros**:
- Already exists in schema
- No schema changes required
- Extension authors may already populate this

**Cons**:
- Semantic mismatch: `examples` are for documentation, not command enumeration
- May not cover all commands (extensions might not have examples for every command)
- Parsing `usage` strings is fragile
- Not designed for this purpose

**Implementation complexity**: Low (but hacky)

---

### Approach 3: Hidden Subcommand for Command Discovery (Runtime Query)

**Description**: Extensions implement a hidden subcommand (e.g., `<ext> __commands` or `<ext> completion fig`) that azd can call to retrieve the command tree.

```bash
# azd calls this internally
./demo __commands --format json
# Returns:
# {"commands": [{"path": "context", "args": [], "flags": [...]}, ...]}
```

**Pros**:
- Always up-to-date with actual extension implementation
- Can include rich metadata (flags, arguments, descriptions)
- Could reuse Fig spec format for consistency
- Single source of truth (command structure comes from actual code)

**Cons**:
- Runtime overhead (need to invoke extension process)
- Requires all extensions to implement this subcommand
- Go extensions could leverage Cobra's built-in capabilities, but other languages need custom implementation
- When to call it? (during install, on-demand, cached?)

**Implementation complexity**: Medium-High

---

### Approach 4: Bundled Command Spec File in Extension Archive

**Description**: During `azd x pack`, automatically generate a `commands.json` or `fig-spec.json` file and include it in the extension archive. azd reads this file after install.

```
extension.tar.gz/
├── extension.yaml
├── demo (binary)
└── commands.json  # Auto-generated command tree
```

**Generation for Go extensions (first-class support)**:
```go
// In pack.go, add step to generate command spec
rootCmd := cmd.NewRootCommand()
spec := figspec.BuildSpec(rootCmd)
// Write to commands.json
```

**Pros**:
- No runtime overhead after install
- Generated automatically for Go extensions using existing figspec package
- Single source of truth from actual Cobra command tree
- Could include flags and arguments for future telemetry
- Language agnostic for consumption (just parse JSON)

**Cons**:
- Requires build-time tooling for each language
- Non-Go extensions need equivalent tooling
- File could become stale if extension is modified without re-packing

**Implementation complexity**: Medium (Go), High (other languages)

---

### Approach 5: Extension Reports Command Path via gRPC Callback

**Description**: Extension reports its actual invoked command path back to azd via the existing gRPC connection.

```go
// In extension code, after parsing args:
azdClient.Telemetry().ReportCommandPath(ctx, "demo context")
```

**Pros**:
- Accurate - extension knows exactly what was invoked
- No manifest maintenance
- Works for all languages that use gRPC client

**Cons**:
- Requires extension authors to add this call
- Can be forgotten or incorrectly implemented
- Happens after extension starts, not before

**Implementation complexity**: Medium

---

### Approach 6: Hybrid - Bundled Spec + Lazy Discovery

**Description**: Combine approaches 4 and 3:
1. Go extensions auto-generate `commands.json` during pack (using figspec)
2. Other extensions can either:
   - Manually create `commands.json`, OR
   - Implement `__commands` subcommand that azd calls on first invocation (cached)

**Pros**:
- Best of both worlds
- Go gets first-class automated support
- Other languages have flexibility
- Consistent JSON format across all methods

**Cons**:
- More complex implementation
- Two code paths to maintain

**Implementation complexity**: Medium-High

---

## Recommended Approach

**Primary Recommendation: Approach 4 (Bundled Command Spec) with Go First-Class Support**

This approach best balances the requirements:

1. **Privacy-safe**: Only known commands are logged
2. **Accurate**: Generated from actual command tree, not manual
3. **Performant**: No runtime overhead
4. **Extensible**: JSON format can include flags/args for future enhancements
5. **First-class Go support**: Reuses existing `figspec` package
6. **VS Code IntelliSense synergy**: Same source data powers both telemetry and completions

## Implementation Plan

### Phase 1: Define Command Spec Format

Create a simple JSON schema for command specs (simpler than full Fig spec, focused on telemetry needs):

```json
{
  "extensionId": "microsoft.azd.demo",
  "version": "0.4.0",
  "commands": [
    {
      "path": "context",
      "flags": ["--help", "--debug"]
    },
    {
      "path": "prompt", 
      "flags": ["--help", "--debug"]
    },
    {
      "path": "mcp serve",
      "flags": ["--transport", "--help", "--debug"]
    }
  ]
}
```

Alternatively, reuse the Fig spec JSON format for consistency since we already have the infrastructure.

### Phase 2: Modify Pack Process (Go Extensions)

Update `extensions/microsoft.azd.extensions/internal/cmd/pack.go` to:

1. Before creating archive, invoke the built binary with a hidden command (e.g., `--generate-command-spec`) 
2. Or build a utility that inspects the Go binary's Cobra tree and generates the spec
3. Include `commands.json` in the archive alongside `extension.yaml`

```go
// In packExtensionBinaries, after building
func generateCommandSpec(binaryPath, outputPath string) error {
    // Option A: Invoke binary with special flag
    cmd := exec.Command(binaryPath, "__export-commands", "--format", "json")
    output, err := cmd.Output()
    // Write to commands.json
    
    // Option B: For Go, use reflection/build tooling
    // (more complex but doesn't require extension cooperation)
}
```

### Phase 3: Modify Extension Install

In the extension manager, after extracting the archive:
1. Parse `commands.json` if present
2. Store command tree in extension metadata (in config.json or separate cache)

```go
type Extension struct {
    // ... existing fields
    Commands []CommandSpec `json:"commands,omitempty"`
}
```

### Phase 4: Use Command Tree for Telemetry

In `cmd/extensions.go` when the extension action runs:
1. After parsing args, match against known commands
2. Log the matched command path (or "unknown" if no match)

```go
func (a *extensionAction) Run(ctx context.Context) (*actions.ActionResult, error) {
    // ... existing code
    
    // Match args against known commands
    commandPath := matchCommandPath(extension.Commands, a.args)
    
    tracing.SetUsageAttributes(
        fields.ExtensionId.String(extension.Id),
        fields.ExtensionVersion.String(extension.Version),
        fields.ExtensionCommand.String(commandPath), // NEW
    )
    
    // ... invoke extension
}

func matchCommandPath(commands []CommandSpec, args []string) string {
    // Try to match args to known command paths
    // Return longest matching prefix
    // Return "unknown" if no match
}
```

### Phase 5: Fallback for Non-Go Extensions

For extensions not written in Go:
1. Allow manual `commands.json` creation
2. Document the schema in extension-framework.md
3. Consider providing language-specific tooling (CLI generators for Python/JS/C#)

## File Changes Required

### New files

| File | Purpose |
|------|---------|
| `cli/azd/pkg/extensions/command_spec.go` | Types and matching logic for command specs |
| `cli/azd/extensions/commands.schema.json` | JSON schema for commands.json |

### Modified files

| File | Changes |
|------|---------|
| `cli/azd/extensions/microsoft.azd.extensions/internal/cmd/pack.go` | Generate command spec during pack |
| `cli/azd/pkg/extensions/extension.go` | Add Commands field to Extension struct |
| `cli/azd/pkg/extensions/manager.go` | Load commands.json on install |
| `cli/azd/cmd/extensions.go` | Log command path in telemetry |
| `cli/azd/internal/tracing/fields/fields.go` | Add ExtensionCommand field |
| `cli/azd/extensions/extension.schema.json` | Optional: add commands property |
| `cli/azd/docs/extension-framework.md` | Document the feature |

## Alternative Consideration: Reuse Fig Spec Directly

Instead of a new `commands.json` format, we could reuse the existing Fig spec:
- Already generated by `azd completion fig`
- Already have full infrastructure in `internal/figspec/`
- Would require extensions to implement `completion fig` subcommand

However, this ties telemetry to shell completion, which may not be desired.

## Future Enhancements

Once the command spec infrastructure is in place:

1. **Flag telemetry**: Log which flags are used (without values)
2. **Command popularity**: Track most-used extension commands
3. **Error correlation**: Better error context with full command path
4. **Extension analytics**: Help extension authors understand usage patterns

## Summary Comparison

| Approach | Complexity | Accuracy | Maintenance | Runtime Cost | Go Support | Multi-lang |
|----------|------------|----------|-------------|--------------|------------|------------|
| 1. Static manifest | Low | Medium | High | None | Manual | Yes |
| 2. examples.usages | Low | Low | Medium | None | Manual | Yes |
| 3. Runtime query | Medium-High | High | Low | Medium | Easy | Varies |
| 4. Bundled spec | Medium | High | Low | None | Auto | Manual |
| 5. gRPC callback | Medium | High | Low | Low | Easy | Easy |
| 6. Hybrid | Medium-High | High | Low | Low | Auto | Flexible |

**Recommendation**: Start with **Approach 4** for Go extensions with auto-generation during pack, and allow manual `commands.json` creation for other languages. This provides the best balance of accuracy, performance, and developer experience.
