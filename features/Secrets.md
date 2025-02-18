# Detailed Explanation of Secrets Functionality & Notifications Documentation

## 1. Overview of Secrets Functionality

The **Midsommar** codebase includes a secure secrets management feature enabling administrators and services to store sensitive data (like passwords, tokens, API keys) in an encrypted format. This prevents exposure of credentials and aligns with security best practices.

## 2. Secret References

### Understanding Secret References

Secret references use the format `$SECRET/VAR_NAME` to reference stored secrets. For example, `$SECRET/OPENAI_KEY` references a secret stored with the variable name "OPENAI_KEY". This pattern allows for:
- Secure storage of sensitive values
- Easy reference in configuration
- Consistent secret management across the application

Secret references can be used in any string field where sensitive data needs to be stored securely. Common use cases include:
- API Keys (e.g., `$SECRET/OPENAI_KEY`)
- API Endpoints (e.g., `$SECRET/CUSTOM_API_ENDPOINT`)
- Database Credentials
- Service URLs
- Authentication Tokens

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
       // Preserve references for all secret fields
       llm.APIKey = secrets.GetValue(llm.APIKey, true)
       llm.APIEndpoint = secrets.GetValue(llm.APIEndpoint, true)
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
           // Resolve all secret fields
           llms[i].APIKey = secrets.GetValue(llms[i].APIKey, false)
           llms[i].APIEndpoint = secrets.GetValue(llms[i].APIEndpoint, false)
       }
       return llms, nil
   }
   ```

3. **Proxy/Runtime Layer**
   ```go
   // Always resolve values when making external calls
   config.Token = secrets.GetValue(config.Token, false) // resolve for API calls
   config.Endpoint = secrets.GetValue(config.Endpoint, false) // resolve endpoint
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
   - Test both behaviors:
     ```go
     // Test API response preserves references
     assert.Equal(t, "$SECRET/OPENAI_KEY", response.APIKey)
     assert.Equal(t, "$SECRET/CUSTOM_API_ENDPOINT", response.APIEndpoint)
     
     // Test operation gets actual values
     assert.Equal(t, "actual-secret-value", resolvedKey)
     assert.Equal(t, "https://api.example.com", resolvedEndpoint)
     ```

4. **Error Handling**
   - Handle cases where secret might not exist
   - Provide clear error messages without exposing sensitive data
   - Consider fallback strategies for missing secrets
   - Validate resolved values before use (e.g., valid URLs)

[Rest of the original content remains unchanged...]
