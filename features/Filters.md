# Filters

## Overview
Filters are a core component of the Midsommar system that provide a way to process and modify data or behavior in various contexts. They are primarily used in conjunction with Tools and Chats to control and customize functionality.

## Data Model

### Filter Structure
```go
type Filter struct {
    ID          uint   // Primary key
    Name        string // Name of the filter
    Description string // Description of what the filter does
    Script      []byte // The actual filter implementation script
}
```

## Core Functionality

### Filter Management
1. **CRUD Operations**
   - Create new filters with name, description, and script
   - Retrieve filters by ID or name
   - Update existing filters
   - Delete filters
   - List all filters with pagination support

2. **Tool Integration**
   - Filters can be associated with Tools through a many-to-many relationship
   - Tools can have multiple filters
   - Filters can be shared across multiple tools

3. **Chat Integration**
   - Filters can be associated with Chats
   - Multiple filters can be applied to a single chat
   - Filters affect chat behavior through their scripts

## Service Layer

### Filter Service
The Filter Service provides the following operations:

1. **Filter Creation**
   ```go
   CreateFilter(name, description string, script []byte) (*Filter, error)
   ```

2. **Filter Retrieval**
   ```go
   GetFilterByID(id uint) (*Filter, error)
   GetFilterByName(name string) (*Filter, error)
   GetAllFilters(pageSize int, pageNumber int, all bool) ([]Filter, int64, int, error)
   ```

3. **Filter Updates**
   ```go
   UpdateFilter(id uint, name, description string, script []byte) (*Filter, error)
   ```

4. **Filter Deletion**
   ```go
   DeleteFilter(id uint) error
   ```

### Tool Service Filter Operations
1. **Filter Association Management**
   ```go
   AddFilterToTool(toolID uint, filterID uint) error
   RemoveFilterFromTool(toolID uint, filterID uint) error
   GetToolFilters(toolID uint) ([]Filter, error)
   SetToolFilters(toolID uint, filterIDs []uint) error
   ```

## Script Execution Engine

### Scripting Package
The `scripting` package provides the core functionality for executing filter scripts:

1. **Script Runner**
   ```go
   scripting.NewScriptRunner(script []byte) *ScriptRunner
   ```
   - Creates a new script execution environment
   - Handles script lifecycle and execution context
   - Provides isolation between different script executions

2. **Execution Contexts**
   - Chat Session Context
   - Proxy Context
   - Tool Context

### Integration Points

#### 1. Chat Session Integration
```go
func NewChatSession(chat *models.Chat, mode ChatMode, db *gorm.DB, svc *services.Service, withFilters []*models.Filter, userID *uint, sessionID *string) (*ChatSession, error)
```
- Filters are initialized at chat session creation
- Scripts are executed during message processing
- Filters can modify chat behavior and message content

#### 2. Proxy Integration
```go
func (p *Proxy) executeFilters(filters []*models.Filter, ctx context.Context) error
```
- Filters are executed in the proxy layer
- Can modify request/response behavior
- Supports request validation and transformation

## User Interface

### Admin Frontend
Located in `ui/admin-frontend/src/admin/pages/FilterList.js`:

1. **Filter Management**
   - List view of all filters
   - Create/Edit/Delete operations
   - Filter script editor
   - Filter association management

2. **Script Editor Features**
   - Syntax highlighting
   - Script validation
   - Template support
   - Error handling

## Usage Contexts

### 1. Tool Enhancement
- Filters can be attached to tools to modify their behavior
- Multiple filters can be chained together on a single tool
- Filters can be added or removed dynamically

### 2. Chat Processing
- Filters can be applied to chats to modify message processing
- Chat creation supports filter configuration through filterIDs
- Filters affect RAG (Retrieval-Augmented Generation) behavior

## Implementation Details

### 1. Database Schema
- Uses GORM for database operations
- Many-to-many relationships handled through junction tables:
  - `tool_filters` for Tool-Filter associations
  - Implicit filter associations in Chat model

### 2. Script Execution
- Filters store their implementation logic in the Script field
- Scripts are stored as byte arrays for flexibility in implementation
- Script execution context depends on the integration point (Tool vs Chat)

## Security Considerations

1. **Access Control**
   - Filter operations should be restricted to authorized users
   - Filter scripts should be validated before execution
   - Filter associations should respect tool and chat access permissions

2. **Script Safety**
   - Script content should be validated before storage
   - Execution environment should be properly sandboxed
   - Resource limits should be enforced during script execution

