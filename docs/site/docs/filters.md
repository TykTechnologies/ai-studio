---
title: "Filters"
weight: 40
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Filters and Middleware

The **Filters List View** allows administrators to manage filters and middleware applied to prompts or data sent to Large Language Models (LLMs) via the AI Gateway or Chat Rooms. Filters and middleware ensure data governance, compliance, and security by processing or controlling the flow of information. Below is an enhanced description with the distinction between **Filters** and **Middleware**:

---

#### **Filters: Unified Blocking and Modification**

Filters in Midsommar provide comprehensive request/response processing with both **blocking** and **modification** capabilities:

1. **Blocking Filters**:
   - **Purpose**: Governance controls that deny requests based on content analysis.
   - **Behavior**:
     - Analyze message content, metadata, and context.
     - Block requests that violate policies or contain restricted content.
     - Example: Block prompts containing PII, sensitive keywords, or unauthorized patterns.

2. **Modification Filters**:
   - **Purpose**: Transform message content before it reaches the LLM or after tool responses.
   - **Behavior**:
     - Redact sensitive information (emails, phone numbers, SSNs).
     - Enhance system prompts with safety instructions.
     - Normalize or transform content across vendors.
     - Example: Automatically redact PII while allowing the request to proceed.

3. **Combined Approach**:
   - Filters can both inspect AND modify in a single script.
   - Example: Redact emails from user messages, but block if SSN is detected.

**Key Capabilities**:
- ✅ **Request Filters**: Modify/block requests before reaching LLM
  - LLM Proxy Requests (before reaching LLM)
  - Chat Session Messages (before RAG search and LLM)
  - File Content (before RAG indexing)
  - Tool Responses (after tool execution)
- ✅ **Response Filters**: Block LLM responses based on content
  - LLM Proxy Responses (REST and streaming)
  - Chat Session Responses (regular and streaming)

---

#### **Table Overview**

1. **Name**:
   - The name of the filter or middleware (e.g., `Anonymize PII (LLM)`, `Fixed PII Filter`).

2. **Description**:
   - A brief summary of the filter or middleware's functionality (e.g., "Uses Regex to remove obvious PII").

3. **Actions**:
   - A menu (three-dot icon) that allows administrators to:
     - Edit the filter or middleware.
     - Delete the filter or middleware.

---

#### **Features**

1. **Add Filter Button**:
   - A green button labeled **+ ADD FILTER**, located in the top-right corner. Clicking this button opens a form to create a new filter or middleware.

2. **Pagination Dropdown**:
   - Located at the bottom-left corner, this control allows administrators to adjust the number of entries displayed per page.

---

#### **Examples of Filters and Middleware**

- **Filters**:
  - **PII Detector**: A regex-based filter that blocks prompts containing sensitive PII.
  - **JIRA Field Analysis**: Ensures no PII is included in data retrieved from JIRA fields before passing to the LLM.

- **Middleware**:
  - **Anonymize PII (LLM)**: Uses an LLM to anonymize sensitive data before sending it downstream.
  - **NER Service Filter**: A Named Entity Recognition (NER) microservice that modifies outputs to remove identified entities.

---

#### **Use Cases**

1. **Prompt Validation with Filters**:
   - Ensures that only compliant and secure prompts are sent to LLMs.
   - Example: Blocking a prompt with sensitive data that should not be processed by an unapproved vendor.

2. **Data Preprocessing with Middleware**:
   - Prepares data from tools or external sources for safe interaction with LLMs by modifying or anonymizing content.
   - Example: Removing sensitive ticket details from a JIRA query before sending to an LLM.

3. **Organizational Security**:
   - Both filters and middleware ensure sensitive information is protected and handled in line with organizational governance policies.

4. **Enhanced Tool Interactions**:
   - Middleware supports tools by transforming their outputs, enabling richer and safer LLM interactions.

---

#### **Key Benefits**

