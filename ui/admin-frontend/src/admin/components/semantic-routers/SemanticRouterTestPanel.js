import React, { useState } from "react";
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  FormControl,
  Grid,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import apiClient from "../../utils/apiClient";
import { PrimaryButton } from "../../styles/sharedStyles";
import { REASON_LABELS, TARGET_MODEL_ROUTER, apiErrorDetail } from "./semanticRouterModel";

const nameById = (rows, id) => {
  const row = (rows || []).find((r) => String(r.id) === String(id));
  return row?.attributes?.name || null;
};

/** "gpt-4o on OpenAI" / "alias cheap via model router Prod". */
export const describeTarget = (target, { llms = [], modelRouters = [] } = {}) => {
  if (!target) return "";
  if (target.type === TARGET_MODEL_ROUTER) {
    const name = nameById(modelRouters, target.model_router_id) || `#${target.model_router_id}`;
    return `${target.model} via model router ${name}`;
  }
  const name = nameById(llms, target.llm_id) || `LLM #${target.llm_id}`;
  return `${target.model} on ${name}`;
};

const formatScore = (score) => (typeof score === "number" ? score.toFixed(2) : "—");

// Per-route scores of one stage, highest first, as small bars.
const StageScores = ({ scores }) => {
  const entries = Object.entries(scores || {}).sort((a, b) => b[1] - a[1]);
  if (entries.length === 0) return null;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, minWidth: 180 }}>
      {entries.map(([route, score]) => (
        <Box key={route} sx={{ display: "flex", alignItems: "center", gap: 1 }} data-testid="stage-score">
          <Typography variant="caption" sx={{ width: 80, fontFamily: "monospace" }} noWrap title={route}>
            {route}
          </Typography>
          <LinearProgress
            variant="determinate"
            value={Math.max(0, Math.min(1, score)) * 100}
            sx={{ flexGrow: 1, height: 6, borderRadius: 3 }}
          />
          <Typography variant="caption" sx={{ width: 32, textAlign: "right" }}>
            {formatScore(score)}
          </Typography>
        </Box>
      ))}
    </Box>
  );
};

