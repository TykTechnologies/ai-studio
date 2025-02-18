# Detailed Explanation of Secrets Functionality & Notifications Documentation

## 1. Overview of Secrets Functionality

The **Midsommar** codebase includes a secure secrets management feature enabling administrators and services to store sensitive data (like passwords, tokens, API keys) in an encrypted format. This prevents exposure of credentials and aligns with security best practices.

## 2. Secret References

### Understanding Secret References

The system supports two types of references for accessing sensitive data:

1. **Secret References** use the format `$SECRET/VAR_NAME` to reference stored secrets. For example, `$SECRET/OPENAI_KEY` references a secret stored with the variable name "OPENAI_KEY".

2. **Environment Variables** use the format `$ENV/VAR_NAME` to reference environment variables. For example, `$ENV/API_KEY` will resolve to the value of the API_KEY environment variable.

These patterns allow for:
- Secure storage of sensitive values
- Easy reference in configuration
- Consistent secret management across the application
- Flexible integration with environment-based configuration

Both secret references and environment variables can be used in any string field where sensitive data needs to be stored securely. Common use cases include:

**Using Secret References:**
- API Keys (e.g., `$SECRET/OPENAI_KEY`)
- API Endpoints (e.g., `$SECRET/CUSTOM_API_ENDPOINT`)
- Database Credentials
- Service URLs
- Authentication Tokens

**Using Environment Variables:**
- Development Configuration (e.g., `$ENV/DEBUG_MODE`)
- Local API Keys (e.g., `$ENV/LOCAL_API_KEY`)
- Service-specific Settings (e.g., `$ENV/SERVICE_PORT`)
- Runtime Configuration (e.g., `$ENV/LOG_LEVEL`)

### When to Use Secret References

1. **API Responses (preserveRef=true)**
   - When returning data through API endpoints
   - When displaying configuration in the UI
   - When editing/updating configurations
   Example:
   ```go
   // In API handlers or responses
   llm.APIKey = secrets.GetValue(llm.APIKey, true) // Keeps "$SECRET/OPENAI_KEY"
   llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, true) // Keeps "$SECRET/CUSTOM_API_ENDPOINT"
   ```

2. **Runtime Operations (preserveRef=false)**
   - When making actual API calls
   - When connecting to databases
   - When the actual secret value is needed
   Example:
   ```go
   // In service layer or when making API calls
   llm.APIKey = secrets.GetValue(llm.APIKey, false) // Resolves to actual value
   llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, false) // Resolves to actual endpoint
   ```

### Implementation Guidelines

1. **API Layer**
   ```go
   // Always preserve references in API responses
   func (s *Service) GetLLMByID(id uint) (*models.LLM, error) {
       llm := models.NewLLM()
       if err := llm.Get(s.DB, id); err != nil {
           return nil, err
       }
       // Preserve references for all secret and environment fields
       llm.APIKey = secrets.GetValue(llm.APIKey, true)         // e.g., "$SECRET/OPENAI_KEY"
       llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, true) // e.g., "$SECRET/CUSTOM_API_ENDPOINT"
       llm.LogLevel = secrets.GetValue(llm.LogLevel, true)     // e.g., "$ENV/LOG_LEVEL"
       return llm, nil
   }
   ```

2. **Service Layer**
   ```go
   // Resolve actual values for operations
   func (s *Service) GetActiveLLMs() (models.LLMs, error) {
       llms := models.LLMs{}
       if err := llms.GetActiveLLMs(s.DB); err != nil {
           return nil, err
       }
       for i := range llms {
           // Resolve all secret and environment fields
           llms[i].APIKey = secrets.GetValue(llms[i].APIKey, false)         // resolves $SECRET or $ENV
           llms[i].APIEndpoint = secrets.GetValue(llms[i].APIEndpoint, false) // resolves $SECRET or $ENV
           llms[i].LogLevel = secrets.GetValue(llms[i].LogLevel, false)     // resolves $ENV/LOG_LEVEL to actual value
       }
       return llms, nil
   }
   ```

3. **Proxy/Runtime Layer**
   ```go
   // Always resolve values when making external calls
   config.Token = secrets.GetValue(config.Token, false)     // resolves $SECRET/API_TOKEN
   config.Endpoint = secrets.GetValue(config.Endpoint, false) // resolves $SECRET/API_ENDPOINT
   config.Debug = secrets.GetValue(config.Debug, false)     // resolves $ENV/DEBUG_MODE
   ```

### Best Practices

1. **API Responses**
   - Always preserve references (`preserveRef=true`) in API responses
   - This prevents accidental exposure of secrets in logs or UI
   - Makes configuration editable without losing secret references
   - Apply to all fields that might contain secrets

2. **Service Operations**
   - Resolve values (`preserveRef=false`) when actual secret is needed
   - Only resolve at the point of use
   - Keep secret values in memory for minimal time
   - Remember to resolve all secret fields before use

3. **Testing**
   - Test both reference types and behaviors:
     ```go
     // Test API response preserves references
     assert.Equal(t, "$SECRET/OPENAI_KEY", response.APIKey)
     assert.Equal(t, "$SECRET/CUSTOM_API_ENDPOINT", response.APIEndpoint)
     assert.Equal(t, "$ENV/DEBUG_MODE", response.DebugMode)
     
     // Test operation gets actual values
     assert.Equal(t, "actual-secret-value", resolvedKey)
     assert.Equal(t, "https://api.example.com", resolvedEndpoint)
     assert.Equal(t, "true", resolvedDebugMode) // from environment variable
     
     // Test environment variable resolution
     os.Setenv("TEST_MODE", "development")
     assert.Equal(t, "development", secrets.GetValue("$ENV/TEST_MODE", false))
     ```

4. **Error Handling**
   - Handle cases where secret might not exist
   - Provide clear error messages without exposing sensitive data
   - Consider fallback strategies for missing secrets
   - Validate resolved values before use (e.g., valid URLs)

[Rest of the original content remains unchanged...]
