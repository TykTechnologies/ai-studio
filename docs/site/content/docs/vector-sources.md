---
title: "Vector Sources"
weight: 25
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Vector Data Sources View for Tyk AI Portal

The **Vector Data Sources View** enables administrators to manage and configure vector-based data sources. These data sources can be queried via the API or included in Chat Rooms as selectable or default resources to shape the behavior of interactions. Below is a breakdown of the features and options in this section:

---

#### **Table Overview**

1. **Name**:
   - The name of the vector data source (e.g., `Tyk Documentation`, `Tyk Stack Overflow`).

2. **Short Description**:
   - A brief summary describing the data source and its purpose (e.g., "Scraped documentation from the tyk.io website").

3. **DB Source Type**:
   - Specifies the database system used for the vector data source (e.g., `Pinecone`).

4. **Embed Vendor**:
   - Indicates the embedding vendor responsible for processing data into vectorized formats (e.g., `OpenAI`).

5. **Privacy Score**:
   - A numerical value representing the privacy level of the data source:
     - **Higher scores** indicate better privacy handling.
     - **Lower scores** suggest less stringent privacy measures.

6. **Tags**:
   - Keywords or labels associated with the data source, making it easier to categorize or identify (e.g., `tyk`, `docs`).

7. **Active**:
   - A status indicator showing whether the data source is currently active:
     - **Green dot**: Active and available for use.
     - **Red dot**: Inactive and not available for querying or inclusion in Chat Rooms.

8. **Actions**:
   - A menu (three-dot icon) that provides options to:
     - Edit the data source.
     - Delete the data source.

---

#### **Features**

1. **Add Data Source Button**:
   - A green button labeled **+ ADD DATASOURCE**, located in the top-right corner. Clicking this opens a form to create and configure a new vector data source.

2. **Pagination Dropdown**:
   - Found at the bottom-left corner, this control allows administrators to adjust how many data sources are displayed per page.

---

#### **Use Cases**

1. **API Queries**:
   - Vector data sources configured here can be queried via the API, enabling programmatic access to indexed data for tasks like content search or retrieval.

2. **Chat Rooms**:
   - Data sources can be:
     - **User-selectable**: Allowing end-users to choose the data source for specific queries.
     - **Default**: Pre-included in the Chat Room to guide its behavior or provide predefined context.

---

#### **Quick Insights**
The **Vector Data Sources View** simplifies the integration and management of vector databases and embedding systems, ensuring that administrators can tailor data availability and privacy levels. This section is crucial for enabling seamless API and Chat Room interactions with structured, searchable datasets.

### Edit/Create Data Source

The **Edit/Create Data Source Form** is used to configure or update vector data sources for querying and embedding purposes. It provides options for setting access details, embedding configurations, and initializing the data source with files via a quick upload feature.

---

#### **Form Sections and Fields**

### **Basic Information**
1. **Name** *(Required)*:
   - The name of the data source (e.g., `Tyk Documentation`).
   - This is how the data source will appear in the portal and API calls.

2. **Short Description** *(Optional)*:
   - A concise description of the data source and its purpose.
     Example: "Scraped documentation from the tyk.io website and other sources."

3. **User** *(Dropdown)*:
   - Assigns the owner of the data source to a specific user.

4. **Vector Database Type** *(Dropdown)*:
   - Specifies the type of vector database used (e.g., `Pinecone`).

5. **Embedding Service Vendor** *(Dropdown)*:
   - The vendor used for embedding (e.g., `OpenAI`).

6. **Privacy Score** *(Integer)*:
   - A numerical value to indicate the privacy level of the data source.
     - **Higher scores**: Better privacy protections.
     - **Lower scores**: Less stringent privacy controls.

7. **Active** *(Toggle)*:
   - Determines whether the data source is enabled and available for use.
     - **Enabled**: Active and usable in Chat Rooms or API queries.
     - **Disabled**: Not available for interaction.

---

### **Vector Database Access Details**
1. **Database / Namespace Name** *(Required)*:
   - Specifies the unique identifier or namespace for the data source within the vector database.

2. **Connection String** *(Required)*:
   - URL or endpoint for accessing the vector database.
     Example: `https://example-vector-db.pinecone.io`.

3. **API Key** *(Required)*:
   - Authentication key for accessing the vector database.
     - **View/Hide Toggle**: Allows the key to be shown or hidden for security.

---

### **Embedding Service Details**
1. **Model** *(Optional)*:
   - Specifies the embedding model used for processing data (e.g., `sentence-transformers/all-mpnet-base-v2`).

2. **Service URL** *(Optional)*:
   - The endpoint of the embedding service (e.g., `http://embedding-service.com/api/v1`).

3. **API Key** *(Optional)*:
   - Authentication key for accessing the embedding service.
     - **View/Hide Toggle**: Ensures security when displaying sensitive information.

---

### **Additional Information**
1. **Long Description** *(Optional)*:
   - A detailed explanation of the data source and its contents.

2. **Icon URL** *(Optional)*:
   - A link to an image that visually represents the data source.

3. **Tags** *(Optional)*:
   - Labels for categorizing or describing the data source (e.g., `tyk`, `docs`).
     - **Add Tag Button**: Adds tags for better organization and searchability.

---

### **File Quick Upload**
1. **Upload Additional Datasource Documentation**:
   - Allows administrators to upload files to seed the data source.
     - The files are processed, chunked, and embedded into the vector database.

2. **Start Processing** *(Button)*:
   - Initiates the processing of uploaded files for indexing into the vector database.

---

### **Action Buttons**
1. **Update Datasource / Create Datasource**:
   - Saves the data source configuration or creates a new one based on the current inputs.

2. **Back to Datasources**:
   - Navigates back to the **Vector Data Sources View** without saving changes.

---

#### **Purpose**
This form provides a centralized interface for managing vector data sources, including access details, embedding configurations, and initialization from files. It supports seamless integration of structured datasets into the Tyk AI ecosystem for use in Chat Rooms or API queries.
