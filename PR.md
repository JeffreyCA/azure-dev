## Fix progress bar flickering in container operations when using extensions

### Issue
When running `azd up` with the `azure.ai.agents` extension installed, the progress bar flickered rapidly between multiple progress messages:

**During provision action:**
- Flickered between "Initialize bicep provider" and "Packaging service echo-agent (Tagging container image)"
- The "Initialize bicep provider" message was stale from a previous step

**During deploy action:**
- Flickered between "Publishing service echo-agent (Pushing container image)" and "Deploying service echo-agent (Starting Agent Container)"
- The "Publishing" message was stale from a previous operation

### Root Cause
The gRPC container service (`container_service.go`) was wrapping container operations with `async.RunWithProgress` and calling `console.ShowSpinner()` to display progress.

When extensions call these gRPC methods, this created a conflict:
- **Outer layer** (deploy.go): Calls `ShowSpinner()` with outer progress updates
- **Inner layer** (container_service.go): Also calls `ShowSpinner()` with inner progress updates from containerHelper

Both layers competed to update the same spinner UI, causing the flickering.

### Solution
Removed progress display from the container gRPC service. The service now:
1. Creates a progress channel (required by containerHelper methods)
2. Drains progress updates silently in a background goroutine
3. Lets the command layer handle all UI updates

This maintains proper separation of concerns:
- Internal gRPC services handle operations without UI side effects
- Command layer (deploy.go, etc.) owns all progress display
- Extensions get clean operation semantics without UI conflicts

### Changes
- Modified `Build()`, `Package()`, and `Publish()` methods in `cli/azd/internal/grpcserver/container_service.go`
- Replaced `async.RunWithProgress` observer pattern with silent progress draining
- No changes to containerHelper or command layer logic