## Integration Points

1. **LLM Service**
   - Filters can be applied to LLM configurations
   - Supports vendor-specific filter implementations

2. **Tool Service**
   - Filters modify tool behavior and capabilities
   - Supports dynamic filter chain configuration

3. **Chat Service**
   - Filters process chat messages and responses
   - Affects RAG behavior and tool interactions

## Testing

### 1. Unit Tests
- `services/filter_test.go`: Service layer tests
- `api/filter_handlers_test.go`: API endpoint tests
- `scripting/scripting_test.go`: Script execution tests

### 2. Integration Tests
- Filter execution in chat sessions
- Filter execution in proxy layer
- Tool-filter interaction tests

## Error Handling

### 1. Script Execution Errors
```go
cs.errors <- fmt.Errorf("error creating script runner")
```
- Script compilation errors
- Runtime errors
- Resource limit violations

### 2. Service Layer Errors
- Database operation errors
- Validation errors
- Permission errors

## Performance Considerations

### 1. Script Execution
- Script execution is isolated per context
- Resource limits are enforced
- Caching of compiled scripts when possible

### 2. Database Operations
- Efficient filter retrieval for associated entities
- Pagination support for large filter lists
- Optimized many-to-many relationships

## Implementation Status

### Completed Features

1. **Core Filter Implementation**
   - `models/filters.go` - Core filter model and database operations
   - `services/filter_service.go` - Filter service implementation
   - `services/filter_test.go` - Filter service tests

2. **API Layer**
   - `api/filter_handlers.go` - REST API handlers for filters
   - `api/filter_handlers_test.go` - API handler tests

3. **Scripting Engine**
   - `scripting/scripting.go` - Core scripting engine implementation
   - `scripting/scripting_test.go` - Scripting engine tests
   - `scriptExtensions/script_extensions.go` - Script extensions and utilities

4. **Documentation**
   - `docs/site/content/docs/filters.md` - Filter documentation
   - `features/Filters.md` - Filter feature specification

5. **UI Components**
   - `ui/admin-frontend/src/admin/components/filters/FilterDetails.js` - Filter detail view
   - `ui/admin-frontend/src/admin/components/filters/FilterForm.js` - Filter creation/editing form
   - `ui/admin-frontend/src/admin/pages/FilterList.js` - Filter list page

6. **Integration Files**
   - `services/tool_service.go` - Tool-Filter integration
   - `services/chat_service.go` - Chat-Filter integration
   - `proxy/proxy.go` - Proxy layer filter integration
   - `chat_session/chat_session.go` - Chat session filter integration

7. **Model Integration**
   - `models/tool.go` - Tool model with filter associations
   - `models/chat.go` - Chat model with filter associations

8. **Core Data Model**
   ```go
   type Filter struct {
       gorm.Model
       ID          uint   `json:"id" gorm:"primaryKey"`
       Name        string `json:"name"`
       Description string `json:"description"`
       Script      []byte `json:"script"`
   }
   ```
   - Basic CRUD operations implemented
   - Database operations with proper error handling

2. **API Layer** (`api/filter_handlers.go`)
   - RESTful endpoints following JSON:API specification
   - Complete CRUD operations:
     - POST /filters
     - GET /filters/{id}
     - PATCH /filters/{id}
     - DELETE /filters/{id}
     - GET /filters
   - Pagination support with X-Total-Count and X-Total-Pages headers
   - Proper error handling and validation

3. **Tool Integration**
   - Database Schema (`models/tool.go`):
     ```go
     type Tool struct {
         // ...
         Filters []Filter `gorm:"many2many:tool_filters;" json:"filters"`
         // ...
     }
     ```
   - Service Methods (`services/tool_service.go`):
     ```go
     AddFilterToTool(toolID uint, filterID uint) error
     RemoveFilterFromTool(toolID uint, filterID uint) error
     GetToolFilters(toolID uint) ([]Filter, error)
     SetToolFilters(toolID uint, filterIDs []uint) error
     ```

4. **Frontend Implementation**
   - List View (`ui/admin-frontend/src/admin/pages/FilterList.js`):
     - Paginated list of filters
     - Sorting capabilities
     - Basic CRUD operations
   - Filter Form (`ui/admin-frontend/src/admin/components/filters/FilterForm.js`):
     - Create/Edit functionality
     - Basic validation
     - Base64 encoding/decoding of scripts
   - Filter Details (`ui/admin-frontend/src/admin/components/filters/FilterDetails.js`):
     - Detailed view of filter properties
     - Script display in monospace font

