# Tyk AI Studio

![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)
[![GitHub Latest Release](https://img.shields.io/github/v/release/TykTechnologies/ai-studio?color=8836FB)](https://github.com/TykTechnologies/ai-studio/releases)
[![GitHub Stars](https://img.shields.io/github/stars/TykTechnologies/ai-studio?logoColor=8836FB)](https://github.com/TykTechnologies/ai-studio/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/TykTechnologies/ai-studio.svg?logoColor=8836FB)](https://github.com/TykTechnologies/ai-studio/fork)

---
[Official Documentation](https://tyk.io/docs/ai-management/overview/) | [AI Studio Homepage](https://tyk.io/tyk-ai-studio/) | [Community Forum](https://community.tyk.io) | [Contributing](CONTRIBUTING.md)

**Open source AI management platform for secure, governed, and scalable AI integration**

Tyk AI Studio is an open source platform that addresses the critical challenges organizations face when adopting AI: shadow AI usage, compliance requirements, security concerns, and spiraling costs. Built for developers and platform teams, it provides structured AI management through governance, monitoring, and unified access controls.

## Why Tyk AI Studio?

AI adoption brings significant challenges that require structured solutions:

🚫 **Shadow AI Prevention** - Stop unauthorized AI tool usage without oversight  
🔒 **Security & Compliance** - Maintain data privacy and meet regulatory requirements  
💰 **Cost Control** - Monitor usage and prevent unlimited AI consumption  
🎯 **Unified Access** - Single interface for multiple AI vendors and internal tools  

## Core Capabilities

### AI Gateway
The heart of secure AI integration, providing:
- **Multi-vendor LLM support**: OpenAI, Anthropic, Mistral, Vertex AI, Bedrock, Gemini, Huggingface, Ollama, and custom models
- **Secure API proxying**: Centralized access control and credential management  
- **Usage monitoring**: Real-time tracking of costs, usage patterns, and performance
- **Rate limiting**: Prevent excessive token consumption and manage budgets
- **Content filtering**: Scriptable policy enforcement for data security

### AI Portal  
Developer-focused service catalog featuring:
- **Curated AI services**: Easy discovery and access to approved AI tools
- **Unified interface**: Single point of access for all AI capabilities
- **Integration support**: Connect with internal systems and external workflows
- **Rapid prototyping**: Secure experimentation with built-in policy controls

### AI Chat Interface
Collaborative workspace that provides:
- **Universal chat experience**: Single interface for LLMs, tools, and internal data sources
- **Custom chatrooms**: Dedicated spaces for teams, projects, or specific use cases
- **Built-in governance**: Policy enforcement without disrupting user workflows
- **Document integration**: Upload and use documents within AI conversations

### MCP Integration
Standards-based AI component integration:
- **Remote MCP support**: Connect internal APIs and tools without custom scripts
- **Local MCP servers**: Generate installable servers for testing or restricted environments
- **Standardized interactions**: Following Model Context Protocol specifications
- **Secure connections**: All MCP traffic routed through the AI Gateway for visibility

## Real-World Applications

**Software Development**
- Integrate AI with Jira, GitHub, and internal development tools
- Accelerate development cycles with AI-powered assistance
- Maintain security while enabling developer productivity

**Financial Services**  
- Ensure only anonymized data reaches external LLMs
- Track AI usage costs by department or team
- Meet compliance requirements while enabling innovation

**Healthcare**
- Route LLM traffic through governed pathways for HIPAA compliance
- Protect sensitive patient data with content filtering
- Enable AI-driven insights while maintaining privacy controls

**General Business Applications**
- Query internal APIs and databases through conversational interfaces
- Integrate AI with existing business systems and workflows
- Enable non-technical users to leverage AI capabilities safely

## Quick Start

Get up and running in under 5 minutes:

### Option 1: Docker (Recommended)

1. **Clone and setup**
```bash
git clone https://github.com/TykTechnologies/ai-studio.git
cd ai-studio
cp .env.example .env
```

2. **Configure environment**
Edit `.env` file with your AI provider credentials and desired settings.

3. **Start the platform**
```bash
docker compose up --build
```

4. **Access the interface**
- UI: http://localhost:3000
- API: http://localhost:8080
- Proxy: http://localhost:9090

When you first register, your account will automatically become admin with a default user group created.

### Option 2: Native Development

**Prerequisites**
- Go 1.22+
- Node.js 18+
- Clone the langchaingo fork: https://github.com/lonelycode/langchaingo

**Start development servers**
```bash
make start-dev  # Starts both frontend and backend
make stop-dev   # Stops both services
```

## Installation Options

### Docker
```bash
# Production deployment
docker compose up -d

# Development with hot reload
docker compose up --build
```

### Kubernetes
```bash
# Using Helm (if available)
helm install ai-studio ./helm

# Using kubectl
kubectl apply -f k8s/
```

### Native Binary
```bash
# Build production binary
make build

# The binary includes both API and UI
./ai-studio
```

## Architecture

Tyk AI Studio follows a clean three-tier architecture:

**Model Layer**: Data structures and database-level CRUD operations  
**Service Layer**: Business logic and data access to the model layer  
**API Layer**: REST interface to the service layer  

**Frontend Structure**:
- **Admin**: Managing AI models, tools, and data sources
- **Portal**: User interface for interacting with AI models and tools

The AI Gateway sits at the center, proxying all AI interactions while enforcing policies, monitoring usage, and maintaining security controls.

## Supported AI Vendors

- **OpenAI**: GPT models, Embeddings, and more
- **Anthropic**: Claude models and assistants  
- **Mistral**: Open source and commercial models
- **Google**: Vertex AI and Gemini models
- **AWS**: Bedrock model access
- **Hugging Face**: Open source model ecosystem
- **Ollama**: Local model deployment
- **Custom Models**: Bring your own model integrations

## Key Benefits

✅ **Reduce Risk** - Centralized access controls and comprehensive monitoring  
✅ **Improve Efficiency** - Streamlined workflows for both developers and end users  
✅ **Cost Control** - Real-time usage tracking with budgeting and rate limiting  
✅ **Support Compliance** - Built-in policy enforcement and audit logging  
✅ **Enable Scale** - Standards-based architecture supporting growth  

## Documentation & Resources

**Official Documentation**
- [AI Management Overview](https://tyk.io/docs/ai-management/overview/)
- [AI Studio Documentation](https://tyk.io/docs/ai-management/ai-studio/overview/)
- [MCP Integration Guide](https://tyk.io/docs/ai-management/mcps/overview/)

**Product Information**
- [AI Studio Homepage](https://tyk.io/tyk-ai-studio/)
- [Request Demo](https://tyk.io/ai-demo/)

**Developer Resources**
- [Contributing Guidelines](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)
- [Code of Conduct](CLA.md)

## Configuration

All configuration is managed through the `.env` file. Key settings include:

```bash
# AI Provider Credentials
OPENAI_API_KEY=your-openai-key
ANTHROPIC_API_KEY=your-anthropic-key

# Database Configuration
DATABASE_URL=your-database-url

# Telemetry (Optional)
TELEMETRY_ENABLED=false  # Disable usage statistics collection
```

**Privacy Note**: Tyk AI Studio collects anonymized usage statistics to improve the platform. No personal data or AI conversation content is collected. Set `TELEMETRY_ENABLED=false` to disable.

## Development

### Building from Source
```bash
# Build backend
go build

# Build with admin frontend included
make build

# Run tests
make test

# Clean build artifacts
make clean
```

### Additional Commands
```bash
make start-frontend    # Frontend development server only
make start-backend     # Backend development server only
make stop-frontend     # Stop frontend server
make stop-backend      # Stop backend server
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests for specific package
go test ./api/...

# Run tests with coverage
go test -coverprofile=coverage.out -coverpkg=./... ./...
```

## Community & Support

**Get Help**
- [Community Forum](https://community.tyk.io) - Technical support and discussions
- [GitHub Issues](https://github.com/TykTechnologies/ai-studio/issues) - Bug reports and feature requests
- [Official Documentation](https://tyk.io/docs/ai-management/overview/) - Comprehensive guides and references

**Contributing**
We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on how to:
- Report bugs and request features
- Submit pull requests
- Sign the Contributor License Agreement
- Join our development community

## Licensing

Tyk AI Studio is released under the GNU Affero General Public License v3.0. See [LICENSE.md](LICENSE.md) for full details.

**Contributor License Agreement**: All contributors must sign the [Tyk CLA](CLA.md) before contributions can be accepted.

## Project Status

Tyk AI Studio is actively developed and maintained by Tyk Technologies. It serves as the open source foundation for Tyk's AI management ecosystem.

**Stability**: Production ready for AI gateway and basic management features  
**Development**: Active development with regular releases  
**Community**: Welcoming contributions and feedback from the community  

---

**Built with ❤️ by Tyk**

If you're using Tyk AI Studio, give us a star ⭐️ and let us know how it's working for you!
