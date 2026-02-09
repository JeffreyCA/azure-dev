## Title
Introduce AI model/quota framework primitives and agent-init recovery flow

## Related Issues
- Closes #6718
- Parent: #6708

## Summary
This PR introduces first-class AI model/quota primitives in the azd extension framework and defines `azure.ai.agents init` recovery UX for model/location/quota failures.

This PR provides:
- Data primitives through `AzdClient.Ai()`
- Interactive model/deployment/location prompts through `AzdClient.Prompt()`
- Quota-aware location selection for a specific model with explicit user recovery paths

## What Changed

### New framework AI service methods
Added `AiModelService` methods:
- `ListModels`
- `ResolveModelDeployments`
- `ListUsages`
- `ListLocationsWithQuota`
- `ListModelLocationsWithQuota`

`ListModelLocationsWithQuota` returns location entries with `max_remaining_quota` for model-specific location pickers.

### New framework prompt methods
Added `PromptService` methods:
- `PromptAiModel`
- `PromptAiDeployment`
- `PromptAiLocationWithQuota`
- `PromptAiModelLocationWithQuota`

`PromptAiModelLocationWithQuota` includes spinner-based loading before presenting choices.
`PromptAiModel` includes quota-aware filtering across available locations when `filter.locations` is not set.

### `azure.ai.agents` init recovery flow
Added recovery behavior in `init` to avoid abrupt exits and reduce dead-end loops:
- Added a third recovery option: `Choose a different model (all regions)`
- After selecting all-regions model, immediately prompts for location with quota for that model
- Handles no-models/no-locations cases by re-entering recovery prompt instead of failing
- Recovery menu copy includes explicit location/model context
- Centralized `AZURE_LOCATION` environment updates via helper (`updateEnvLocation`)
- Switched recovery control flow to iterative handling (instead of recursive re-entry) for clearer state transitions

### Prompt UX behavior
- Version labels include quota context when available (for example, `[up to X quota available]`)
- SKU labels and location labels use `quota available` wording
- SKU selection is always shown, even when there is only one valid SKU candidate

## API Semantics and Design Notes

### Two-layer model
- **Data layer** (`AzdClient.Ai()`): list/resolve/usage/quota primitives
- **Prompt layer** (`AzdClient.Prompt()`): interactive model/location/deployment selection

### Location semantics
- `PromptAiModel` location scoping is defined by `filter.locations` only.
- `PromptAiDeployment` location scoping is defined by `options.locations` only.
- If those location lists are empty, prompts operate across subscription locations.
- For `PromptAiModel` with quota enabled:
  - empty `filter.locations` applies quota filtering across available locations
  - one explicit location (`filter.locations` length 1) applies single-location quota filtering
  - multiple explicit locations (`filter.locations` length > 1) apply quota filtering across that provided location set
  - models are kept when quota is sufficient in at least one effective location
- Quota-aware deployment selection still requires exactly one explicit location (`PromptAiDeployment`).

### Capacity semantics
- Prompt-time capacity input is validated against SKU constraints (`min`, `max`, `step`).
- Prompt-time capacity input is also validated against available quota when quota checks are enabled.

## Validation
Executed successfully:

```bash
# from cli/azd
go test ./pkg/ai ./internal/grpcserver -count=1
go build ./...

# from cli/azd/extensions/azure.ai.agents
go test ./internal/cmd/... -count=1
go build ./...
```