5. **Script Execution** (`scripting/scripting.go`)
   - Tengo script execution engine
   - Concurrent execution support via mutex
   - Basic error handling
   - Support for custom modules

### Pending Implementation

1. **Frontend Enhancements**
   - Script Editor (`ui/admin-frontend/src/admin/components/filters/FilterEditor.js`) [TODO]:
     ```jsx
     const FilterEditor = () => {
       // Monaco Editor integration
       // Syntax highlighting for Tengo
       // Real-time validation
       // Template support
     }
     ```
   - Script Testing Interface (`ui/admin-frontend/src/admin/components/filters/FilterTester.js`) [TODO]:
     ```jsx
     const FilterTester = () => {
       // Test payload input
       // Execution results display
       // Performance metrics
     }
     ```

2. **Script Execution Enhancements** (`scripting/scripting.go`) [TODO]:
   ```go
   type ScriptRunner struct {
       mu            sync.Mutex
       source        []byte
       compiledCache *tengo.Compiled
       resourceLimits ResourceLimits
   }

   type ResourceLimits struct {
       MaxExecutionTime time.Duration
       MaxMemoryUsage   int64
       MaxOperations    int64
   }
   ```

3. **Chat Integration** [TODO]:
   - Chat Session Filter (`chat_session/filter_executor.go`):
     ```go
     type FilterExecutor interface {
         ExecuteFilters(ctx context.Context, msg *models.Message) error
         ModifyRAGBehavior(ctx context.Context) error
     }
     ```
   - Message Processing (`chat_session/message_processor.go`):
     ```go
     type MessageProcessor struct {
         filters []models.Filter
         executor FilterExecutor
     }
     ```

4. **Monitoring System** (`analytics/filter_analytics.go`) [TODO]:
   ```go
   type FilterAnalytics struct {
       ExecutionMetrics map[uint]*Metrics
       UsageStats       map[uint]*Usage
   }

   type Metrics struct {
       ExecutionTime    []time.Duration
       MemoryUsage     []int64
       SuccessRate     float64
       LastExecuted    time.Time
   }
   ```

5. **Version Control** (`models/filter_version.go`) [TODO]:
   ```go
   type FilterVersion struct {
       gorm.Model
       FilterID    uint
       Version     int
       Script      []byte
       ChangedBy   string
       ChangeLog   string
       DeployedAt  *time.Time
   }
   ```

6. **Testing Infrastructure** [TODO]:
   - Integration Tests (`api/filter_handlers_test.go`):
     ```go
     func TestFilterToolIntegration(t *testing.T)
     func TestFilterChatIntegration(t *testing.T)
     func TestFilterPerformance(t *testing.T)
     ```
   - Security Tests (`scripting/security_test.go`):
     ```go
     func TestScriptSandboxing(t *testing.T)
     func TestResourceLimits(t *testing.T)
     ```
   - Edge Cases (`models/filters_test.go`):
     ```go
     func TestFilterEdgeCases(t *testing.T)
     func TestConcurrentAccess(t *testing.T)
     ```

7. **Documentation** [TODO]:
   - API Documentation (`docs/site/content/docs/filters.md`)
   - Script Writing Guide (`docs/site/content/docs/filter-scripts.md`)
   - Integration Guide (`docs/site/content/docs/filter-integration.md`)
   - Best Practices (`docs/site/content/docs/filter-best-practices.md`)

### Required Changes

1. **Database Migrations**:
   ```sql
   -- Add version control support
   CREATE TABLE filter_versions (
       id SERIAL PRIMARY KEY,
       filter_id INTEGER REFERENCES filters(id),
       version INTEGER,
       script BYTEA,
       changed_by VARCHAR(255),
       change_log TEXT,
       deployed_at TIMESTAMP
   );

   -- Add analytics support
   CREATE TABLE filter_metrics (
       id SERIAL PRIMARY KEY,
       filter_id INTEGER REFERENCES filters(id),
       execution_time INTEGER,
       memory_usage INTEGER,
       success BOOLEAN,
       executed_at TIMESTAMP
   );
   ```

