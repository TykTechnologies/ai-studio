import React, { useMemo, useState } from "react";
import { Box, Chip, Paper, Stack, Typography } from "@mui/material";
import HowToVoteIcon from "@mui/icons-material/HowToVote";
import SmartToyOutlinedIcon from "@mui/icons-material/SmartToyOutlined";
import { ApprovalCard, FormCard } from "../../../../portal/components/chat-v2/parts/HumanToolCard";
import { sampleValues } from "./schemaFields";

const slugify = (s) =>
  String(s || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

/**
 * Shows the tool the way it will appear: first as the assistant sees it
 * (the function it can call and the description that tells it when), then
 * the card the person in the chat gets, filled with example arguments.
 */
const ClientToolPreview = ({ kind, toolName, toolDescription, title, instructions, parameterFields, responseSchema }) => {
  const [answer, setAnswer] = useState(null);
  const args = useMemo(() => sampleValues(parameterFields), [parameterFields]);
  const fnName = slugify(toolName) || "your-tool";
  const cardTitle = title || toolDescription || toolName || "Your input is needed";
  const argNames = Object.keys(args);

  return (
    <Stack spacing={2} data-testid="client-tool-preview">
      <Paper variant="outlined" sx={{ p: 2, bgcolor: "background.default" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
          <SmartToyOutlinedIcon fontSize="small" color="action" />
          <Typography variant="subtitle2">When the assistant uses it</Typography>
        </Box>
        <Typography variant="body2" sx={{ mb: 1 }}>
          The assistant sees a function called <code>{fnName}</code>
          {argNames.length > 0 && (
            <>
              {" "}
              taking <code>{argNames.join(", ")}</code>
            </>
          )}
          . It calls it when the description below applies to the conversation, so write the description as an instruction: what
          the tool is for, and when to reach for it.
        </Typography>
        <Paper variant="outlined" sx={{ p: 1.5, bgcolor: "background.paper" }}>
          <Typography variant="body2" color={toolDescription ? "text.primary" : "text.secondary"} sx={{ whiteSpace: "pre-wrap" }}>
            {toolDescription || "No description yet. Example: \"Collect a shipping address whenever an order needs a delivery destination.\""}
          </Typography>
        </Paper>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 1 }}>
          While the card is open the reply pauses; the answer is sent back to the assistant as the function's result
          {kind === "approval" ? ' ({"approved": true|false, "comment": "..."})' : " (the submitted fields)"}.
        </Typography>
      </Paper>

      <Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
          <HowToVoteIcon fontSize="small" color="warning" />
          <Typography variant="subtitle2">What the person in the chat sees</Typography>
          <Chip size="small" label="example values" variant="outlined" />
        </Box>
        <Paper
          variant="outlined"
          sx={{ p: 2, borderLeft: "4px solid", borderLeftColor: "warning.main", bgcolor: "background.surfaceWarningDefault", maxWidth: 640 }}
          data-testid="client-tool-preview-card"
        >
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
            Your input is needed
          </Typography>
          {kind === "form" ? (
            <FormCard key={JSON.stringify(responseSchema)} title={cardTitle} description={instructions} args={args} schema={responseSchema} addResult={(v) => setAnswer(v)} nested />
          ) : (
            <ApprovalCard title={cardTitle} description={instructions} args={args} addResult={(v) => setAnswer(v)} />
          )}
        </Paper>
        {answer && (
          <Paper variant="outlined" sx={{ p: 1.5, mt: 1, maxWidth: 640 }} data-testid="client-tool-preview-answer">
            <Typography variant="caption" color="text.secondary">
              The assistant would receive:
            </Typography>
            <Box component="pre" sx={{ m: 0, mt: 0.5, fontSize: 12, whiteSpace: "pre-wrap" }}>
              {JSON.stringify(answer, null, 2)}
            </Box>
          </Paper>
        )}
      </Box>
    </Stack>
  );
};

export default ClientToolPreview;
