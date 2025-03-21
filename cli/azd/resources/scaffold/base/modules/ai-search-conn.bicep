@description('The name of the AI Hub')
param aiHubName string

@description('The name of the AI Search')
param aiSearchName string

resource search 'Microsoft.Search/searchServices@2024-06-01-preview' existing = {
  name: aiSearchName
}

resource hub 'Microsoft.MachineLearningServices/workspaces@2024-10-01' existing = {
  name: aiHubName

  resource AzureAISearch 'connections@2024-10-01' = {
    name: 'AzureAISearch-connection'
    properties: {
      category: 'CognitiveSearch'
      target: 'https://${search.name}.search.windows.net'
      authType: 'ApiKey'
      isSharedToAll: true
      credentials: {
        key: search.listAdminKeys().primaryKey
      }
      metadata: {
        ApiType: 'Azure'
        ResourceId: search.id
      }
    }
  }
}