1. **Improved Data Governance**:
   - Filters and middleware work together to enforce strict controls over data flow, protecting sensitive information.

2. **Flexibility**:
   - Middleware allows for data transformation, enhancing interoperability between tools and LLMs.
   - Filters ensure compliance without altering user-provided prompts.

3. **Compliance and Security**:
   - Prevent unauthorized or sensitive data from reaching unapproved vendors, ensuring regulatory compliance.

This detailed structure for **Filters and Middleware** provides organizations with robust governance tools to secure and optimize data workflows in the Tyk AI Studio.

### Filter Edit View (and example Filter)

The **Filter Edit View** enables administrators to create or modify filters using the **Tengo scripting language**. Filters serve as governance tools that analyze input data (e.g., prompts or files) and decide whether the content is permitted to pass to the upstream LLM. In this example, the filter uses regular expressions (regex) to detect Personally Identifiable Information (PII) and blocks the prompt if any matches are found.

---

#### **Form Sections and Fields**

1. **Name** *(Required)*:
   - Specifies the name of the filter (e.g., `PII Detector`).

2. **Description** *(Optional)*:
   - A brief summary of the filter's purpose and functionality (e.g., "Simple Regex-based PII detector to prevent the wrong data being sent to LLMs").

3. **Script** *(Required)*:
   - A **Tengo script** that defines the logic of the filter. The script evaluates input data and determines whether the filter approves or blocks it.
   - The example script detects PII using a collection of regex patterns and blocks the data if a match is found.

---

#### **New Unified Script API**

Modern filters use a unified API that provides rich context and supports both blocking and modification:

**Input Object:**
```javascript
input := {
    raw_input: "...",      // Full JSON request payload
    messages: [            // Normalized message array with roles
        {role: "system", content: "You are helpful"},
        {role: "user", content: "Hello"}
    ],
    vendor_name: "openai", // LLM vendor (openai, anthropic, google_ai, etc.)
    model_name: "gpt-4",   // Model being called
    is_chat: false,        // Context: chat session (true) or proxy (false)
    context: {             // Additional metadata
        llm_id: 123,
        app_id: 456,
        user_id: 789
    }
}
```

**Output Object:**
```javascript
output := {
    block: false,           // Set true to block the request
    payload: "",            // Modified JSON payload (or empty for no change)
    messages: [],           // Alternative: modified message array
    message: ""             // Optional reason/log message
}
```

---

#### **Example Script 1: Blocking Filter (PII Detection)**

This script blocks requests containing PII patterns:

```tengo
text := import("text")

// Check all user messages for email addresses
should_block := false
block_reason := ""

for msg in input.messages {
    if msg.role == "user" {
        if text.contains(msg.content, "@") {
            should_block = true
            block_reason = "Email addresses not allowed"
            break
        }
    }
}

output := {
    block: should_block,
    payload: input.raw_input,
    message: block_reason
}
```

---

#### **Example Script 2: Modification Filter (Email Redaction)**

This script redacts emails while allowing the request to proceed:

```tengo
tyk := import("tyk")

// Use helper to redact email addresses across all messages
modified_payload := tyk.redact_pattern(
    input,
    "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}",
    "[EMAIL_REDACTED]"
)

output := {
    block: false,
    payload: modified_payload,
    message: "Emails redacted"
}
```

---

#### **Example Script 3: Advanced Modification (Messages Array)**

This script shows complex message modification using the messages array approach:

```tengo
text := import("text")

// Modify messages based on role
modified := []

for msg in input.messages {
    new_msg := {
        role: msg.role,
        content: msg.content
    }

    // Add safety prefix to system prompts
    if msg.role == "system" {
        new_msg.content = "[SAFETY MODE] " + msg.content
    }

    // Redact emails from user messages
    if msg.role == "user" {
        new_msg.content = text.replace(msg.content, "@", "[AT]", -1)
    }

    modified = append(modified, new_msg)
}

output := {
    block: false,
    messages: modified,  // System handles vendor-specific JSON reconstruction
    message: "Content modified"
}
```

