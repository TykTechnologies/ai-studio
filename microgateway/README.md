# Microgateway

A production-ready microgateway for AI/LLM API management built on the Midsommar AI Gateway library.

## Features

- 🚀 **High Performance**: Built with Go for low latency and high throughput
- 🔐 **Token Authentication**: Secure API access with token-based authentication
- 📊 **Analytics & Monitoring**: Real-time analytics and usage tracking
- 💰 **Budget Management**: Per-app and per-LLM budget controls
- 🔄 **Multi-LLM Support**: OpenAI, Anthropic, Google, Vertex AI, Ollama
- 🗄️ **Database Flexibility**: PostgreSQL for production, SQLite for development
- 🎯 **Management API**: Full CRUD operations for all entities
- 🏥 **Health Checks**: Kubernetes-ready health and readiness endpoints
- 📦 **Easy Deployment**: Docker, Kubernetes, and binary distributions

## Quick Start

### Using Docker Compose

```bash
# Clone the repository (if part of midsommar project)
cd microgateway

# Start with Docker Compose
docker-compose -f deployments/docker-compose.yml up

# The gateway will be available at http://localhost:8080
```

### Using Binary

```bash
# Build the binary
make build

# Create configuration
cp configs/.env.example .env
# Edit .env with your settings

# Run migrations
make migrate

# Start the gateway
make run
```

## Configuration

The gateway is configured through environment variables. See `configs/.env.example` for all available options.

Key configuration areas:
- **Server**: Port, TLS, timeouts
- **Database**: PostgreSQL or SQLite connection
- **Cache**: In-memory caching settings
- **Security**: JWT secrets, encryption keys
- **Analytics**: Buffer size, flush intervals

## API Documentation

### Management API

The management API is available at `/api/v1` and requires admin authentication.

#### Health Endpoints
- `GET /health` - Basic health check
- `GET /ready` - Readiness check with dependency validation

#### LLM Management
- `GET /api/v1/llms` - List LLMs
- `POST /api/v1/llms` - Create LLM
- `GET /api/v1/llms/{id}` - Get LLM
- `PUT /api/v1/llms/{id}` - Update LLM
- `DELETE /api/v1/llms/{id}` - Delete LLM

#### App Management
- `GET /api/v1/apps` - List apps
- `POST /api/v1/apps` - Create app
- `GET /api/v1/apps/{id}` - Get app
- `PUT /api/v1/apps/{id}` - Update app
- `DELETE /api/v1/apps/{id}` - Delete app

### Gateway API

The gateway proxies requests to configured LLMs:

```bash
# OpenAI-compatible endpoint
POST /llm/rest/{llm-slug}/chat/completions

# Streaming endpoint
POST /llm/stream/{llm-slug}/chat/completions
```

## Development

### Prerequisites

- Go 1.21+
- PostgreSQL 14+ or SQLite 3
- Make (optional but recommended)

### Building from Source

```bash
# Install dependencies
make deps

# Run tests
make test

# Build binary
make build

# Run with hot reload (development)
make dev
```

### Running Tests

```bash
# Unit tests
make test-unit

# Integration tests
make test-integration

# E2E tests
make test-e2e

# All tests with coverage
make coverage
```

### Development Workflow

```bash
# Format code
make fmt

# Vet code
make vet

# Lint code
make lint

# Security scan
make security

# Run all checks
make fmt vet lint test
```

## Deployment

### Docker

```bash
# Build Docker image
make docker-build

# Start with docker-compose
make docker-compose-up

# Stop services
make docker-compose-down
```

### Production Checklist

- [ ] Change default JWT secret (`JWT_SECRET`)
- [ ] Set strong encryption key (`ENCRYPTION_KEY`)
- [ ] Configure TLS certificates
- [ ] Set up database backups
- [ ] Configure monitoring/alerting
- [ ] Set appropriate resource limits
- [ ] Enable audit logging
- [ ] Configure rate limiting
- [ ] Set up log aggregation
- [ ] Review security settings

## Architecture

The microgateway follows a clean three-tier architecture:

- **Model Layer**: Database models and CRUD operations
- **Service Layer**: Business logic and data access
- **API Layer**: REST interface and request handling

### Key Components

1. **Authentication**: Token-based auth with caching
2. **Database Layer**: GORM-based with migration support
3. **Service Container**: Dependency injection and lifecycle management
4. **Analytics**: Buffered event collection and analysis
5. **Budget Control**: Real-time cost tracking and enforcement

### Database Schema

- `api_tokens` - API authentication tokens
- `apps` - Application configurations
- `llms` - LLM provider configurations
- `credentials` - App credential pairs
- `budget_usage` - Usage and cost tracking
- `analytics_events` - Request/response events

## Configuration Reference

### Environment Variables

#### Server Configuration
- `PORT` - Server port (default: 8080)
- `HOST` - Server host (default: 0.0.0.0)
- `TLS_ENABLED` - Enable HTTPS (default: false)

#### Database Configuration
- `DATABASE_TYPE` - Database type: sqlite/postgres
- `DATABASE_DSN` - Database connection string
- `DB_AUTO_MIGRATE` - Run migrations on startup (default: true)

#### Security Configuration
- `JWT_SECRET` - JWT signing secret (required)
- `ENCRYPTION_KEY` - AES encryption key, 32 characters (required)

#### Cache Configuration
- `CACHE_ENABLED` - Enable token caching (default: true)
- `CACHE_MAX_SIZE` - Maximum cache entries (default: 1000)
- `CACHE_TTL` - Cache time-to-live (default: 1h)

## Troubleshooting

### Common Issues

1. **Database Connection Errors**
   - Verify DATABASE_DSN is correct
   - Check database server is running
   - Ensure proper permissions

2. **Authentication Failures**
   - Verify JWT_SECRET is set correctly
   - Check token format and expiration
   - Validate app permissions

3. **Performance Issues**
   - Enable caching with CACHE_ENABLED=true
   - Adjust ANALYTICS_BUFFER_SIZE
   - Monitor database query performance

### Logging

The gateway uses structured JSON logging by default. Set `LOG_LEVEL=debug` for detailed output.

```bash
# View logs in development
make docker-compose-up
docker-compose logs -f microgateway
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the same license as the parent Midsommar project.