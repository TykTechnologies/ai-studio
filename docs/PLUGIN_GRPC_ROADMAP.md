# AI Studio Plugin gRPC Services - Roadmap & Gaps

## Current Implementation Status: 60% Complete

### ✅ **Phase 1: Core Infrastructure (100% Complete)**
- [x] Enhanced Plugin model with service access control
- [x] gRPC protobuf service definitions
- [x] Authentication & authorization interceptor
- [x] Service scope constants and enforcement
- [x] Universal manifest handling via GetManifest gRPC
- [x] Comprehensive testing framework

### ✅ **Phase 2: Core Services (80% Complete)**
- [x] Plugin Management Service (List, Get, UpdateConfig)
- [x] LLM Management Service (List, Get, GetPlugins)
- [x] Analytics Service (Summary, Usage, Cost - basic)
- [x] App Management Service (List, Get)
- [x] Tools Management Service (List, Get, GetOperations, CallOperation)
- [ ] **Missing**: gRPC server integration into main application
- [ ] **Missing**: Admin UI for service authorization

## Detailed Gap Analysis

### 🔴 **Critical Gaps (Blocks Production Use)**

#### **1. gRPC Server Integration**
**Status**: Not implemented
**Impact**: Framework exists but can't be used
**Files needed**:
- Main application server setup
- gRPC port configuration
- Interceptor registration
- Service registration

**Implementation needed**:
```go
// main.go or server setup
grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(grpc.PluginAuthInterceptor(db))
)
mgmtpb.RegisterAIStudioManagementServiceServer(grpcServer,
    grpc.NewAIStudioManagementServer(service))
```

#### **2. Admin Authorization UI**
**Status**: Backend methods exist, no frontend
**Impact**: Admins can't authorize plugin service access
**Files needed**:
- Plugin details page showing requested scopes
- Authorize/Revoke service access buttons
- Service scope explanation tooltips

### 🟡 **Medium Priority Gaps (Limits Plugin Capabilities)**

#### **3. Extended Service Coverage**

**Missing Services** (in priority order):

1. **Datasources & Data Catalogues** (~15 endpoints)
   - Datasource CRUD, embedding processing
   - Data catalogue management, tag associations
   - **Plugin Impact**: Can't manage RAG data sources
   - **Scopes needed**: `datasources.read`, `datasources.write`, `data-catalogues.read`

2. **Advanced Analytics** (~12 endpoints)
   - Chat records per day, tool usage over time
   - Model/vendor usage analysis, detailed breakdowns
   - **Plugin Impact**: Limited analytics dashboard capabilities
   - **Scopes needed**: `analytics.detailed`, `analytics.reports`

3. **Catalogues & Tags** (~12 endpoints)
   - LLM catalogue management, tag CRUD
   - Content organization and categorization
   - **Plugin Impact**: Can't organize or categorize content
   - **Scopes needed**: `catalogues.read`, `catalogues.write`, `tags.read`

#### **4. Performance Optimizations**
**Missing**:
- Connection pooling for plugin gRPC calls
- Caching for high-frequency analytics calls
- Request batching capabilities

### 🟢 **Low Priority Gaps (Nice to Have)**

#### **5. Administrative Services**
- Model Prices & Vendors (~8 endpoints)
- Chat & LLM Settings (~10 endpoints)
- **Impact**: Limited admin functionality in plugins

#### **6. Sensitive Services (Intentionally Excluded)**
- Credentials & Secrets (~8 endpoints)
- User & Group Management
- **Reason**: Too sensitive for plugin access

## Implementation Roadmap

### **Sprint 1: Production Readiness (Critical)**
**Goal**: Make framework usable in production

1. **Integrate gRPC server into main AI Studio application**
   - Add server setup to main.go
   - Configure gRPC port (default: 50052)
   - Register interceptors and services
   - Add configuration options

2. **Create admin UI for service authorization**
   - Add "Service Access" section to plugin details page
   - Show requested scopes with descriptions
   - Add authorize/revoke buttons
   - Display current authorization status

3. **Auto-extract scopes during plugin loading**
   - Wire `ExtractAndStoreServiceScopes()` into plugin registration
   - Automatically parse manifest when plugin is loaded
   - Update plugin database record with declared scopes