---

#### **Example Script 4: Combined Blocking + Modification**

This script redacts emails but blocks if SSN is detected:

```tengo
text := import("text")
tyk := import("tyk")

// First check for SSN (hard block)
ssn_pattern := "\\d{3}-\\d{2}-\\d{4}"
has_ssn := false

for msg in input.messages {
    if text.re_match(ssn_pattern, msg.content) {
        has_ssn = true
        break
    }
}

if has_ssn {
    output := {
        block: true,
        payload: "",
        message: "Blocked: SSN detected"
    }
} else {
    // No SSN - redact emails and continue
    modified_payload := tyk.redact_pattern(
        input,
        "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}",
        "[EMAIL]"
    )

    output := {
        block: false,
        payload: modified_payload,
        message: "Emails redacted"
    }
}
```

---

#### **Available Helper Functions**

The `midsommar` module provides helper functions for common message modification tasks:

1. **`redact_pattern(input, pattern, replacement)`**:
   - Redacts a regex pattern from all messages (system, user, assistant)
   - Parameters:
     - `input` - The input object provided to your script
     - `pattern` - Regular expression pattern (string)
     - `replacement` - Replacement string
   - Returns: Modified payload as string
   - Example: `tyk.redact_pattern(input, "\\d{3}-\\d{2}-\\d{4}", "[SSN]")`

**Vendor-Agnostic**: Helper functions automatically handle differences between OpenAI, Anthropic, Google AI, and other vendor formats.

---

#### **Message Modification Approaches**

**Approach 1: Helper Functions** (Simple, recommended for pattern-based redaction)
```tengo
tyk := import("tyk")
modified := tyk.redact_pattern(input, "@\\S+", "[EMAIL]")
output := {block: false, payload: modified, message: ""}
```

**Approach 2: Messages Array** (Flexible, recommended for complex logic)
```tengo
modified := []
for msg in input.messages {
    new_msg := {role: msg.role, content: msg.content}
    if msg.role == "user" {
        // Apply custom modification logic
        new_msg.content = transform(msg.content)
    }
    modified = append(modified, new_msg)
}
output := {block: false, messages: modified, message: ""}
```

---

#### **Accessing Message Context**

Scripts can access rich contextual information:

```tengo
// Access vendor and model information
if input.vendor_name == "anthropic" {
    // Anthropic-specific logic
}

// Count messages by role
user_count := 0
for msg in input.messages {
    if msg.role == "user" {
        user_count = user_count + 1
    }
}

// Check if this is a chat session or proxy request
if input.is_chat {
    // Chat-specific logic
}

// Access metadata
app_id := input.context.app_id
user_id := input.context.user_id
```

---

#### **Action Buttons**
1. **Update Filter / Create Filter**:
   - Saves the filter configuration, making it active for future data processing.

2. **Back to Filters**:
   - Returns to the Filters List View without saving changes.

---

#### **Purpose and Benefits**

1. **Data Governance**:
   - Enforces strict control over what data can be sent to LLMs, ensuring compliance with privacy regulations.

2. **Flexibility**:
   - Filters can be tailored to specific organizational needs using custom scripts.

3. **Security**:
   - Prevents sensitive or unauthorized data from leaking to unapproved vendors or external systems.

This **Filter Edit View** provides a robust and customizable interface for creating scripts to enforce data governance and security in the Tyk AI Studio.

### Example Middleware for Tools

Middleware filters in the Tyk AI Studio modify data coming from tools before passing it to the LLM. These filters are applied to sanitize, anonymize, or enhance the data to ensure it complies with organizational standards and privacy regulations. Below is an example of a middleware filter that sanitizes Personally Identifiable Information (PII), specifically email addresses, from the tool's output.

---

#### **Middleware Script: Email Redaction Example**

