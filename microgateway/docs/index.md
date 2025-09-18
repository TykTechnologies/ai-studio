# Microgateway Documentation

**Comprehensive documentation for the Microgateway AI/LLM management platform.**

The Microgateway is a production-ready gateway for AI/LLM API management built on the Midsommar AI Gateway library, providing centralized control, budgeting, analytics, and extensibility for AI applications.

## 📚 Documentation Structure

### **Getting Started**
- **[Introduction & Features](introduction.md)** - Overview, key features, and capabilities
- **[Compiling](compiling.md)** - Building the microgateway from source
- **[CLI Compilation](cli-compilation.md)** - Building the mgw CLI tool
- **[CLI Usage](cli-usage.md)** - Complete CLI reference and examples

### **Core Features**
- **[Budgets](features/budgets.md)** - Cost management and budget controls
- **[LLMs](features/llms.md)** - LLM provider management and configuration
- **[Analytics](features/analytics.md)** - Usage analytics and monitoring
- **[Proxy Logs](features/proxy-logs.md)** - Request/response logging and analysis
- **[Apps](features/apps.md)** - Application management and multi-tenancy
- **[API Keys (Tokens)](features/api-keys.md)** - Authentication and token management

### **Extensibility**
- **[How Plugins Work](extensibility/plugin-system.md)** - Plugin architecture overview
- **[Plugin Installation](extensibility/plugin-installation.md)** - Binary and folder installation
- **[Plugin Distribution](extensibility/plugin-distribution.md)** - OCI format distribution
- **[Plugin Hooks & Interfaces](extensibility/plugin-hooks.md)** - Available hooks and APIs
- **[Data Plugins](extensibility/data-plugins.md)** - Global data collection plugins
  - **[Configuring Data Plugins](extensibility/data-plugin-config.md)** - Configuration guide
  - **[AI Studio Integration](extensibility/ai-studio-logs.md)** - Sending logs to AI Studio

### **Scaling**
- **[Hub-and-Spoke Overview](scaling/hub-spoke-overview.md)** - Distributed architecture
- **[Controller to Edge](scaling/controller-edge.md)** - Microgateway controller patterns
- **[AI Studio to Microgateway](scaling/ai-studio-controller.md)** - AI Studio integration
- **[Namespaces](scaling/namespaces.md)** - Multi-tenant namespace management

### **Configuration**
- **[Environment Variables](config/environment-variables.md)** - Complete configuration reference
- **[Database Configuration](config/database.md)** - Database setup and tuning
- **[Security Configuration](config/security.md)** - Security settings and best practices
- **[Performance Tuning](config/performance.md)** - Optimization and scaling settings
- **[Monitoring Configuration](config/monitoring.md)** - Observability and metrics setup

---

## 🚀 Quick Start

1. **Build the microgateway**:
   ```bash
   cd microgateway
   make build-both
   ```

2. **Configure environment**:
   ```bash
   cp configs/.env.example .env
   # Edit .env with your settings
   ```

3. **Start the gateway**:
   ```bash
   ./dist/microgateway -migrate
   ./dist/microgateway
   ```

4. **Use the CLI**:
   ```bash
   export MGW_URL="http://localhost:8080"
   export MGW_TOKEN="your-admin-token"
   ./dist/mgw system health
   ```

## 📖 Key Concepts

- **Multi-Provider Support**: OpenAI, Anthropic, Google AI, Vertex AI, Ollama
- **Budget Management**: Real-time cost tracking and enforcement
- **Analytics & Monitoring**: Comprehensive usage analytics and performance metrics
- **Plugin System**: Extensible architecture with OCI-distributed plugins
- **Hub-and-Spoke**: Distributed deployment with centralized management
- **Multi-Tenancy**: Application isolation with separate credentials and budgets

## Documentation Structure Summary

This documentation provides comprehensive coverage of all microgateway capabilities:

- **23 Documentation Files**: Complete coverage of all aspects
- **Getting Started**: From compilation to first usage
- **Core Features**: All major functionality documented
- **Extensibility**: Complete plugin system documentation
- **Scaling**: Hub-and-spoke architecture with namespaces
- **Configuration**: Environment variables, database, security, performance, monitoring

## External Resources

- **[API Reference](../API_REFERENCE.md)** - Complete REST API documentation
- **[Build & Deploy Guide](../BUILD_DEPLOY.md)** - Deployment instructions
- **[CLI Examples](../CLI_EXAMPLES.md)** - CLI usage examples
- **[Configuration Guide](../CONFIGURATION.md)** - Legacy environment variable reference
- **[Admin Setup](../ADMIN_SETUP.md)** - Admin token bootstrap guide
- **[Features Overview](../FEATURES.md)** - Legacy feature documentation

---

*This documentation provides complete guidance from basic setup to advanced enterprise deployment and extensibility.*