2. **Configuration Updates** (`config/config.go`):
   ```go
   type FilterConfig struct {
       MaxExecutionTime    time.Duration `env:"FILTER_MAX_EXECUTION_TIME" default:"5s"`
       MaxMemoryUsage     int64         `env:"FILTER_MAX_MEMORY_MB" default:"100"`
       EnableVersioning   bool          `env:"FILTER_ENABLE_VERSIONING" default:"true"`
       EnableAnalytics    bool          `env:"FILTER_ENABLE_ANALYTICS" default:"true"`
   }
   ```


## Codebase Structure

This section provides a comprehensive overview of all files involved in the Filter system, organized by their purpose and responsibility.

### Core Models and Types

1. **Models**
   - `models/filters.go` - Core Filter struct and CRUD operations
   - `models/tool.go` - Tool-filter associations and management
   - `models/chat.go` - Chat-filter integration
   - `models/user_message.go` - Message filtering support

2. **Scripting Engine**
   - `scripting/scripting.go` - Core script execution engine
   - `scripting/scripting_test.go` - Script execution testing
   - `scriptExtensions/script_extensions.go` - Custom script extensions

### Services Layer

1. **Filter Services**
   - `services/filter_service.go` - Main filter business logic
   - `services/filter_test.go` - Service layer testing

2. **Integration Services**
   - `services/tool_service.go` - Tool-filter integration
   - `services/chat_service.go` - Chat session filter integration
   - `services/notification_service.go` - Filter event notifications

### API Layer

1. **Handlers**
   - `api/filter_handlers.go` - Filter management endpoints
   - `api/filter_handlers_test.go` - API endpoint testing

2. **Integration Handlers**
   - `api/tool_handlers.go` - Tool-filter operations
   - `api/chat_handlers.go` - Chat filter operations
   - `api/chat_session_handler.go` - Chat session filter management

### Frontend Components

1. **Filter Management**
   - `ui/admin-frontend/src/admin/pages/FilterList.js` - Filter listing page
   - `ui/admin-frontend/src/admin/components/filters/FilterForm.js` - Filter creation/editing
   - `ui/admin-frontend/src/admin/components/filters/FilterDetails.js` - Filter details view

### Integration Points

1. **Chat System**
   - `chat_session/chat_session.go` - Chat session filter execution
   - `chat_session/gorm_history.go` - Filter history tracking

2. **Proxy Layer**
   - `proxy/proxy.go` - Request/response filtering
   - `proxy/analyze_utils.go` - Filter analysis utilities

### Documentation

1. **Feature Specifications**
   - `features/Filters.md` - Main filter specification
   - `docs/site/content/docs/filters.md` - Filter documentation

2. **API Documentation**
   - `docs/swagger/docs.go` - API documentation
   - `docs/site/content/docs/filter-scripts.md` - Script writing guide
   - `docs/site/content/docs/filter-integration.md` - Integration guide
   - `docs/site/content/docs/filter-best-practices.md` - Best practices

### Testing Infrastructure

1. **Unit Tests**
   - `models/filters_test.go` - Model testing
   - `services/filter_test.go` - Service testing
   - `api/filter_handlers_test.go` - API testing

2. **Integration Tests**
   - `scripting/scripting_test.go` - Script execution testing
   - `chat_session/chat_session_test.go` - Chat integration testing
   - `proxy/proxy_test.go` - Proxy integration testing

### File Responsibilities

Each file in the codebase serves a specific purpose in the filter ecosystem:

1. **Core Functionality:**
   - Model files define data structures and relationships
   - Service files implement business logic
   - Handler files expose external API endpoints

2. **Integration:**
   - Chat session files manage interactive filtering
   - Proxy files handle request/response filtering
   - Tool service manages filter associations

3. **Frontend:**
   - List component provides filter management
   - Form component handles filter creation/editing
   - Details component shows filter information

4. **Documentation:**
   - Feature specs provide technical details
   - API docs guide implementation
   - Best practices guide development

5. **Testing:**
   - Unit tests verify core functionality
   - Integration tests ensure system cohesion
   - Performance tests validate efficiency

This structure ensures:
- Clear separation of concerns
- Comprehensive testing coverage
- Complete documentation
- Maintainable codebase
- Scalable architecture

## Future Considerations

1. **Filter Categories**
   - Implement filter categorization for better organization
   - Support filter tagging for improved searchability

2. **Filter Templates**
   - Provide pre-built filter templates for common use cases
   - Support filter composition from existing filters

3. **Advanced Analytics**
   - Machine learning for filter optimization
   - Usage pattern analysis
   - Performance prediction

4. **Collaboration Features**
   - Filter sharing between teams
   - Collaborative editing
   - Access control granularity