```tengo
// Import the 'text' module for regular expression operations
text := import("text")

// Define regular expression patterns for various PII
email_pattern := `[\w\.-]+@[\w\.-]+\.\w+`

// Define the function to sanitize PII in the input string
filter := func(input) {
    // Replace email addresses
    input = text.re_replace(email_pattern, input, "[REDACTED EMAIL]")

    return input
}

// Process the input payload
result := filter(payload)
```

---

#### **Explanation of the Script**

1. **Module Import**:
   - The `text` module is imported to enable regular expression operations (`text.re_replace`).

2. **Regex Pattern**:
   - A regex pattern is defined to detect email addresses:
     - Example pattern: `[\w\.-]+@[\w\.-]+\.\w+`
     - This pattern matches standard email formats.

3. **Filter Function**:
   - The `filter` function accepts an input string (e.g., tool output) and:
     - Uses `text.re_replace` to identify email addresses.
     - Replaces detected email addresses with `[REDACTED EMAIL]`.

4. **Return Processed Output**:
   - The sanitized output is returned, ensuring that sensitive information like email addresses is redacted before reaching the LLM.

---

#### **Use Case for Middleware**

**Tool Example**:
Imagine a tool, such as `Support Ticket Viewer`, which retrieves user tickets from a system. These tickets often contain email addresses. Middleware ensures that no sensitive email information is included in the output sent to the LLM.

- **Input Payload Example**:
   ```text
   User email: john.doe@example.com has reported an issue with their account.
   ```

- **Sanitized Output**:
   ```text
   User email: [REDACTED EMAIL] has reported an issue with their account.
   ```

---

#### **Benefits of Middleware**

1. **Data Privacy**:
   - Protects sensitive user information by ensuring it is sanitized before being sent to external systems.

2. **Compliance**:
   - Ensures organizational adherence to privacy laws like GDPR or HIPAA.

3. **Enhanced Security**:
   - Prevents accidental sharing of PII with external vendors or LLMs.

---

## Available Tengo Modules

Filters have access to powerful standard library modules:

### **text** - String Operations
```tengo
text := import("text")

// Common functions:
text.contains(str, substr)           // Check if substring exists
text.replace(str, old, new, n)       // Replace occurrences
text.to_upper(str)                   // Convert to uppercase
text.to_lower(str)                   // Convert to lowercase
text.split(str, sep)                 // Split string
text.trim_space(str)                 // Remove whitespace
text.re_match(pattern, str)          // Regex match
text.re_replace(pattern, str, repl)  // Regex replace
```

### **json** - JSON Operations
```tengo
json := import("json")

parsed := json.decode(json_string)   // Parse JSON
encoded := json.encode(object)       // Encode to JSON
```

### **fmt** - Formatting and Printing
```tengo
fmt := import("fmt")

fmt.println("Debug:", variable)      // Print for debugging
formatted := fmt.sprintf("Value: %v", val)  // Format strings
```

### **tyk** - Extended Capabilities (Enterprise)
```tengo
tyk := import("tyk")

// Redact regex patterns from all messages (vendor-agnostic)
modified := tyk.redact_pattern(input, pattern, replacement)
// Returns: Modified payload as string

// Make HTTP requests from within filters
result := tyk.makeHTTPRequest(method, url, headers, body)
// Returns: {status: 200, response: "..."}

// Call other LLMs for analysis/enrichment
response := tyk.llm(llm_id, settings_object, prompt)
// Parameters:
//   - llm_id: ID of the managed LLM to use
//   - settings_object: Map with model settings (model_name, temperature, max_tokens, etc.)
//   - prompt: The prompt text to send
// Returns: LLM response as string
```

