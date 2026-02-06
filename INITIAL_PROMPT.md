Can you dive deep into this issue and come up with a high quality plan: https://github.com/Azure/azure-dev/issues/6718
  Parent issue: https://github.com/Azure/azure-dev/issues/6708

  Currently, the gRPC-based extension framework has a number of gRPC services, like EnvironmentService, AccountService, etc.
  There's a growing need for AI related extensions to be able to look up available models that users are able to deploy, based on location, quota/usage, model preferences and constraints.

  We do have a few places in azd core where we've implemented our own model/quota/usage checks, see:
  - cli/azd/pkg/infra/provisioning/bicep/prompt.go
  cli/azd/pkg/azapi/cognitive_service.go

  The way they're implemented may not be the most optimal or following best idiomatic approach, but would like to reuse that and expand/build additional capabilities on top of that if possible, to expose methods that 
  could be helpful for extension devs that work with Foundry AI models, deployments, etc.

  See:
  https://learn.microsoft.com/en-us/azure/ai-foundry/openai/how-to/quota?view=foundry-classic&tabs=rest
  https://learn.microsoft.com/en-us/rest/api/aiservices/accountmanagement/usages/

  A separate workstream is happening in parallel in the azure.ai.agents extension to implement better model/location prompts UX, see:

  cli/azd/extensions/azure.ai.agents/internal/cmd/init.go
  cli/azd/extensions/azure.ai.agents/internal/pkg/azure/ai/model_catalog.go

  Once these extension framework capabilities are released, the goal is to update that extension to use the built-in methods/services instead.

  Note that the extension does not *yet* implement or take into account the user's or the subscription's available usages/quota, but should be something we anticipate for and incorporate.

  Think about what would make for useful API methods to expose, and also consider things not necessarily already implemented in azd core.