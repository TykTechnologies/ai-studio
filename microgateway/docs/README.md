# Microgateway Documentation

This directory contains comprehensive documentation for the microgateway, including configuration guides, deployment examples, and operational procedures.

## Documentation Structure

### Hub-and-Spoke Architecture
- [**hub-spoke-overview.md**](./hub-spoke-overview.md) - Overview of the distributed architecture
- [**hub-spoke-configuration.md**](./hub-spoke-configuration.md) - Complete configuration guide
- [**hub-spoke-deployment.md**](./hub-spoke-deployment.md) - Deployment examples and patterns
- [**hub-spoke-operations.md**](./hub-spoke-operations.md) - Operational procedures and monitoring

### Reference Documentation
- [**configuration-reference.md**](./configuration-reference.md) - Complete environment variable reference
- [**api-reference.md**](./api-reference.md) - API endpoints and usage
- [**troubleshooting.md**](./troubleshooting.md) - Common issues and solutions

### Advanced Topics
- [**security.md**](./security.md) - Security considerations and best practices
- [**monitoring.md**](./monitoring.md) - Monitoring and observability setup
- [**migration.md**](./migration.md) - Migration guides and upgrade procedures

## Quick Start

For a quick hub-and-spoke setup, see the [deployment guide](./hub-spoke-deployment.md#quick-start).

## Architecture Modes

The microgateway supports three operational modes:

1. **Standalone** - Traditional single-instance deployment (default)
2. **Control** - Central hub managing configuration for edge instances
3. **Edge** - Lightweight gateway receiving configuration from control instance

Each mode is detailed in the respective documentation files.