/** The outcome of one test: the chosen route, why, and the per-stage trace. */
export const DecisionView = ({ decision, target, llms, modelRouters }) => (
  <Box data-testid="semantic-router-decision">
    <Grid container spacing={2} sx={{ mb: 2 }}>
      <Grid item xs={12} sm={6} md={3}>
        <Typography variant="body2" color="text.secondary">
          Route
        </Typography>
        <Chip label={decision.route || "—"} color="primary" size="small" data-testid="decision-route" />
      </Grid>
      <Grid item xs={12} sm={6} md={3}>
        <Typography variant="body2" color="text.secondary">
          Reason
        </Typography>
        <Typography variant="body1" data-testid="decision-reason">
          {REASON_LABELS[decision.reason] || decision.reason || "—"}
        </Typography>
      </Grid>
      <Grid item xs={6} md={2}>
        <Typography variant="body2" color="text.secondary">
          Score
        </Typography>
        <Typography variant="body1" data-testid="decision-score">
          {formatScore(decision.score)}
        </Typography>
      </Grid>
      <Grid item xs={6} md={2}>
        <Typography variant="body2" color="text.secondary">
          Latency
        </Typography>
        <Typography variant="body1">{`${decision.latency_ms ?? 0} ms`}</Typography>
      </Grid>
      {decision.shadow_route && (
        <Grid item xs={12} md={2}>
          <Typography variant="body2" color="text.secondary">
            Shadow route
          </Typography>
          <Chip label={decision.shadow_route} size="small" variant="outlined" data-testid="decision-shadow-route" />
        </Grid>
      )}
      {target && (
        <Grid item xs={12}>
          <Typography variant="body2" color="text.secondary">
            Served by
          </Typography>
          <Typography variant="body1" data-testid="decision-target">
            {describeTarget(target, { llms, modelRouters })}
          </Typography>
        </Grid>
      )}
      {decision.shadow_route && (
        <Grid item xs={12}>
          <Typography variant="caption" color="text.secondary">
            Shadow mode: the classifier chose <code>{decision.shadow_route}</code>, but the request is
            served by the default route.
          </Typography>
        </Grid>
      )}
      {decision.input && (
        <Grid item xs={12}>
          <Typography variant="body2" color="text.secondary">
            Text classified
          </Typography>
          <Typography variant="body2" sx={{ fontFamily: "monospace", whiteSpace: "pre-wrap" }}>
            {decision.input}
          </Typography>
        </Grid>
      )}
    </Grid>
    {(decision.trace || []).length > 0 && (
      <Table size="small" aria-label="Classification trace">
        <TableHead>
          <TableRow>
            <TableCell>Stage</TableCell>
            <TableCell>Route</TableCell>
            <TableCell>Scores</TableCell>
            <TableCell>Detail</TableCell>
            <TableCell align="right">Latency</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {decision.trace.map((stage, index) => (
            <TableRow key={`${stage.stage}-${index}`} data-testid="trace-row">
              <TableCell>
                {stage.stage}
                {stage.skipped && <Chip label="skipped" size="small" sx={{ ml: 1 }} />}
              </TableCell>
              <TableCell>{stage.route || "—"}</TableCell>
              <TableCell>
                <StageScores scores={stage.scores} />
              </TableCell>
              <TableCell>
                {stage.error ? (
                  <Typography variant="body2" color="error">
                    {stage.error}
                  </Typography>
                ) : (
                  stage.detail || ""
                )}
              </TableCell>
              <TableCell align="right">{`${stage.latency_ms ?? 0} ms`}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    )}
  </Box>
);

/**
 * Sends one user message through the router's classifier and shows the
 * decision. With `routerId` the saved router is tested
 * (POST /semantic-routers/:id/test); otherwise `getDraft()` supplies the
 * unsaved attributes (POST /semantic-routers/test).
 */
const SemanticRouterTestPanel = ({
  routerId,
  getDraft,
  routeNames = [],
  allowExplicit = false,
  llms = [],
  modelRouters = [],
}) => {
  const [message, setMessage] = useState("");
  const [model, setModel] = useState("auto");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState("");

  const modelOptions = allowExplicit ? ["auto", ...routeNames] : ["auto"];
  const selectedModel = modelOptions.includes(model) ? model : "auto";

  const runTest = async () => {
    setRunning(true);
    setError("");
    setResult(null);
    const body = {
      messages: [{ role: "user", content: message }],
      ...(selectedModel !== "auto" && { model: selectedModel }),
    };
    try {
      const response = routerId
        ? await apiClient.post(`/semantic-routers/${routerId}/test`, body)
        : await apiClient.post("/semantic-routers/test", { router: getDraft(), ...body });
      setResult(response?.data?.data || null);
    } catch (err) {
      console.error("Error testing semantic router:", err);
      setError(apiErrorDetail(err) || "The test failed");
    } finally {
      setRunning(false);
    }
  };

  return (
    <Box data-testid="semantic-router-test-panel">
      <Grid container spacing={2} alignItems="flex-start">
        <Grid item xs={12} md={allowExplicit ? 9 : 12}>
          <TextField
            fullWidth
            multiline
            minRows={2}
            label="Test prompt"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            helperText="Sent as one user message. Nothing is forwarded to the target; only the classifier (and any embedding or judge LLM) is called."
          />
        </Grid>
        {allowExplicit && (
          <Grid item xs={12} md={3}>
            <FormControl fullWidth>
              <InputLabel id="semantic-router-test-model-label">Model</InputLabel>
              <Select
                labelId="semantic-router-test-model-label"
                value={selectedModel}
                label="Model"
                onChange={(e) => setModel(e.target.value)}
              >
                {modelOptions.map((option) => (
                  <MenuItem key={option} value={option}>
                    {option}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
        )}
        <Grid item xs={12}>
          <PrimaryButton
            variant="contained"
            onClick={runTest}
            disabled={running || !message.trim()}
            startIcon={running ? <CircularProgress size={16} color="inherit" /> : null}
          >
            Test routing
          </PrimaryButton>
        </Grid>
        {error && (
          <Grid item xs={12}>
            <Alert severity="error" data-testid="semantic-router-test-error">
              {error}
            </Alert>
          </Grid>
        )}
        {result?.decision && (
          <Grid item xs={12}>
            <DecisionView decision={result.decision} target={result.target} llms={llms} modelRouters={modelRouters} />
          </Grid>
        )}
      </Grid>
    </Box>
  );
};

export default SemanticRouterTestPanel;
