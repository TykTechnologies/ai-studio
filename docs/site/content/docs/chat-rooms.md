---
title: "Chat Rooms"
weight: 70
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Chat Rooms

The **Chat Rooms View** enables administrators to manage interactive environments where non-technical users can engage with Large Language Models (LLMs). These chat rooms provide access to LLMs, data sources, and tools in a secure, group-based context. They are highly customizable to meet specific organizational needs.

---

#### **Chat Rooms List Overview**

1. **Columns**:
   - **Name**:
     - The name of the chat room (e.g., `RevOps`, `Tyk Dashboard`).
   - **LLM**:
     - The Large Language Model configured for the chat room (e.g., `Anthropic`, `OpenAI GPT-4`).
   - **LLM Settings**:
     - The specific configuration or model variant for the LLM (e.g., `claude-3-5-sonnet-20240620`, `gpt-4`).
   - **Groups**:
     - The user groups that have access to the chat room (e.g., `Default`, `Tyk Dashboard Customers`).
   - **Actions**:
     - A dropdown menu (three-dot icon) to perform actions such as editing or deleting the chat room.

---

#### **Features**

1. **Add Chat Room Button**:
   - A green button labeled **+ ADD CHAT ROOM**, located at the top-right.
   - Opens a form to create a new chat room with custom configurations.

2. **Pagination Control**:
   - Located at the bottom-left, this dropdown lets administrators adjust the number of chat rooms displayed per page.

---

#### **Purpose of Chat Rooms**

1. **User Interaction**:
   - Chat rooms serve as a user-friendly interface for non-technical users to interact with LLMs without requiring API integrations or technical expertise.

2. **Access Control**:
   - Chat rooms are secured with group-based permissions, ensuring that only authorized users can access specific LLMs, data sources, and tools.

3. **Integration of Resources**:
   - Chat rooms can incorporate:
     - **LLMs**: Specific models for natural language understanding and generation.
     - **Data Sources**: Vector databases or document repositories for enhanced interactions.
     - **Tools**: APIs and utilities for retrieving or manipulating external data.

---

#### **Customization Options**

1. **LLM Selection**:
   - Choose an LLM provider and specific model for each chat room.

2. **Group-Based Security**:
   - Assign user groups to ensure access is restricted to appropriate personnel.

3. **Integrated Tool and Data Access**:
   - Add tools and data sources to enhance the functionality of the chat room.

4. **Governance Settings**:
   - Enable filters or middleware for data sanitization and compliance with organizational policies.

---

#### **Use Cases**

1. **Support and Assistance**:
   - A chat room like `RevOps` can provide automated responses and insights for revenue operations teams.

2. **Customer Engagement**:
   - Chat rooms configured for external groups, such as `Tyk Dashboard Customers`, can deliver personalized assistance.

3. **Internal Collaboration**:
   - Chat rooms like `Wadsworth` can be used by internal teams to query data and tools securely.

---

#### **Benefits**

1. **Simplified Access**:
   - Non-technical users can access advanced AI capabilities through an intuitive chat interface.

2. **Controlled Environment**:
   - Group-based permissions and customizable settings ensure robust security and governance.

3. **Enhanced Productivity**:
   - By integrating LLMs, data sources, and tools, chat rooms enable streamlined workflows and decision-making.

---

The **Chat Rooms View** empowers administrators to create secure, accessible, and resource-rich environments for LLM interactions, catering to both internal and external use cases while maintaining strict control over data and tool usage.

### Chat Room Configuration

The **Chat Room Configuration View** allows administrators to define or modify the behavior, resources, and access controls for a specific chat room. This level of customization ensures that chat rooms are tailored to user needs and organizational requirements.

---

#### **Key Fields and Options**

### **1. Basic Information**
- **Name** *(Required)*:
  - The name of the chat room (e.g., `Wadsworth`).

- **LLM Settings** *(Dropdown)*:
  - Select the specific configuration of the Large Language Model to use (e.g., `claude-3-5-sonnet-20240620`).

- **LLM** *(Dropdown)*:
  - Choose the LLM vendor (e.g., `Anthropic`, `OpenAI`).

- **Groups** *(Dropdown)*:
  - Assign user groups to control access to the chat room.
  - Example: `Default`, `Commercial`.

---

### **2. Filters and System Prompts**
- **Filters** *(Optional)*:
  - Add governance filters to ensure compliance (e.g., PII detection, anonymization filters).

- **System Prompt** *(Optional)*:
  - Define a specialized prompt that guides the LLM’s behavior.
  - Example: "You are a highly professional assistant for engineering teams. Always provide concise and technical answers."

---

### **3. Tool and Data Source Configuration**
- **Enable Tool Support** *(Toggle)*:
  - Allows the LLM to use tools integrated into the chat room (e.g., web scrapers, Jira search).
  - **Enabled**: Tools are accessible for enhanced interactions.
  - **Disabled**: Tools are restricted, limiting functionality for better control.

- **Default Data Source** *(Dropdown)*:
  - Preselect a vector data source that will always be included in the chat room's context.
  - Example: `Tyk Documentation`.

- **Default Tools** *(Dropdown)*:
  - Assign specific tools to the chat room for automatic access.
  - Example: Jira search tools, web scraping utilities.

---

### **4. Extra Context**
- **Upload Context File**:
  - Add supplementary files to provide the LLM with more context for interactions.
  - Example: Instructional guides, predefined FAQs.

---

### **5. Actions**
- **Update Chat Room Button**:
  - Saves all changes made to the chat room configuration.

- **Back to Chat Rooms Button**:
  - Returns to the **Chat Rooms List View** without saving changes.

---

#### **Use Cases for Configuration**

1. **Specialized Use**:
   - Configure a chat room with a specific system prompt and default tools for a unique team or project.

2. **Governance**:
   - Apply filters to ensure compliance with organizational or regulatory requirements.

3. **Controlled Flexibility**:
   - Enable or disable tool support based on the maturity and technical expertise of the intended users.

4. **Enhanced User Experience**:
   - Preload essential data sources or tools to streamline user interactions with the LLM.

---

#### **Benefits of Customization**
- **Tailored Interactions**:
  - Define the behavior and resources available in the chat room to suit different teams or workflows.

- **Improved Governance**:
  - Add filters to enforce data security and compliance policies.

- **Optimized Performance**:
  - Preload frequently used resources to reduce latency and improve response accuracy.

- **User-Specific Adaptation**:
  - Adjust tool and data source access based on user needs, ensuring a better experience for both technical and non-technical users.

---

This configuration interface empowers administrators to create highly specialized, secure, and efficient chat rooms that meet the dynamic needs of users and organizations alike.