**Example - Use LLM for PII Detection:**
```tengo
tyk := import("tyk")

// Get user message
user_msg := ""
for msg in input.messages {
    if msg.role == "user" {
        user_msg = msg.content
        break
    }
}

// Use another LLM to detect PII
// Define settings for the LLM call
settings := {
    model_name: "gpt-3.5-turbo",
    temperature: 0.0,
    max_tokens: 10,
    system_prompt: "You are a PII detection assistant. Answer only 'yes' or 'no'."
}

pii_check_prompt := "Does this text contain PII?: " + user_msg
pii_result := tyk.llm(1, settings, pii_check_prompt)

output := {
    block: pii_result == "yes",
    payload: input.raw_input,
    message: pii_result == "yes" ? "PII detected by LLM" : ""
}
```

**Example - Call External Service:**
```tengo
tyk := import("tyk")
json := import("json")

// Get user message
user_msg := ""
for msg in input.messages {
    if msg.role == "user" {
        user_msg = msg.content
    }
}

// Call external PII detection API
headers := {
    "Content-Type": "application/json",
    "Authorization": "Bearer YOUR_TOKEN"
}
body := json.encode({text: user_msg})

result := tyk.makeHTTPRequest("POST", "https://pii-api.example.com/detect", headers, body)

// Parse response
response := json.decode(result.response)
has_pii := response.has_pii

output := {
    block: has_pii,
    payload: input.raw_input,
    message: has_pii ? "External PII service detected sensitive data" : ""
}
```

---

---

## Tool Response Filters

Filters can also be applied to tool responses (e.g., API calls, database queries). Tool responses are **plain strings**, not JSON-structured messages, so they require simpler handling.

### **Example 1: Block Tool Responses Containing Errors**

```tengo
text := import("text")

// Access tool response from messages array
tool_output := ""
if len(input.messages) > 0 {
    tool_output = input.messages[0].content
}

// Block if response indicates an error
should_block := text.contains(tool_output, "error") || text.contains(tool_output, "failed")

output := {
    block: should_block,
    payload: input.raw_input,
    message: should_block ? "Tool returned error response" : ""
}
```

### **Example 2: Redact PII from Tool Responses**

```tengo
text := import("text")

// Tool responses are plain strings - use direct string manipulation
modified := input.raw_input

// Get tool content
if len(input.messages) > 0 {
    content := input.messages[0].content

    // Redact email addresses
    modified = text.re_replace("[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}", content, "[EMAIL]")

    // Redact phone numbers
    modified = text.re_replace("\\(?\\d{3}\\)?[\\s.-]?\\d{3}[\\s.-]?\\d{4}", modified, "[PHONE]")
}

output := {
    block: false,
    payload: modified,
    message: "PII redacted from tool response"
}
```

### **Example 3: Filter Based on Tool Name**

```tengo
// Access tool metadata from context
tool_name := ""
if input.context && input.context.tool_name {
    tool_name = input.context.tool_name
}

// Only allow specific tools to return data
allowed_tools := ["weather_api", "stock_prices"]
is_allowed := false

for allowed in allowed_tools {
    if tool_name == allowed {
        is_allowed = true
        break
    }
}

output := {
    block: !is_allowed,
    payload: input.raw_input,
    message: is_allowed ? "" : "Tool '" + tool_name + "' is not allowed"
}
```

**Note**: The `tyk.redact_pattern()` helper is designed for LLM message structures (JSON format) and will not work with tool responses. For tool responses, use direct string manipulation with the `text` module as shown above.

---

---

## Filter Execution Order in Chat Sessions

When a user sends a message in a chat session, filters are executed at multiple points in the pipeline:

### **1. User Message Filters** (Before RAG)
**When**: After preprocessing, before RAG vector search
**Purpose**: Redact PII from user messages before they're used for vector similarity search
**Context**: `input.messages[0].role == "user"`

```tengo
text := import("text")

// Redact before RAG search
modified := text.replace(input.messages[0].content, "@", "[EMAIL]", -1)

output := {
    block: false,
    payload: modified,
    message: ""
}
```

