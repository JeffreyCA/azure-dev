## Title
Add AI model/quota framework primitives and improve agents init recovery flow

## Related Issues
- Closes #6718
- Parent: #6708

## Summary
This PR adds first-class AI model/quota primitives to the azd extension framework and improves `azure.ai.agents init` recovery UX for model/location/quota failures.

The API now cleanly supports:
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

### `azure.ai.agents` init flow improvements
Added recovery behavior in `init` to avoid abrupt exits and reduce dead-end loops:
- Added a third recovery option: `Choose a different model (all regions)`
- After selecting all-regions model, immediately prompts for location with quota for that model
- Handles no-models/no-locations cases by re-entering recovery prompt instead of failing
- Improved recovery menu copy with explicit location/model context
- Centralized `AZURE_LOCATION` environment updates via helper (`updateEnvLocation`)

## API Semantics and Design Notes

### Two-layer model
- **Data layer** (`AzdClient.Ai()`): list/resolve/usage/quota primitives
- **Prompt layer** (`AzdClient.Prompt()`): interactive model/location/deployment selection

### Location semantics
- `PromptAiModel` location scoping is defined by `filter.locations` only.
- `PromptAiDeployment` location scoping is defined by `options.locations` only.
- If those location lists are empty, prompts operate across subscription locations.
- Quota-aware selection still requires exactly one explicit location for deployment/model-quota checks.

### Capacity semantics
- Prompt-time capacity input is validated against SKU constraints (`min`, `max`, `step`).

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
