---
title: "Tools"
weight: 30
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Tools List

The **Tools List View** provides administrators with an overview of available tools that can be assigned to user groups and used in Chat Rooms. Tools enhance interaction by allowing users to add functionality to their context, enabling the Large Language Model (LLM) to perform specific tasks. Below is a breakdown of the features and information in this section:

---

#### **Table Overview**

1. **Name**:
   - The name of the tool (e.g., `JIRA Search`, `Hubspot API`).

2. **Description**:
   - A brief explanation of the tool's purpose and functionality (e.g., "Enables search and retrieval access to Tyk's JIRA system").

3. **Privacy Score**:
   - A numerical value indicating the tool's privacy level:
     - **Higher scores**: More stringent privacy measures.
     - **Lower scores**: Lesser privacy protections.
   - Tools with specific privacy scores can only be used with LLM vendors that are compatible with those scores.

4. **Actions**:
   - A menu (three-dot icon) that allows administrators to:
     - Edit the tool configuration.
     - Delete the tool from the portal.

---

#### **Features**

1. **Add Tool Button**:
   - A green button labeled **+ ADD TOOL** located in the top-right corner. Clicking this button opens a form to create a new tool and configure its details.

2. **Pagination Dropdown**:
   - Located at the bottom-left corner, this control allows administrators to adjust the number of tools displayed per page.

---

#### **Use Cases**

1. **Group Assignment**:
   - Tools can be assigned to specific user groups, ensuring access is limited to authorized users.

2. **Chat Room Integration**:
   - Tools assigned to groups appear as options in Chat Rooms. Users can add these tools to their interaction context, enabling the LLM to perform actions like API calls, searches, or data retrieval.

3. **Vendor Compatibility**:
   - Each tool's privacy score ensures it can only be used with LLM vendors that meet or exceed the tool's privacy requirements.

---

#### **Examples of Tools**

- **JIRA Search**:
   - Allows users to query and analyze tickets in JIRA.

- **Hubspot CRM API**:
   - Provides access to Hubspot data for querying deal details.

- **Web Scraper**:
   - Enables users to scrape web pages provided by the user or other tools.
   - Privacy Score: 0.

- **Zendesk**:
   - Grants access to the support ticketing system
   - Privacy Score: 20.

---

#### **Purpose**
The **Tools List View** simplifies the management and configuration of tools available for use in Chat Rooms. By assigning tools to user groups and ensuring compatibility through privacy scores, administrators can enhance user interactions while maintaining security and compliance standards.

### Edit/Create Tools

The **Edit/Create Tool Form** allows administrators to define or modify tools that can be integrated into Chat Rooms or API interactions. Tools enhance the capabilities of the LLM by granting it access to external systems and services via OpenAPI specifications. Below is a detailed breakdown of the features and fields in this form:

---

#### **Form Sections and Fields**

### **Tool Information**
1. **Name** *(Required)*:
   - The name of the tool (e.g., `Tyk JIRA Search`).

2. **Description** *(Optional)*:
   - A brief summary explaining what the tool does and how it can be used (e.g., "Enables search and retrieval access to Tyk's JIRA system").

3. **Privacy Score** *(Integer)*:
   - A numerical value that defines the tool's privacy level. Tools with higher privacy scores can only be used with compatible LLM vendors.

---

### **OpenAPI Specification**
1. **OAS Spec** *(Required)*:
   - The OpenAPI Specification (OAS) JSON or YAML file that defines the tool's API. This specifies the endpoints, operations, and parameters the tool supports.

2. **Operations** *(Required)*:
   - A list of operation IDs from the OpenAPI spec that the LLM is allowed to access (e.g., `searchForIssuesById`, `getIssue`). At least one operation must be specified.

---

### **Dependencies**
- **Add Dependency** *(Optional)*:
   - Dependencies are other tools that must be available for this tool to function properly.
     Example: A Web Search tool may depend on a Web Scraper tool to handle specific tasks.

---

### **Middleware**
- **Add Filter** *(Optional)*:
   - Middleware allows administrators to apply preprocessing or postprocessing logic to the data returned by the tool before it is sent to the LLM.
     Example: Removing sensitive information or anonymizing responses.

---

### **Authentication Details**
1. **Auth Schema Name** *(Optional)*:
   - Specifies the authentication schema from the OpenAPI spec that the tool will use (e.g., `basicAuth`, `bearerToken`).

2. **Auth Key** *(Optional)*:
   - The authentication key required to access the tool’s API.
     - **View/Hide Toggle**: Ensures sensitive information is handled securely.

---

### **Extra Context**
- **Upload Additional Tool Documentation** *(Optional)*:
   - Allows administrators to upload files that provide extra instructions or context for the LLM on how to use the tool effectively.
     Example: User guides, sample queries, or detailed operation descriptions.

---

#### **Action Buttons**
1. **Update Tool / Create Tool**:
   - Saves the tool configuration or creates a new tool based on the provided details.

2. **Back to Tools**:
   - Navigates back to the **Tools List View** without saving changes.

---

### **Purpose and Use Cases**

1. **Tool Integration**:
   - Tools extend the LLM's capabilities by providing access to APIs and services, enabling tasks like data retrieval, searching, or automation.

2. **Enhanced Context**:
   - The extra context and dependencies ensure that the LLM operates effectively and understands the tool's usage.

3. **Privacy and Security**:
   - Privacy scores, middleware, and authentication ensure secure and compliant usage of tools within the portal.

This form is a comprehensive interface for managing tools, enabling fine-grained control over their functionality, security, and integration into Chat Rooms or API workflows.