**Impact**: The filtered/modified message is used for:
- ✅ RAG vector similarity search
- ✅ Subsequent LLM processing
- ✅ Chat history storage

### **2. File Content Filters** (Before RAG)
**When**: When files are attached to messages
**Purpose**: Filter sensitive content from uploaded files before indexing
**Context**: `input.context.file_ref` contains the file reference

### **3. Tool Response Filters** (After Tool Execution)
**When**: After a tool returns data, before sending to LLM
**Purpose**: Filter sensitive data from external API responses
**Context**: `input.messages[0].role == "tool"`, `input.context.tool_name` available

---

## Best Practices

1. **Always define `output`** - Scripts must set the output variable
2. **Use `tyk.redact_pattern` for LLM messages** - Handles vendor differences automatically
3. **Use `text` module for tool responses** - Direct string manipulation
4. **Use messages array for complex LLM modifications** - Gives you full control
5. **Provide clear block messages** - Help users understand policy violations
6. **Test across vendors** - OpenAI, Anthropic, and Google AI have different formats
7. **Check message roles** - Different logic for system, user, assistant, and tool messages
8. **Handle edge cases** - Empty arrays, missing fields, etc.
9. **Consider RAG impact** - Filters run before RAG, so redactions affect vector search

---

## Migration from Legacy API

**Old API** (still supported for backward compatibility):
```tengo
filter := func(payload) {
    // Returns true/false for blocking only
    return true
}
result := filter(payload)
```

**New Unified API** (recommended):
```tengo
// Rich input with messages, vendor info, context
output := {
    block: false,       // Blocking capability
    payload: "",        // Modification capability
    messages: [],       // Alternative modification approach
    message: ""         // Optional message
}
```

---

## Response Filters

**Response Filters** enable administrators to block LLM responses based on content analysis, providing governance controls on what LLMs can say to end users.

### Key Characteristics

1. **Block-Only**: Response filters can only block responses, not modify them
2. **Works on LLM Responses**: Applied to LLM responses only (not tool responses)
3. **Streaming Support**: Execute per-chunk during streaming with access to accumulated buffer
4. **Script-Controlled**: Filter scripts decide when to evaluate based on buffer length

### Configuration

Enable response filtering by checking **"Is this a Response Filter?"** when creating or editing a filter in the admin UI.

### Script API for Response Filters

Response filters use the same `ScriptInput`/`ScriptOutput` structure as request filters, with additional response-specific fields:

**Input Object (Non-Streaming):**
```tengo
input := {
    raw_input: "The LLM's complete response text",
    is_response: true,
    is_chunk: false,
    vendor_name: "openai",
    model_name: "gpt-4",
    is_chat: true,  // or false for proxy
    context: {
        llm_id: 123,
        session_id: "abc-123",  // Only in chat context
        user_id: 789,
        chat_id: 456,
        status_code: 200
    }
}
```

**Input Object (Streaming):**
```tengo
input := {
    raw_input: "current chunk text",
    is_response: true,
    is_chunk: true,
    chunk_index: 5,
    current_buffer: "accumulated response text so far",
    vendor_name: "openai",
    model_name: "gpt-4",
    is_chat: false,
    context: {
        llm_id: 123,
        app_id: 456
    }
}
```

**Output Object:**
```tengo
output := {
    block: false,   // Set true to block/interrupt response
    message: ""     // Block reason (shown to user)
}
```

**Note**: The `payload` and `messages` fields in output are **ignored** for response filters.

### Example 1: Block Refund Promises (Works for Streaming and Non-Streaming)

```tengo
text := import("text")

// Get response text - use buffer for streaming
response_text := input.is_chunk ? input.current_buffer : input.raw_input

// Default output
output := {
    block: false,
    message: ""
}

// For streaming: only evaluate once we have enough context
if !input.is_chunk || len(response_text) >= 100 {
    // Check for forbidden patterns
    forbidden := ["will refund", "issue a refund", "provide a refund", "get a refund"]
    should_block := false

    for pattern in forbidden {
        if text.contains(text.to_lower(response_text), pattern) {
            should_block = true
            break
        }
    }

    output = {
        block: should_block,
        message: should_block ? "Response blocked: Cannot promise refunds to customers" : ""
    }
}
```

