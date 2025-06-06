## Apps

**1. Overview & Purpose**

The Apps feature in Midsommar provides a way to create, configure, and manage AI applications that leverage LLMs, data sources, and tools. Apps serve as containers for AI capabilities, allowing users to build custom solutions for specific use cases.

**Core Objectives:**

* **Resource Integration:** Enable applications to subscribe to and utilize LLMs, data sources, and tools.
* **Access Control:** Manage which resources an app can access based on user permissions and privacy settings.
* **Budget Management:** Control and monitor spending on AI resources at the application level.
* **Flexibility:** Support a wide range of AI use cases through customizable configurations.
* **User Experience:** Provide a simple interface for creating and managing AI applications.

**User Roles & Interactions:**

* **Administrator:** Creates and manages apps for the organization, assigns apps to users, and configures global settings.
* **App Developer:** Creates and configures apps with specific LLMs, data sources, and tools.
* **End User:** Uses apps to interact with AI capabilities for specific tasks or workflows.

**2. Implementation Details**

* **App Model Structure:**
  ```go
  type App struct {
      ID              uint       `json:"id" gorm:"primary_key"`
      Name            string     `json:"name"`
      Description     string     `json:"description"`
      UserID          uint       `json:"user_id"`
      User            User       `json:"-" gorm:"foreignkey:UserID"`
      CredentialID    uint       `json:"credential_id"`
      Credential      Credential `json:"-" gorm:"foreignkey:CredentialID"`
      LLMs            []LLM      `gorm:"many2many:app_llms;" json:"llms"`
      Datasources     []Datasource `gorm:"many2many:app_datasources;" json:"datasources"`
      Tools           []Tool     `gorm:"many2many:app_tools;" json:"tools"` // New relationship with Tools
      MonthlyBudget   *float64   `json:"monthly_budget"`
      BudgetStartDate *time.Time `json:"budget_start_date"`
  }
  ```

* **Resource Subscription Management:**
  * **LLM Subscription:** Apps can subscribe to multiple LLMs through the `app_llms` join table.
  * **Datasource Subscription:** Apps can subscribe to multiple datasources through the `app_datasources` join table.
  * **Tool Subscription:** Apps can now subscribe to multiple tools through the `app_tools` join table.
  * Each subscription type has dedicated methods for adding, removing, and retrieving associated resources.

* **Privacy Score Validation:**
  * Apps enforce privacy compatibility between datasources and LLMs.
  * Datasources with high privacy scores can only be used with LLMs that have appropriate privacy capabilities.
  * This validation occurs during app creation and updates.

* **Budget Management:**
  * Apps can have optional monthly budgets to control spending.
  * Budget start dates determine when the budget cycle begins.
  * Usage is tracked and monitored against the budget.

* **App-Tool Integration:**
  * The new `app_tools` join table establishes a many-to-many relationship between apps and tools.
  * Apps can subscribe to multiple tools, and tools can be used by multiple apps.
  * Tool access is controlled by permissions and privacy settings.
  * Similar to LLM and datasource subscriptions, tool subscriptions enhance app capabilities.

**3. API Endpoints**

* **App Management:**
  * `POST /apps`: Create a new app.
  * `GET /apps/{id}`: Get an app by ID.
  * `PUT /apps/{id}`: Update an app.
  * `DELETE /apps/{id}`: Delete an app.
  * `GET /apps`: List all apps (filtered by user).

* **Resource Subscription Management:**
  * `GET /apps/{app_id}/llms`: Get all LLMs associated with an app.
  * `GET /apps/{app_id}/datasources`: Get all datasources associated with an app.
  * `GET /apps/{app_id}/tools`: Get all tools associated with an app.
  * `POST /apps/{app_id}/tools/{tool_id}`: Associate a tool with an app.
  * `DELETE /apps/{app_id}/tools/{tool_id}`: Disassociate a tool from an app.

**4. UI Components**

* **App List Page:**
  * Displays all apps with search, filtering, and pagination.
  * Shows app name, description, owner, and associated resources.
  * Provides actions for editing, viewing details, and deleting apps.

* **App Form:**
  * Creates and edits apps with comprehensive configuration options.
  * Includes fields for name, description, user, credential, and budget settings.
  * Provides multi-select dropdowns for LLMs, datasources, and tools.
  * Validates resource access permissions and privacy score compatibility.

* **App Details Page:**
  * Displays detailed information about a specific app.
  * Shows all associated resources including LLMs, datasources, and tools.
  * Provides options to add or remove resource associations.
  * Displays usage statistics and budget information.

**5. Use Cases**

* **Chatbots:** Create conversational AI applications with specific LLMs and knowledge bases.
* **Document Analysis:** Build apps that analyze and extract information from documents using specialized tools.
* **Data Exploration:** Develop apps that help users explore and understand their data through natural language queries.
* **Content Generation:** Create apps for generating various types of content using specific LLMs and tools.
* **Research Assistant:** Build apps that help with research by combining search tools with LLM capabilities.
* **Custom Workflows:** Develop specialized apps for specific business processes or workflows.

**6. Future Enhancements**

* **App Templates:** Pre-configured app templates for common use cases.
* **App Sharing:** Ability to share apps between users or organizations.
* **Advanced Permissions:** More granular control over who can use specific app features.
* **App Versioning:** Support for versioning apps to track changes over time.
* **App Marketplace:** A marketplace for discovering and sharing apps.
* **Enhanced Analytics:** More detailed analytics on app usage and performance.
* **App-Specific Tool Configurations:** Allow apps to have custom configurations for subscribed tools.
