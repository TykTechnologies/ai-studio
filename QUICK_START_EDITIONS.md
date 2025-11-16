# Quick Start: Community vs Enterprise Edition

## TL;DR

```bash
# Community Edition (public users)
git clone github.com/TykTechnologies/midsommar
cd midsommar
make build
# → bin/midsommar-ce, microgateway/dist/microgateway-ce

# Enterprise Edition (authorized users)
git clone github.com/TykTechnologies/midsommar
cd midsommar
make init-enterprise
make build
# → bin/midsommar-ent, microgateway/dist/microgateway-ent
```

## Which Edition Do I Have?

```bash
make show-edition
# Community Edition: "Current edition: ce"
# Enterprise Edition: "Current edition: ent"
```

## Build Commands

### Midsommar (Main App)

| Command | Output | Requires Enterprise |
|---------|--------|---------------------|
| `make build` | `bin/midsommar-{ce\|ent}` | Auto-detects |
| `make build-local` | `bin/midsommar-{ce\|ent}` | Auto-detects |
| `make build-enterprise` | `bin/midsommar-ent` | ✅ Yes |

### Microgateway

| Command | Output | Requires Enterprise |
|---------|--------|---------------------|
| `cd microgateway && make build` | `dist/microgateway-{ce\|ent}` | Auto-detects |
| `cd microgateway && make build-community` | `dist/microgateway-ce` | ❌ No |
| `cd microgateway && make build-enterprise` | `dist/microgateway-ent` | ✅ Yes |

## Running the Services

### Community Edition

```bash
# Midsommar
./bin/midsommar-ce

# Microgateway (edge mode)
cd microgateway
./dist/microgateway-ce -env=./envs/edge.env

# Microgateway (standalone)
./dist/microgateway-ce -env=./envs/standalone.env
```

### Enterprise Edition

```bash
# Midsommar
./bin/midsommar-ent

# Microgateway (edge mode)
cd microgateway
./dist/microgateway-ent -env=./envs/edge.env

# Microgateway (standalone)
./dist/microgateway-ent -env=./envs/standalone.env
```

## What's Different?

### Community Edition
- ✅ All proxy features
- ✅ Cost tracking
- ✅ Analytics dashboard
- ❌ No budget enforcement
- ❌ No budget alerts

### Enterprise Edition
- ✅ Everything in CE
- ✅ Budget enforcement
- ✅ Budget alerts (email)
- ✅ Advanced features

## Testing

```bash
# Test CE
make test

# Test ENT
make test BUILD_TAGS="-tags enterprise"
```

## Troubleshooting

### Error: "Enterprise submodule not initialized"

**Solution**: You don't have access to the enterprise repository.
- For access: contact enterprise@tyk.io
- Or build CE instead: `make build` (without init-enterprise)

### Error: "Import cycle not allowed"

**Solution**: This shouldn't happen. Report to maintainers.

### Binary won't run

**Check which edition:**
```bash
ls bin/
ls microgateway/dist/

# Run the correct edition
./bin/midsommar-ce          # Community
./bin/midsommar-ent         # Enterprise
```

## Quick Reference

### I want to...

**Build Community Edition**
```bash
make build
```

**Build Enterprise Edition (if I have access)**
```bash
make init-enterprise
make build
```

**Check my edition**
```bash
make show-edition
```

**Run tests**
```bash
make test
```

**Clean everything**
```bash
make clean
cd microgateway && make clean
```

**Update enterprise code (ENT team)**
```bash
make update-enterprise
git commit -m "Update enterprise submodule"
```

---

For complete documentation, see [ENTERPRISE_FRAMEWORK.md](ENTERPRISE_FRAMEWORK.md)
