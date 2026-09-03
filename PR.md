# Title

feat(extensions): dependency-aware uninstall and ownership tracking

# Description

Fixes #XXXX

This PR makes `azd extension uninstall` dependency-aware, records why each extension was installed, and reorganizes `azd extension show` so it can explain an extension's dependencies and dependents. It closes the gap where uninstalling the `microsoft.foundry` extension pack stranded its seven dependencies and uninstalling one of those dependencies silently broke `azure.ai.agents`.

### Issue

Install and update understand extension dependencies, but uninstall does not, and the installed record carries nothing to reason with.

- `azd extension uninstall microsoft.foundry` removed only the empty pack record and left every dependency installed, with no hint that nothing needed it anymore.
- `azd extension uninstall azure.ai.projects` succeeded while `azure.ai.agents` and the pack still declared it, leaving them with a missing dependency.
- Shared and transitive dependencies (`azure.ai.inspector` and `azure.ai.projects` are required by both the pack and `azure.ai.agents`) had no safe handling in either direction.
- `azd extension show` could not list dependencies or reverse dependencies, failed for installed extensions that no registry lists, and printed blank rows for packs.

### Ownership tracking

Each installed record now stores the installed version's dependency list and an `installedAsDependency` flag, so uninstall planning never needs registry access.

- Installs by name (`azd extension install`, `azd init`, project auto-install) leave the flag unset and clear it on a record that a pack pulled in earlier; updates preserve it.
- Records written before this change carry neither value and are treated as installs by name with no known dependencies: never removed as orphans and never blocking. `azd extension update` backfills the dependency list on such records, including already-current children, so existing installs gain dependent protection after one update. Ownership is never guessed.

### Dependency-aware uninstall

The command plans the whole removal from the installed records before removing anything.

- Uninstalling an extension that other installed extensions require fails with the list of dependents. `--force` proceeds with a warning, and naming the dependents in the same command lifts the block.
- Dependencies that were installed for the removed extensions and are no longer required are listed and removed after one confirmation, transitively. The default answer is yes and `--no-prompt` takes it; declining keeps them and prints the command to remove them later. `--no-dependencies` skips the step, and kept dependencies are listed with the reason.
- Blank ids are rejected, since an empty id previously matched an arbitrary installed record, and `--all` runs through the same path.

```
$ azd extension uninstall azure.ai.projects
ERROR: extension azure.ai.projects is required by installed extensions: azure.ai.agents, microsoft.foundry
Suggestion: Run 'azd extension uninstall azure.ai.agents microsoft.foundry' to remove the dependents first, or pass --force to remove it anyway.

$ azd extension uninstall microsoft.foundry   # azure.ai.agents was installed by name earlier
The following dependencies were installed for microsoft.foundry and are no longer required:
  azure.ai.connections (1.0.0-beta.4)
  azure.ai.routines (1.0.0-beta.4)
  azure.ai.skills (1.0.0-beta.4)
  azure.ai.toolboxes (1.0.0-beta.5)

? Remove these 4 dependencies as well? (Y/n)

  (✓) Done: Uninstalling microsoft.foundry (1.0.0-beta.2)
  (✓) Done: Uninstalling azure.ai.connections dependency (1.0.0-beta.4, no longer required)
  (✓) Done: Uninstalling azure.ai.routines dependency (1.0.0-beta.4, no longer required)
  (✓) Done: Uninstalling azure.ai.skills dependency (1.0.0-beta.4, no longer required)
  (✓) Done: Uninstalling azure.ai.toolboxes dependency (1.0.0-beta.5, no longer required)
  (-) Skipped: Uninstalling azure.ai.agents dependency (1.0.0-beta.13, not installed as a dependency)
  (-) Skipped: Uninstalling azure.ai.inspector dependency (1.0.0-beta.5, required by azure.ai.agents)
  (-) Skipped: Uninstalling azure.ai.projects dependency (1.0.0-beta.8, required by azure.ai.agents)
```

### `azd extension show`

The layout follows `azd tool show` and now answers whether an extension can be installed, updated, or removed.

- Empty rows and sections are omitted, `Not installed` replaces `N/A`, and `Requires azd` and `Other Versions` rows carry update and compatibility annotations.
- New `Dependencies` and `Required By` sections list declared dependencies with their installed state and the installed extensions that require the extension.
- Installed extensions that no source lists (bundle installs, delisted) are shown from their installed record, and an installed extension listed by several sources uses the source it was installed from instead of prompting.
- JSON output switches to camelCase keys with `omitempty`, matching other azd commands. This is a breaking change for the previous PascalCase keys; the command group is beta and no in-repo consumer depends on them.

### Telemetry

Adds the `ext.uninstall` event, emitted once per removed extension with the existing id, version, and source category attributes. The command emits it, not the manager, so the uninstall an update performs internally does not count. No new fields.

### Testing

- Unit tests for uninstall planning: pack removal, shared and transitive dependencies, sibling ordering, blocked and combined errors, `--force`, `--no-dependencies`, legacy records, and blank ids.
- Unit tests for ownership recording on install, preservation across updates, promotion by install, init, and auto-install, and backfill of parents and already-current children.
- Command tests for uninstall output with the confirmation accepted and declined, the `ext.uninstall` span, and show resolution, JSON shape, and display layout.
- Manual run against the live `azd` registry covering every scenario above with `microsoft.foundry`.
- Usage and fig-spec snapshots regenerated; build, vet, lint, and cspell clean.

## Telemetry Change Checklist

### New Fields
- [x] N/A: no new fields (the `ext.uninstall` span reuses `extension.id`, `extension.version`, and `extension.source.category`)

### New Events
- [x] Event constant defined in `events/events.go`
- [x] Event constant is an exported string `const` whose Go identifier contains `Event` (end it with `Prefix` for a prefix-match group) so the GDPR classifier discovers it
- [x] Event documented in `docs/specs/metrics-audit/telemetry-schema.md`
- [x] Event follows naming convention (`prefix.noun.verb`)

### Privacy
- [x] Classification assigned using decision tree (existing fields only, all SystemMetadata)
- [x] No `CustomerContent` emitted in telemetry
- [x] No unhashed user-provided values
- [x] No PII in string attributes (names, emails, paths)
- [ ] Privacy review triggered (new event; reviewer to confirm)

### Testing
- [x] Unit test verifies attributes are set on the span
- [ ] Integration test confirms end-to-end emission (not applicable)
- [ ] Verified field appears correctly in local telemetry output

### Downstream
- [x] N/A: no dashboard, Kusto function, or LENS job queries the new event yet

### Documentation
- [x] Feature-telemetry matrix updated (`docs/specs/metrics-audit/feature-telemetry-matrix.md`)
- [x] Telemetry schema updated (`docs/specs/metrics-audit/telemetry-schema.md`)
- [x] Hashed-field table in `privacy-review-checklist.md` unchanged (nothing new is hashed)
- [x] This checklist is complete
