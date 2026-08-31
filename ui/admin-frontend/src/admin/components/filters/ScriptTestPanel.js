import React, { useState } from "react";
import {
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Typography,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Grid,
  Button,
  Paper,
  Box,
  FormControlLabel,
  Checkbox,
  CircularProgress,
  FormHelperText,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import apiClient from "../../utils/apiClient";

const ScriptTestPanel = ({ script, filterType }) => {
  const [testInput, setTestInput] = useState({
    raw_input: filterType === "response"
      ? "I will issue a refund to your account immediately."
      : "Tell me how to bypass security systems",
    vendor_name: "openai",
    model_name: "gpt-4",
    is_response: filterType === "response",
    // The proxy hardcodes IsChat: false on both the request and response
    // filter paths, so defaulting this to true tested a case that never runs.
    is_chat: false,
    is_chunk: false,
    chunk_index: 0,
    current_buffer: "",
    status_code: 200,
    // The shape the proxy actually supplies (see proxy.go and
    // response_filter_utils.go): llm_id, app_id and request_id, and no
    // user_id at all. The panel used to suggest {"app_id": 1, "user_id": 5},
    // so a filter written against it read undefined in production.
    context: '{"llm_id": 1, "app_id": 1, "request_id": "test-request-id"}',
  });

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState(null);

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setTestInput({ ...testInput, [name]: value });
  };

  const handleCheckboxChange = (e) => {
    const { name, checked } = e.target;
    setTestInput({ ...testInput, [name]: checked });
  };

  const handleTest = async () => {
    if (!script || !script.trim()) {
      setTestResult({
        success: false,
        error: "No script to test. Please write a script first.",
      });
      return;
    }

    setTesting(true);
    setTestResult(null);

    try {
      // Parse context JSON
      let contextObj = {};
      if (testInput.context.trim()) {
        try {
          contextObj = JSON.parse(testInput.context);
        } catch (e) {
          setTestResult({
            success: false,
            error: `Invalid JSON in context field: ${e.message}`,
          });
          setTesting(false);
          return;
        }
      }

      // tyk.redact_pattern switches behaviour on whether raw_input parses as
      // JSON, so pasting a real request body -- the obvious thing to paste --
      // used to come back with the messages stripped out, because this was
      // hardcoded to []. That looks like a catastrophic redaction failure and
      // is nothing of the sort. Derive them from the body instead.
      let derivedMessages = [];
      try {
        const parsed = JSON.parse(testInput.raw_input);
        if (parsed && Array.isArray(parsed.messages)) {
          derivedMessages = parsed.messages;
        }
      } catch (e) {
        // raw_input is plain text, which is a legitimate way to use the panel.
      }

      // Build script input
      const scriptInput = {
        raw_input: testInput.raw_input,
        vendor_name: testInput.vendor_name,
        model_name: testInput.model_name,
        is_response: testInput.is_response,
        is_chat: testInput.is_chat,
        is_chunk: testInput.is_chunk,
        chunk_index: parseInt(testInput.chunk_index, 10),
        current_buffer: testInput.current_buffer,
        status_code: parseInt(testInput.status_code, 10),
        context: contextObj,
        messages: derivedMessages,
      };

      const response = await apiClient.post("/filters/test", {
        script: script,
        input: scriptInput,
      });

      setTestResult(response.data);
    } catch (error) {
      console.error("Test execution error:", error);
      setTestResult({
        success: false,
        error: error.response?.data?.error || error.message || "Unknown error occurred",
      });
    } finally {
      setTesting(false);
    }
  };

  const getResultColor = () => {
    if (!testResult) return "grey.100";
    if (!testResult.success) return "#ffebee"; // Light red
    if (testResult.output?.block) return "#fff3e0"; // Light orange
    return "#e8f5e9"; // Light green
  };

  const getResultIcon = () => {
    if (!testResult) return "";
    if (!testResult.success) return "❌";
    if (testResult.output?.block) return "🚫";
    return "✅";
  };

  return (
    <Accordion>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        aria-controls="test-panel-content"
        id="test-panel-header"
      >
        <Typography variant="h6">Test</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              Test your filter script with sample input before saving. Configure the input
              parameters below and click "Run script" to see the output.
            </Typography>
          </Grid>

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Raw Input"
              name="raw_input"
              value={testInput.raw_input}
              onChange={handleInputChange}
              multiline
              rows={4}
              placeholder={
                filterType === "response"
                  ? "Sample LLM response text"
                  : "Sample user input text"
              }
            />
          </Grid>

          <Grid item xs={6}>
            <FormControl fullWidth>
              <InputLabel id="scripttestpanel-vendor-label">Vendor</InputLabel>
              <Select
              labelId="scripttestpanel-vendor-label"
              id="scripttestpanel-vendor"
                name="vendor_name"
                value={testInput.vendor_name}
                onChange={handleInputChange}
                label="Vendor"
              >
                <MenuItem value="openai">OpenAI</MenuItem>
                <MenuItem value="anthropic">Anthropic</MenuItem>
                <MenuItem value="google">Google AI</MenuItem>
                <MenuItem value="azure">Azure OpenAI</MenuItem>
              </Select>
              <FormHelperText>
                LLM provider (available in script as input.vendor_name)
              </FormHelperText>
            </FormControl>
          </Grid>

          <Grid item xs={6}>
            <TextField
              fullWidth
              label="Model Name"
              name="model_name"
              value={testInput.model_name}
              onChange={handleInputChange}
              placeholder="gpt-4, claude-3-opus, etc."
              helperText="Model identifier (available in script as input.model_name)"
            />
          </Grid>

          <Grid item xs={4}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={testInput.is_response}
                  onChange={handleCheckboxChange}
                  name="is_response"
                />
              }
              label="Is Response"
            />
          </Grid>

          <Grid item xs={4}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={testInput.is_chat}
                  onChange={handleCheckboxChange}
                  name="is_chat"
                />
              }
              label="Is Chat"
            />
          </Grid>

          <Grid item xs={4}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={testInput.is_chunk}
                  onChange={handleCheckboxChange}
                  name="is_chunk"
                />
              }
              label="Is Chunk (Streaming)"
            />
          </Grid>

          {testInput.is_chunk && (
            <>
              <Grid item xs={4}>
                <TextField
                  fullWidth
                  label="Chunk Index"
                  name="chunk_index"
                  type="number"
                  value={testInput.chunk_index}
                  onChange={handleInputChange}
                />
              </Grid>

              <Grid item xs={4}>
                <TextField
                  fullWidth
                  label="Status Code"
                  name="status_code"
                  type="number"
                  value={testInput.status_code}
                  onChange={handleInputChange}
                />
              </Grid>

              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Current Buffer (accumulated text)"
                  name="current_buffer"
                  value={testInput.current_buffer}
                  onChange={handleInputChange}
                  multiline
                  rows={3}
                  helperText="For streaming: the accumulated response text so far"
                />
              </Grid>
            </>
          )}

          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Context (JSON)"
              name="context"
              value={testInput.context}
              onChange={handleInputChange}
              multiline
              rows={2}
              placeholder='{"app_id": 1, "user_id": 5, "session_id": "abc123"}'
              helperText="Additional context metadata as JSON"
            />
          </Grid>

          <Grid item xs={12}>
            <Button
              variant="contained"
              onClick={handleTest}
              disabled={testing || !script}
              startIcon={testing && <CircularProgress size={20} />}
            >
              {testing ? "Running..." : "Run script"}
            </Button>
          </Grid>

          {testResult && (
            <Grid item xs={12}>
              <Paper
                sx={{
                  p: 2,
                  bgcolor: getResultColor(),
                  border: "1px solid",
                  borderColor: testResult.success ? "success.main" : "error.main",
                }}
              >
                <Typography variant="subtitle2" gutterBottom>
                  {getResultIcon()} Test Result:
                </Typography>

                {testResult.success ? (
                  <>
                    {/* Compliance events used to be collected server-side and
                        dropped from the response, so a filter that raised them
                        looked silent here. Call them out rather than leaving
                        them buried in the JSON below. */}
                    {testResult.output?.compliance_events?.length > 0 && (
                      <Box sx={{ mb: 1 }}>
                        <Typography variant="caption" sx={{ fontWeight: 600 }}>
                          Compliance events raised:
                        </Typography>
                        {testResult.output.compliance_events.map((event, i) => (
                          <Typography
                            key={i}
                            variant="caption"
                            display="block"
                            sx={{ ml: 1 }}
                          >
                            [{event.severity}] {event.event_type}
                            {event.description ? ` — ${event.description}` : ""}
                          </Typography>
                        ))}
                      </Box>
                    )}
                    <Box component="pre" sx={{ fontSize: 12, overflow: "auto", m: 0 }}>
                      {JSON.stringify(testResult.output, null, 2)}
                    </Box>
                  </>
                ) : (
                  <Typography color="error" component="pre" sx={{ fontSize: 12, m: 0 }}>
                    {testResult.error}
                  </Typography>
                )}
              </Paper>
            </Grid>
          )}
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
};

export default ScriptTestPanel;
