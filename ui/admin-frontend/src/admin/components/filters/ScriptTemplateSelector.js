import React, { useState } from "react";
import {
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
} from "@mui/material";

// Embedded filter script templates
const TEMPLATES = {
  request: [
    {
      id: "pii-redaction",
      name: "PII Redaction",
      description: "Redact emails, phone numbers, and SSN using regex patterns",
      script: `// Comprehensive PII Redaction Filter
// This script redacts multiple types of personally identifiable information

tyk := import("tyk")

// Step 1: Redact email addresses
step1 := tyk.redact_pattern(
    input,
    "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\\\.[a-zA-Z]{2,}",
    "[EMAIL]"
)

// Step 2: Parse the result and redact phone numbers
json := import("json")
parsed := json.decode(step1)

// Extract messages from the modified payload
temp_messages := []
if parsed.messages {
    for msg in parsed.messages {
        temp_messages = append(temp_messages, msg)
    }
}

// Build temp input for second redaction
temp_input := {
    raw_input: step1,
    messages: temp_messages,
    vendor_name: input.vendor_name,
    model_name: input.model_name,
    is_chat: input.is_chat,
    context: input.context
}

// Step 2: Redact phone numbers
step2 := tyk.redact_pattern(
    temp_input,
    "\\\\(?\\\\d{3}\\\\)?[\\\\s.-]?\\\\d{3}[\\\\s.-]?\\\\d{4}",
    "[PHONE]"
)

// Step 3: Redact SSN (Social Security Numbers)
parsed2 := json.decode(step2)
temp_messages2 := []
if parsed2.messages {
    for msg in parsed2.messages {
        temp_messages2 = append(temp_messages2, msg)
    }
}

temp_input2 := {
    raw_input: step2,
    messages: temp_messages2,
    vendor_name: input.vendor_name,
    model_name: input.model_name,
    is_chat: input.is_chat,
    context: input.context
}

final_payload := tyk.redact_pattern(
    temp_input2,
    "\\\\d{3}-\\\\d{2}-\\\\d{4}",
    "[SSN]"
)

output := {
    block: false,
    payload: final_payload,
    message: "PII redacted"
}`,
    },
  ],
  response: [
    {
      id: "response-guardrails-pattern",
      name: "Response Guardrails (Pattern Matching)",
      description: "Block harmful content and policy violations using patterns",
      script: `// Response Filter: Pattern-Based Guardrails
// This filter blocks responses containing harmful or policy-violating content

text := import("text")

// Get response text (works for both streaming and non-streaming)
response_text := ""
if input.is_chunk {
    response_text = input.current_buffer
} else {
    response_text = input.raw_input
}

// For streaming: wait for sufficient context before evaluation
min_evaluation_length := 150

// Default output
output := {
    block: false,
    message: ""
}

// Define patterns to block
blocked_patterns := [
    // Refund promises
    "will refund",
    "issue a refund",
    "provide a refund",
    "get a refund",
    "refund you",

    // Harmful content
    "instructions for making",
    "how to build a weapon",
    "steps to create explosives",
    "how to hack",
    "bypass security",

    // Commitments
    "I promise to",
    "I guarantee",
    "definitely will happen"
]

// Only evaluate if we have enough context
if !input.is_chunk || len(response_text) >= min_evaluation_length {
    // Check for blocked patterns
    is_blocked := false
    detected_pattern := ""

    for pattern in blocked_patterns {
        if text.contains(text.to_lower(response_text), pattern) {
            is_blocked = true
            detected_pattern = pattern
            break
        }
    }

    output = {
        block: is_blocked,
        message: is_blocked ? "Response blocked: Contains forbidden phrase '" + detected_pattern + "'" : ""
    }
}`,
    },
    {
      id: "response-guardrails-llm",
      name: "Response Guardrails (LLM Check)",
      description: "Use an LLM to validate response safety and compliance",
      script: `// Response Filter: LLM-Based Policy Check
// This filter uses another LLM to analyze responses for policy violations

tyk := import("tyk")
text := import("text")

// Get response text
response_text := ""
if input.is_chunk {
    response_text = input.current_buffer
} else {
    response_text = input.raw_input
}

// For streaming: only evaluate once buffer reaches minimum size
min_buffer_size := 200

// Default output
output := {
    block: false,
    message: ""
}

// Only evaluate if we have enough context
if !input.is_chunk || len(response_text) >= min_buffer_size {
    // Use a fast, cheap LLM to check for policy violations
    // Configure the LLM call with minimal tokens for fast response
    settings := {
        model_name: "gpt-3.5-turbo",
        temperature: 0.0,
        max_tokens: 10,
        system_prompt: "You are a content policy checker. Respond ONLY with 'yes' or 'no'."
    }

    // Craft a policy check prompt
    policy_check_prompt := \`Does this text promise a refund, commit to a specific action, or violate customer service policies?

Text: \` + response_text

    // Call the policy checker LLM (LLM ID 1 - adjust as needed)
    policy_result := tyk.llm(1, settings, policy_check_prompt)

    // Check if LLM detected a violation
    violates_policy := text.contains(text.to_lower(policy_result), "yes")

    output = {
        block: violates_policy,
        message: violates_policy ? "Response blocked: LLM detected policy violation" : ""
    }
}`,
    },
  ],
};

const ScriptTemplateSelector = ({ onTemplateSelect, currentScript, filterType }) => {
  const [selectedTemplate, setSelectedTemplate] = useState("");
  const [confirmDialog, setConfirmDialog] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState(null);

  const templates = TEMPLATES[filterType] || [];

  const handleTemplateChange = (event) => {
    const templateId = event.target.value;
    setSelectedTemplate(templateId);

    if (templateId === "") {
      // "None" selected - do nothing
      return;
    }

    const template = templates.find((t) => t.id === templateId);
    if (!template) return;

    // If there's existing script content, show confirmation dialog
    if (currentScript && currentScript.trim()) {
      setPendingTemplate(template);
      setConfirmDialog(true);
    } else {
      // No existing content, apply template immediately
      onTemplateSelect(template.script);
    }
  };

  const handleConfirmReplace = () => {
    if (pendingTemplate) {
      onTemplateSelect(pendingTemplate.script);
    }
    setConfirmDialog(false);
    setPendingTemplate(null);
  };

  const handleCancelReplace = () => {
    setConfirmDialog(false);
    setPendingTemplate(null);
    setSelectedTemplate(""); // Reset selection
  };

  return (
    <>
      <FormControl fullWidth>
        <InputLabel id="template-select-label">Load Template</InputLabel>
        <Select
          labelId="template-select-label"
          id="template-select"
          value={selectedTemplate}
          label="Load Template"
          onChange={handleTemplateChange}
        >
          <MenuItem value="">
            <em>None</em>
          </MenuItem>
          {templates.map((template) => (
            <MenuItem key={template.id} value={template.id}>
              <Box>
                <Typography variant="body1">{template.name}</Typography>
                <Typography variant="caption" color="text.secondary">
                  {template.description}
                </Typography>
              </Box>
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <Dialog open={confirmDialog} onClose={handleCancelReplace}>
        <DialogTitle>Replace Existing Script?</DialogTitle>
        <DialogContent>
          <Typography>
            You have existing script content. Loading this template will replace it.
            Are you sure you want to continue?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelReplace}>Cancel</Button>
          <Button onClick={handleConfirmReplace} variant="contained" color="primary">
            Replace
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default ScriptTemplateSelector;