**Effort**: 1-2 weeks
**Value**: Framework becomes production-ready

### **Sprint 2: Extended Service Coverage (High Value)**
**Goal**: Support data management operations

1. **Add Datasources & Data Catalogues gRPC services**
   - Datasource CRUD operations
   - Data catalogue management
   - Tag associations
   - **Plugin Use Cases**: RAG data management, content organization

2. **Expand Analytics service with detailed endpoints**
   - Chat records over time
   - Tool usage analytics
   - Model/vendor breakdowns
   - **Plugin Use Cases**: Rich analytics dashboards

**Effort**: 2-3 weeks
**Value**: Enables data-focused plugins

### **Sprint 3: Performance & Polish (Optimization)**
**Goal**: Optimize for production scale

1. **Add connection pooling and caching**
   - gRPC connection pool for plugins
   - Analytics data caching (Redis/in-memory)
   - Request batching for bulk operations

2. **Add remaining services**
   - Catalogues & Tags management
   - Model Prices & Vendors (if needed)
   - System information services

**Effort**: 1-2 weeks
**Value**: Production performance and completeness

## Service Implementation Template

For adding new services to the gRPC framework:

### 1. Add to Protobuf
```protobuf
service AIStudioManagementService {
    // New service operations
    rpc ListDatasources(ListDatasourcesRequest) returns (ListDatasourcesResponse);
    rpc GetDatasource(GetDatasourceRequest) returns (GetDatasourceResponse);
}
```

### 2. Add Service Scopes
```go
// Add to models/plugin.go
const (
    ServiceScopeDatasourcesRead  = "datasources.read"
    ServiceScopeDatasourcesWrite = "datasources.write"
)
```

### 3. Implement Server
```go
// services/grpc/datasources_server.go
type DatasourcesServer struct {
    service *services.Service
}

func (s *DatasourcesServer) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
    // Implementation
}
```

### 4. Add to Auth Interceptor
```go
// services/grpc/auth_interceptor.go
scopeMap := map[string]string{
    "/ai_studio_management.AIStudioManagementService/ListDatasources": models.ServiceScopeDatasourcesRead,
}
```

### 5. Add to Unified Server
```go
// services/grpc/ai_studio_management_server.go
func (s *AIStudioManagementServer) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
    return s.datasourcesServer.ListDatasources(ctx, req)
}
```

## Testing Strategy

### Unit Tests
- Test each gRPC service method
- Test authentication and authorization
- Test model conversion (DB ↔ Protobuf)

### Integration Tests
- Test complete plugin authorization workflow
- Test plugin service access end-to-end
- Test error handling and fallbacks

### Performance Tests
- Test gRPC service performance vs REST API
- Test connection pooling effectiveness
- Test caching behavior

## Security Considerations

### Current Security Model
1. **Plugin ID Authentication**: Verified via AI Studio plugin manager
2. **Manifest-Declared Scopes**: Plugins declare required permissions
3. **Admin Authorization**: Explicit approval required for service access
4. **Runtime Enforcement**: Every gRPC method call validated

### Security Best Practices
- **Principle of Least Privilege**: Only grant minimum required scopes
- **Audit Logging**: Log all service access attempts
- **Scope Validation**: Validate scopes against known constants
- **Regular Review**: Periodically review plugin authorizations

## Success Metrics

### **Current Achievement**:
- ✅ **5 core services** implemented (Plugin, LLM, Analytics, Apps, Tools)
- ✅ **12+ service scopes** with granular permissions
- ✅ **Universal manifest handling** for all deployment types
- ✅ **Working example** (Rate Limiting Plugin) with real data integration
- ✅ **Comprehensive testing** (15+ test suites)

### **Production Ready Targets**:
- [ ] **gRPC server integration** into main application
- [ ] **Admin authorization UI** for service access management
- [ ] **10+ additional services** for comprehensive API coverage
- [ ] **Performance optimization** with caching and pooling

### **Long-term Vision**:
- **Complete API coverage**: All non-sensitive AI Studio services available
- **Plugin marketplace**: Rich ecosystem of service-integrated plugins
- **Advanced capabilities**: Plugins with full system integration
- **Developer experience**: Simple, secure service access for plugin developers

The framework provides a **solid foundation** for secure, scalable plugin service access with clear **extension patterns** for adding additional services as needed.