### Example 2: Block Harmful Content (Streaming with Buffer Check)

```tengo
text := import("text")

// Get response text (streaming-aware)
response_text := input.is_chunk ? input.current_buffer : input.raw_input

// Default output
output := {
    block: false,
    message: ""
}

// For streaming: script controls when to evaluate based on buffer length
// Wait until we have enough context before checking
if !input.is_chunk || len(response_text) >= 150 {
    // Check for harmful instruction patterns
    harmful := [
        "instructions for making",
        "how to build a weapon",
        "steps to create explosives"
    ]

    is_harmful := false
    for pattern in harmful {
        if text.contains(text.to_lower(response_text), pattern) {
            is_harmful = true
            break
        }
    }

    output = {
        block: is_harmful,
        message: is_harmful ? "Response blocked: Potentially harmful content detected" : ""
    }
}
```

### Example 3: Combined with LLM Analysis

Use another LLM to analyze the response for policy violations:

```tengo
tyk := import("tyk")
text := import("text")

// Get response text
response_text := input.is_chunk ? input.current_buffer : input.raw_input

// Default output
output := {
    block: false,
    message: ""
}

// For streaming: only evaluate once buffer is large enough
if !input.is_chunk || len(response_text) >= 200 {
    // Use fast LLM to check for policy violations
    settings := {
        model_name: "gpt-3.5-turbo",
        temperature: 0.0,
        max_tokens: 5,
        system_prompt: "You are a content policy checker. Answer only 'yes' or 'no'."
    }

    check_prompt := "Does this response promise a refund or commit to a specific action?: " + response_text
    result := tyk.llm(1, settings, check_prompt)

    should_block := text.contains(text.to_lower(result), "yes")

    output = {
        block: should_block,
        message: should_block ? "Response blocked: LLM policy violation detected" : ""
    }
}
```

### Response Filter Execution

**Proxy (REST)**:
- Executes after response hooks (if any)
- Full response available in `input.raw_input`
- If blocked: Error returned to client instead of response

**Proxy (Streaming)**:
- Executes on every chunk
- Access to both `raw_input` (current chunk) and `current_buffer` (accumulated text)
- If blocked: Streaming stops, error sent to client

**Chat (Non-Streaming)**:
- Executes before adding to chat history
- If blocked: Error published to queue instead of response

**Chat (Streaming)**:
- Executes on every chunk before publishing to queue
- If blocked: Error published, streaming callback returns error to stop further chunks

### Best Practices for Response Filters

1. **Buffer Management** (Streaming): Script controls evaluation timing based on `len(current_buffer)`
2. **Performance**: Keep filter logic lightweight (executes per-chunk in streaming)
3. **Clear Messages**: Provide helpful block messages for users
4. **Fail Open**: Filters fail open on script errors (response allowed through)
5. **LLM-Only**: Response filters only work on LLM responses, not tool responses
6. **Block-Only**: Cannot modify responses, only block them

### When to Use Response Filters vs Request Filters

**Use Request Filters When:**
- Preventing sensitive data from reaching the LLM
- Redacting PII before LLM processing
- Enforcing input policies

**Use Response Filters When:**
- Preventing LLMs from making commitments (refunds, promises)
- Blocking harmful or inappropriate LLM outputs
- Enforcing corporate communication policies
- Detecting policy violations in LLM responses

---

This unified filter system demonstrates how flexible and powerful Midsommar's scripting capabilities are, enabling administrators to enforce strict data governance policies while supporting advanced LLM and tool integration workflows with both blocking and modification capabilities.
