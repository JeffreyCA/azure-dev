## Fix progress bar flickering when using container extensions

### Problem
When running `azd up` with an agent extension that uses container operations, progress text flickered rapidly between two different messages:
- During provision: "Initialize bicep provider" ↔ "Packaging service echo-agent (Tagging container image)"
- During deploy: "Publishing service echo-agent (Pushing container image)" ↔ "Deploying service echo-agent (Starting Agent Container)"

Running `azd deploy` standalone did not exhibit this issue.

### Root Cause
The gRPC container service (used by extensions) was creating its own progress spinners that conflicted with the outer command layer's spinners. Both layers called `console.ShowSpinner()` simultaneously, causing competing goroutines to interleave their UI updates.

### Solution
Removed progress display from `container_service.go` Build/Package/Publish methods. The service now drains progress updates silently while the command layer handles all UI display. This maintains proper separation of concerns: internal services handle operations, command layer handles UI.

### Testing
- Verified code compiles successfully
- Extension mode now has single progress display source
- Standalone mode unchanged (containerHelper updates flow to command layer)
