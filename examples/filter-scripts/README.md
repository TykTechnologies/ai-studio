# Filter Script Examples

This directory contains example Tengo scripts for use with Midsommar's filter system.

## Script API

Filters receive a rich `input` object and must produce an `output` object.

### Input Object

```javascript
input := {
    raw_input: "...",      // Full JSON payload as string
    messages: [            // Array of normalized messages
        {role: "system", content: "You are helpful"},
        {role: "user", content: "Hello"},
        {role: "assistant", content: "Hi there"}
    ],
    vendor_name: "openai", // LLM vendor: openai, anthropic, google_ai, vertex, ollama
    model_name: "gpt-4",   // Model being called
    is_chat: false,        // true for chat sessions, false for proxy requests
    context: {             // Additional metadata
        llm_id: 123,
        app_id: 456,
        user_id: 789
    }
}
```

### Output Object

```javascript
output := {
    block: false,           // Set true to block the request
    payload: "",            // Modified JSON payload (empty = no change)
    messages: [],           // Alternative: return modified message array
    message: ""             // Optional reason/log message
}
```

## Approach 1: Using the Messages Array (Recommended for Complex Modifications)

This approach lets you modify individual messages without worrying about vendor-specific JSON formats.

### Example: Redact Email Addresses from User Messages

```javascript
text := import("text")

// Build new messages array with modifications
modified := []
for msg in input.messages {
    new_msg := {
        role: msg.role,
        content: msg.content
    }

    // Redact emails from user messages only
    if msg.role == "user" {
        new_msg.content = text.replace(msg.content, "@", "[REDACTED]", -1)
    }

    modified = append(modified, new_msg)
}

output := {
    block: false,
    messages: modified,  // System handles vendor-specific reconstruction
    message: "Emails redacted"
}
```

### Example: Add Prefix to System Prompt

```javascript
// Modify the system message
modified := []
for msg in input.messages {
    new_msg := {
        role: msg.role,
        content: msg.content
    }

    if msg.role == "system" {
        new_msg.content = "[SAFETY MODE] " + msg.content
    }

    modified = append(modified, new_msg)
}

output := {
    block: false,
    messages: modified,
    message: ""
}
```

### Example: Content Length Validation

```javascript
// Check all user messages for minimum length
for msg in input.messages {
    if msg.role == "user" {
        if len(msg.content) < 10 {
            output := {
                block: true,
                payload: "",
                message: "User message too short (minimum 10 characters)"
            }
            // Early exit - no need to check further
        }
    }
}

// If we get here, all messages are valid
output := {
    block: false,
    payload: input.raw_input,
    message: ""
}
```

## Approach 2: Using Helper Functions (Simple, Vendor-Agnostic)

The `midsommar` module provides helper functions for common redaction patterns.

### Example: Redact Email Addresses

```javascript
tyk := import("tyk")

// Redact all email addresses with a regex pattern
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

### Example: Redact Credit Card Numbers

```javascript
tyk := import("tyk")

// Redact credit card numbers (simple 16-digit pattern)
modified_payload := tyk.redact_pattern(
    input,
    "\\b\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}[\\s-]?\\d{4}\\b",
    "[CARD_REDACTED]"
)

output := {
    block: false,
    payload: modified_payload,
    message: "Credit cards redacted"
}
```

### Example: Redact Phone Numbers

```javascript
tyk := import("tyk")

// Redact various phone number formats
modified_payload := tyk.redact_pattern(
    input,
    "\\(?\\d{3}\\)?[\\s.-]?\\d{3}[\\s.-]?\\d{4}",
    "[PHONE_REDACTED]"
)

output := {
    block: false,
    payload: modified_payload,
    message: ""
}
```

## Approach 3: Blocking Based on Content

### Example: Block Requests Containing Sensitive Keywords

```javascript
text := import("text")

// Check for sensitive keywords
sensitive_keywords := ["password", "secret", "api_key", "token"]
should_block := false
found_keyword := ""

for msg in input.messages {
    if msg.role == "user" {
        for keyword in sensitive_keywords {
            if text.contains(msg.content, keyword) {
                should_block = true
                found_keyword = keyword
                break
            }
        }
    }
}

output := {
    block: should_block,
    payload: input.raw_input,
    message: should_block ? "Blocked: contains '" + found_keyword + "'" : ""
}
```

### Example: Enforce Vendor/Model Restrictions

```javascript
// Only allow OpenAI GPT-4 models
allowed_vendor := "openai"
allowed_models := ["gpt-4", "gpt-4-turbo", "gpt-4o"]

vendor_ok := input.vendor_name == allowed_vendor
model_ok := false

for model in allowed_models {
    if input.model_name == model {
        model_ok = true
        break
    }
}

output := {
    block: !(vendor_ok && model_ok),
    payload: input.raw_input,
    message: vendor_ok && model_ok ? "" : "Only OpenAI GPT-4 models allowed"
}
```

## Advanced Examples

### Example: PII Detection and Redaction

```javascript
text := import("text")
tyk := import("tyk")

// Multi-pattern PII redaction
modified := tyk.redact_pattern(input, "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}", "[EMAIL]")

// Chain multiple redactions by parsing the result
json := import("json")
parsed := json.decode(modified)

// Further modify if needed...

output := {
    block: false,
    payload: modified,
    message: "PII redacted"
}
```

### Example: Content Moderation with Message Context

```javascript
text := import("text")

// Check if user is asking about previous sensitive topics
conversation_history := []
for msg in input.messages {
    if msg.role == "user" || msg.role == "assistant" {
        conversation_history = append(conversation_history, msg.content)
    }
}

// Block if conversation contains banned topics
banned_topics := ["hack", "exploit", "bypass"]
should_block := false

for content in conversation_history {
    for topic in banned_topics {
        if text.contains(content, topic) {
            should_block = true
            break
        }
    }
}

output := {
    block: should_block,
    payload: input.raw_input,
    message: should_block ? "Conversation contains banned content" : ""
}
```

## Available Modules

- `text` - String operations (contains, replace, split, trim, etc.)
- `json` - JSON encoding/decoding
- `fmt` - Formatting and printing
- `base64` - Base64 encoding/decoding
- `times` - Time operations
- `midsommar` - Custom message modification helpers

## Helper Function Reference

### `midsommar.redact_pattern(input, pattern, replacement)`

Redacts a regex pattern from all messages (system, user, assistant).

**Parameters:**
- `input` - The input object
- `pattern` - Regular expression pattern (string)
- `replacement` - Replacement string

**Returns:** Modified payload as string

**Example:**
```javascript
tyk := import("tyk")
modified := tyk.redact_pattern(input, "\\d{3}-\\d{2}-\\d{4}", "[SSN]")
```

## Best Practices

1. **Always set the output variable** - Scripts must define `output`
2. **Preserve unchanged fields** - Use `input.raw_input` when not modifying
3. **Provide clear block messages** - Help users understand why requests are blocked
4. **Test with multiple vendors** - Different vendors have different JSON formats
5. **Use messages array for complex modifications** - Easier than parsing JSON manually
6. **Use helpers for simple patterns** - `redact_pattern` handles vendor differences
7. **Check roles before modifying** - Different logic for system, user, assistant messages
8. **Handle empty arrays** - Check `len(input.messages)` before iterating

## Debugging Tips

```javascript
fmt := import("fmt")

// Log the input to understand what you're working with
fmt.println("Vendor:", input.vendor_name)
fmt.println("Model:", input.model_name)
fmt.println("Message count:", len(input.messages))

for msg in input.messages {
    fmt.println("Role:", msg.role, "Content:", msg.content)
}
```
