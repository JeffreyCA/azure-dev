# Plan: AI Model Service for Extension Framework

**Issue**: [#6718](https://github.com/Azure/azure-dev/issues/6718) — Expose quota / model availability checks through extension framework  
**Parent**: [#6708](https://github.com/Azure/azure-dev/issues/6708) — [EPIC] Improve azd experience for quota availability

## Problem Statement

Extensions (e.g., `azure.ai.agents`) need to discover AI models, check quota/usage, and find suitable deployment configurations. Today, this logic exists in two places with duplicated patterns:

1. **azd core** (`pkg/azapi/cognitive_service.go`, `internal/cmd/add/add_select_ai.go`, `pkg/infra/provisioning/bicep/prompt.go`) — flat model lists, quota checking for Bicep parameters, SKU location filtering
2. **Extension** (`extensions/azure.ai.agents/internal/pkg/azure/ai/model_catalog.go`) — richer filtering (capabilities, status, format, kind, location), version/SKU listing, deployment config resolution

Neither is exposed via the gRPC extension framework. The extension reimplements Azure SDK calls directly. The goal is to create a new `AiModelService` gRPC service that unifies and exposes these capabilities to all extensions.

## Proposed Approach

Create a new gRPC service called `AiModelService` that consolidates the model catalog, quota/usage, and deployment configuration functionality. The service will be backed by a new Go service layer (`pkg/ai/`) that wraps the existing `azapi` calls and adds higher-level logic (filtering, cross-location aggregation, quota-aware model selection).

### API Design

The gRPC service will expose these RPCs:

```protobuf
service AiModelService {
  // List AI models, optionally filtered by location, capabilities, format, kind, status.
  // If no location is specified, aggregates across all available locations (concurrent).
  rpc ListModels (ListModelsRequest) returns (ListModelsResponse);

  // Get available versions for a specific model in a location.
  rpc ListModelVersions (ListModelVersionsRequest) returns (ListModelVersionsResponse);

  // Get available SKUs for a specific model+version in a location.
  rpc ListModelSkus (ListModelSkusRequest) returns (ListModelSkusResponse);

  // Get deployment configuration for a model, resolving defaults for version/SKU/capacity.
  rpc GetModelDeployment (GetModelDeploymentRequest) returns (GetModelDeploymentResponse);

  // Get quota/usage for AI services in a location (TPM allocation, remaining capacity).
  rpc ListUsages (ListUsagesRequest) returns (ListUsagesResponse);

  // Find locations that have sufficient quota for specified usage requirements.
  // Useful for pre-flight checks before deployment.
  rpc ListLocationsWithQuota (ListLocationsWithQuotaRequest) returns (ListLocationsWithQuotaResponse);

  // Get available SKU locations for AI Services resource types.
  rpc ListSkuLocations (ListSkuLocationsRequest) returns (ListSkuLocationsResponse);
}
```

### Domain-Specific Prompt RPCs (on PromptService)

In addition to the data-only `AiModelService`, we'll add two high-value prompt RPCs to the existing `PromptService`. This follows the established pattern where domain-specific prompts (like `PromptSubscription`, `PromptLocation`, `PromptResourceGroup`) internalize data loading, filtering, display formatting, smart defaults, and no-prompt fallback — things every extension would otherwise duplicate.

**Why these two?** Looking at the extension's model selection flow in `init.go`, there are two high-complexity UX patterns (~300 lines combined) that every AI extension would need to reimplement:

1. **`PromptAiModel`** — The full model selection experience:
   - Loads model catalog (with spinner), filters by capabilities/format/kind/status
   - Validates model availability in the current location
   - Handles model→location mismatches: offers "pick different model in this location" vs "pick different location for this model"
   - Display formatting with model names
   - Returns: selected model name + resolved location

2. **`PromptAiModelDeployment`** — The version→SKU→capacity wizard for a chosen model:
   - Lists available versions with default pre-selected
   - Lists available SKUs with smart fallback chain (GlobalStandard → DataZoneStandard → Standard)
   - Prompts for capacity if no default exists
   - Returns: complete deployment configuration (version, SKU name, capacity)

**What we intentionally DON'T add** as individual prompts: `PromptModelVersion`, `PromptModelSku`, `PromptCapacity` — these are simple enough that `Select()` + the data RPCs suffice for extensions wanting fully custom flows.

### Key Design Decisions

1. **Unified data model**: Use the extension's richer `FilterOptions` pattern (capabilities, statuses, formats, kinds, locations) as the basis for filtering, since it's a superset of what core uses.

2. **Location-aggregated model map**: Models are grouped by name with per-location details (following the extension's `AiModel.ModelDetailsByLocation` pattern), which is more useful than flat per-location lists.

3. **Quota as first-class citizen**: Unlike the extension which doesn't yet consider quota, the service will include `ListUsages` and `ListLocationsWithQuota` RPCs from the start, based on core's existing `locationsWithQuotaFor()` logic.

4. **Subscription-scoped**: All RPCs take `subscription_id` as a required field. Location is optional for `ListModels` (queries all locations if omitted).

5. **Service layer in `pkg/ai/`**: Create a new package with a `ModelService` that wraps `azapi.AzureClient` calls. This allows both the gRPC service AND internal azd commands to share the same logic, replacing the duplicated implementations in `add_select_ai.go` and `prompt.go` over time.

6. **Two-tier API surface**: Data RPCs on `AiModelService` for programmatic access + UX-integrated prompt RPCs on `PromptService` for interactive flows. Extensions choose which tier to use based on their needs.

7. **Kind as filter, not interactive prompt**: Model "Kind" (e.g., `"OpenAI"`, `"AIServices"`) represents which Azure service hosts the model. Both core flows and the extension pre-filter by kind rather than prompting interactively. `PromptAiModel` accepts kinds in its filter options. For the rare case of interactive kind selection, extensions can compose `ListModels()` → extract kinds → `Select()` → `PromptAiModel(filter: kind=chosen)`.

### Core Flow Alignment Verification

The proposed RPCs can reimplement all three core AI prompt flows:

| Core Flow | Reimplementation via RPCs | Notes |
|-----------|--------------------------|-------|
| `promptOpenAi()` (compose "Add OpenAI") | `Select(service type)` → `PromptAiModel(filter: kind=OpenAI, capabilities=chat\|embeddings)` | PromptAiModel handles "no models in location → try different location?" loop |
| `promptAiModel()` (compose "Add AI Model") | `PromptAiModel(filter: kind=AIServices)` → `PromptAiModelDeployment(model)` | Kind pre-filtered; interactive kind selection composable via `ListModels` + `Select` if needed |
| `promptForParameter()` with quota (Bicep) | `ListLocationsWithQuota(requirements)` → existing `PromptLocation(filtered)` | No new prompt RPC needed — composes cleanly with data RPCs |

## Workplan

### Phase 1: Proto Definition & Code Generation
- [ ] Create `grpc/proto/ai_model.proto` with `AiModelService` definition and messages
- [ ] Add `PromptAiModel` and `PromptAiModelDeployment` RPCs + messages to `grpc/proto/prompt.proto`
- [ ] Run `make proto` to generate Go code into `pkg/azdext/`

### Phase 2: Core Service Layer (`pkg/ai/`)
- [ ] Create `pkg/ai/model_service.go` — main service with methods matching the RPCs
  - `ListModels` — concurrent cross-location model fetching + filtering
  - `ListModelVersions` — version extraction for model+location
  - `ListModelSkus` — SKU extraction for model+version+location  
  - `GetModelDeployment` — deployment config resolution with fallback defaults
  - `ListUsages` — quota/usage retrieval per location
  - `ListLocationsWithQuota` — filter locations by quota requirements
  - `ListSkuLocations` — SKU location availability
- [ ] Create `pkg/ai/model_service_test.go` — unit tests with mocked azapi calls

### Phase 3: gRPC Server Implementation — AiModelService
- [ ] Create `internal/grpcserver/ai_model_service.go` — implements `azdext.AiModelServiceServer`
  - Converts between proto messages ↔ `pkg/ai/` domain types
  - Delegates to `pkg/ai/ModelService`
- [ ] Create `internal/grpcserver/ai_model_service_test.go` — tests for proto conversion

### Phase 4: gRPC Server Implementation — Prompt RPCs
- [ ] Add `PromptAiModel` implementation to `internal/grpcserver/prompt_service.go`
  - Loads model catalog via `pkg/ai/ModelService`, filters by options
  - Handles location mismatch: offers "different model" vs "different location"
  - Uses existing `console.Select()` for interactive selection
  - No-prompt fallback: resolve first matching model if non-interactive
- [ ] Add `PromptAiModelDeployment` implementation to `internal/grpcserver/prompt_service.go`
  - Walks through version → SKU → capacity selection sequence
  - Smart defaults: default version, SKU fallback chain, default capacity
  - Uses existing `console.Select()` / `console.Prompt()` for each step
- [ ] Add tests for the new prompt methods

### Phase 5: Registration & Wiring
- [ ] Add `aiModelService` field to `Server` struct in `internal/grpcserver/server.go`
- [ ] Add `aiModelService` parameter to `NewServer()` constructor
- [ ] Add `azdext.RegisterAiModelServiceServer()` call in `Start()`
- [ ] Add `container.MustRegisterScoped(grpcserver.NewAiModelService)` in `cmd/container.go`
- [ ] Register `pkg/ai/ModelService` in IoC container in `cmd/container.go`
- [ ] (Prompt RPCs use existing PromptService registration — no new wiring needed)

### Phase 6: Testing & Validation
- [ ] Build the project (`go build`)
- [ ] Run existing tests to ensure no regressions
- [ ] Run new unit tests
- [ ] Update command snapshots if needed (`UPDATE_SNAPSHOTS=true go test ./cmd -run 'TestFigSpec|TestUsage'`)

### Phase 6 (Future — not in scope for this PR): 
- Refactor `internal/cmd/add/add_select_ai.go` to use `pkg/ai/ModelService`
- Refactor `pkg/infra/provisioning/bicep/prompt.go` to use `pkg/ai/ModelService`
- Update `azure.ai.agents` extension to use gRPC `AiModelService` instead of direct SDK calls
- Add quota-aware model selection to the extension's init flow

## Proto Message Design (Detailed)

### Additions to `prompt.proto` (PromptService)

```protobuf
// --- New RPCs added to existing PromptService ---

// PromptAiModel prompts the user to select an AI model.
// Loads the model catalog, applies filters, handles location-model mismatch
// (offers "pick different model" vs "pick different location").
rpc PromptAiModel (PromptAiModelRequest) returns (PromptAiModelResponse);

// PromptAiModelDeployment prompts the user to configure a model deployment.
// Walks through version → SKU → capacity selection with smart defaults.
rpc PromptAiModelDeployment (PromptAiModelDeploymentRequest) returns (PromptAiModelDeploymentResponse);

// --- Messages ---

message PromptAiModelRequest {
  AzureContext azure_context = 1;         // subscription + location context
  AiModelFilterOptions filter = 2;        // optional filtering criteria
  string message = 3;                     // custom prompt message (default: "Select an AI model")
  optional string default_model = 4;      // pre-selected model name
}

message PromptAiModelResponse {
  string model_name = 1;                  // selected model name
  string location = 2;                    // resolved location (may differ if user switched)
  bool location_changed = 3;             // true if user chose a different location
}

message PromptAiModelDeploymentRequest {
  AzureContext azure_context = 1;         // subscription + location context
  string model_name = 2;                  // the model to configure
  repeated string preferred_skus = 3;     // SKU preference order (default: GlobalStandard, Standard)
  optional string default_version = 4;    // override default version selection
  optional int32 default_capacity = 5;    // override default capacity
}

message PromptAiModelDeploymentResponse {
  string model_name = 1;
  string format = 2;
  string version = 3;
  string sku_name = 4;
  string sku_usage_name = 5;
  int32 capacity = 6;
}
```

### New `ai_model.proto` (AiModelService — data-only RPCs)

```protobuf
// ---- Request/Response Messages ----

message ListModelsRequest {
  string subscription_id = 1;
  // If empty, aggregates models across all locations (concurrent)
  optional string location = 2;
  AiModelFilterOptions filter = 3;
}

message AiModelFilterOptions {
  repeated string capabilities = 1;  // e.g. ["chat", "embeddings"]
  repeated string statuses = 2;      // lifecycle status filter
  repeated string formats = 3;       // e.g. ["OpenAI"]
  repeated string kinds = 4;         // e.g. ["OpenAI", "AIServices"]
  repeated string locations = 5;     // filter by specific locations
}

message ListModelsResponse {
  repeated AiModel models = 1;
}

message AiModel {
  string name = 1;
  map<string, AiModelLocationDetails> details_by_location = 2;
}

message AiModelLocationDetails {
  repeated AiModelVersion versions = 1;
}

message AiModelVersion {
  string version = 1;
  string format = 2;
  string kind = 3;
  bool is_default_version = 4;
  string lifecycle_status = 5;
  map<string, string> capabilities = 6;
  repeated AiModelSku skus = 7;
}

message AiModelSku {
  string name = 1;           // e.g. "Standard", "GlobalStandard"
  string usage_name = 2;     // e.g. "OpenAI.Standard.gpt-4o"
  AiModelSkuCapacity capacity = 3;
}

message AiModelSkuCapacity {
  int32 default = 1;
  int32 maximum = 2;
  int32 minimum = 3;
  int32 step = 4;
}

// -- Versions --
message ListModelVersionsRequest {
  string subscription_id = 1;
  string model_name = 2;
  string location = 3;
}

message ListModelVersionsResponse {
  repeated string versions = 1;
  string default_version = 2;
}

// -- SKUs --
message ListModelSkusRequest {
  string subscription_id = 1;
  string model_name = 2;
  string location = 3;
  string version = 4;
}

message ListModelSkusResponse {
  repeated string skus = 1;
}

// -- Deployment Config --
message GetModelDeploymentRequest {
  string subscription_id = 1;
  string model_name = 2;
  repeated string preferred_locations = 3;
  repeated string preferred_versions = 4;
  repeated string preferred_skus = 5;  // defaults to ["GlobalStandard", "Standard"]
}

message GetModelDeploymentResponse {
  string name = 1;
  string format = 2;
  string version = 3;
  string location = 4;
  AiModelDeploymentSku sku = 5;
}

message AiModelDeploymentSku {
  string name = 1;
  string usage_name = 2;
  int32 capacity = 3;
}

// -- Usage/Quota --
message ListUsagesRequest {
  string subscription_id = 1;
  string location = 2;
}

message ListUsagesResponse {
  repeated AiUsage usages = 1;
}

message AiUsage {
  string name = 1;           // e.g. "OpenAI.S0.AccountCount"
  double current_value = 2;
  double limit = 3;
}

// -- Locations with quota --
message ListLocationsWithQuotaRequest {
  string subscription_id = 1;
  repeated string locations = 2;  // candidate locations to check (empty = all)
  repeated QuotaRequirement requirements = 3;
}

message QuotaRequirement {
  string usage_name = 1;    // e.g. "OpenAI.Standard.gpt-4o"
  double capacity = 2;      // minimum required remaining capacity
}

message ListLocationsWithQuotaResponse {
  repeated string locations = 1;
}

// -- SKU Locations --
message ListSkuLocationsRequest {
  string subscription_id = 1;
  string kind = 2;           // e.g. "AIServices"
  string sku_name = 3;       // e.g. "S0"
  string tier = 4;           // e.g. "Standard"
  string resource_type = 5;  // e.g. "accounts"
}

message ListSkuLocationsResponse {
  repeated string locations = 1;
}
```

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `grpc/proto/ai_model.proto` | Create | AiModelService + data message definitions |
| `grpc/proto/prompt.proto` | Edit | Add PromptAiModel + PromptAiModelDeployment RPCs and messages |
| `pkg/azdext/ai_model.pb.go` | Generated | Proto message Go code |
| `pkg/azdext/ai_model_grpc.pb.go` | Generated | gRPC service Go code |
| `pkg/azdext/prompt.pb.go` | Re-generated | Updated with new prompt messages |
| `pkg/azdext/prompt_grpc.pb.go` | Re-generated | Updated with new prompt RPCs |
| `pkg/ai/model_service.go` | Create | Core service layer wrapping azapi |
| `pkg/ai/model_service_test.go` | Create | Unit tests for model service |
| `internal/grpcserver/ai_model_service.go` | Create | gRPC AiModelService implementation |
| `internal/grpcserver/ai_model_service_test.go` | Create | Tests for AiModelService |
| `internal/grpcserver/prompt_service.go` | Edit | Add PromptAiModel + PromptAiModelDeployment implementations |
| `internal/grpcserver/server.go` | Edit | Add aiModelService field + registration |
| `cmd/container.go` | Edit | IoC registration for new services |

## Notes & Considerations

1. **Backward compatibility**: This is purely additive — no existing APIs change. New RPCs on `PromptService` use the `Unimplemented` base, so old clients are unaffected.
2. **Performance**: Cross-location model listing uses concurrent goroutines with `sync.Map`, matching existing patterns in both core and extension code.
3. **Auth**: The service uses the same subscription-based credential flow as `azapi.AzureClient`. The gRPC auth interceptor in `server.go` already handles extension authentication.
4. **Quota data model**: Usage entries follow Azure's `Cognitive Services Usages` API which returns `Name.Value`, `Limit`, and `CurrentValue`. The proto flattens this to `name`, `limit`, `current_value` for simplicity.
5. **Extension migration path**: Once this ships, `azure.ai.agents` can replace its `ModelCatalogService` with gRPC calls to `AiModelService`, and its ~300-line model selection flow with `PromptAiModel` + `PromptAiModelDeployment`, eliminating its direct Azure SDK dependency entirely.
6. **`pkg/ai/` vs inline in grpcserver**: Having a separate `pkg/ai/` package allows azd core commands (compose, Bicep prompts) to also use this consolidated logic, enabling future refactoring of the duplicated code.
7. **Two-tier API**: Extensions doing standard model selection use the prompt RPCs (2 calls). Extensions needing custom UX use the data RPCs to build their own flow. Both tiers share the same `pkg/ai/ModelService` backend.
8. **Prompt RPCs and `pkg/ai/ModelService` dependency**: The `PromptService` already holds lazy references to various services. Adding a dependency on `pkg/ai/ModelService` follows the same pattern (e.g., how `PromptLocation` depends on `account.SubscriptionsManager` for location listing).
