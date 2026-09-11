import React, { useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Chip,
  FormControl,
  FormControlLabel,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { StyledAccordion } from "../../styles/sharedStyles";

export const DEFAULT_FAILOVER_STATUS_CODES = [408, 429, 500, 502, 503, 504];

export const INHERITED_ACCESS_NOTE =
  "Apps that are allowed to use this LLM will automatically be routed to these fallback LLMs when it fails, even if they have not been granted access to them directly. Budgets are still enforced on the fallback LLM.";

// modelAllowed mirrors the server rule: an empty pattern list allows every
// model, and patterns match anywhere in the name (unanchored).
export const modelAllowed = (patterns, model) => {
  if (!patterns || patterns.length === 0) return true;
  return patterns.some((pattern) => {
    try {
      return new RegExp(pattern).test(model);
    } catch (e) {
      return false;
    }
  });
};

// validateFailover returns a map of row index -> message (and "_" for
// waterfall-level problems). It mirrors services.ValidateLLMFailover so the
// form can highlight a rung before the server rejects it.
export const validateFailover = (failover, llmsById, current) => {
  const errors = {};
  const targets = failover?.targets || [];
  const seen = new Set();
  targets.forEach((t, i) => {
    if (!t.llm_id) {
      errors[i] = "Select an LLM";
      return;
    }
    if (!t.model || !t.model.trim()) {
      errors[i] = "A model is required";
      return;
    }
    const key = `${t.llm_id}:${t.model}`;
    if (seen.has(key)) {
      errors[i] = "Duplicate failover target";
      return;
    }
    seen.add(key);
    const target = llmsById[String(t.llm_id)];
    if (!target) {
      errors[i] = "This LLM no longer exists";
      return;
    }
    if (!modelAllowed(target.allowed_models, t.model)) {
      errors[i] = `Model "${t.model}" is not in the allowed models of ${target.name}`;
      return;
    }
    if (Number(target.privacy_score) < Number(current.privacy_score || 0)) {
      errors[i] = `${target.name} has a lower privacy level (${target.privacy_score}) than this LLM (${current.privacy_score})`;
      return;
    }
    if (target.namespace && target.namespace !== (current.namespace || "")) {
      errors[i] = `${target.name} is in namespace "${target.namespace}", which this LLM cannot reach`;
    }
  });
  if (targets.length > 10) {
    errors._ = "At most 10 failover targets are allowed";
  }
  (failover?.triggers?.status_codes || []).forEach((code) => {
    const n = Number(code);
    if (!((n >= 500 && n <= 599) || n === 408 || n === 429)) {
      errors._ = `Status ${code} cannot trigger failover; only 5xx, 408 and 429 can`;
    }
  });
  return errors;
};

const LLMFailoverSection = ({ value, onChange, llms, currentId, errors = {} }) => {
  const [newCode, setNewCode] = useState("");
  const targets = value?.targets || [];
  const triggers = value?.triggers || {};
  const candidates = llms.filter((l) => String(l.id) !== String(currentId));
  const llmsById = Object.fromEntries(llms.map((l) => [String(l.id), l]));

  const update = (nextTargets, nextTriggers = triggers) => {
    const hasTriggers = nextTriggers && Object.keys(nextTriggers).length > 0;
    onChange({ targets: nextTargets, triggers: hasTriggers ? nextTriggers : null });
  };

  const setTarget = (index, patch) => {
    const next = targets.map((t, i) => (i === index ? { ...t, ...patch } : t));
    update(next);
  };

  const move = (index, delta) => {
    const to = index + delta;
    if (to < 0 || to >= targets.length) return;
    const next = [...targets];
    [next[index], next[to]] = [next[to], next[index]];
    update(next);
  };

  const modelOptions = (llmId) => {
    const target = llmsById[String(llmId)];
    if (!target) return [];
    const opts = [...(target.allowed_models || [])];
    if (target.default_model && !opts.includes(target.default_model)) {
      opts.unshift(target.default_model);
    }
    return opts;
  };

  const statusCodes = triggers.status_codes || [];
  const addCode = () => {
    const n = parseInt(newCode, 10);
    if (!n || statusCodes.includes(n)) return;
    update(targets, { ...triggers, status_codes: [...statusCodes, n] });
    setNewCode("");
  };

  return (
    <Box data-testid="llm-failover-section">
      <Typography variant="subtitle2" gutterBottom>
        Failover
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        When this LLM's upstream fails, requests are retried against these
        fallbacks in order. Each fallback names another LLM and the model to
        ask it for, which must be one of that LLM's allowed models. Failover
        applies to the OpenAI-compatible chat endpoints and only before the
        first token of a streamed response has been sent.
      </Typography>
      <Alert severity="info" sx={{ mb: 2 }}>
        {INHERITED_ACCESS_NOTE}
      </Alert>
      {errors._ && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errors._}
        </Alert>
      )}

      {targets.map((t, index) => {
        const target = llmsById[String(t.llm_id)];
        const allowsAll = target && (!target.allowed_models || target.allowed_models.length === 0);
        return (
          <Grid container spacing={2} alignItems="center" key={index} sx={{ mb: 1 }} data-testid={`failover-row-${index}`}>
            <Grid item xs={12} sm={1}>
              <Typography variant="body2" color="text.secondary">
                {index + 1}.
              </Typography>
            </Grid>
            <Grid item xs={12} sm={4}>
              <FormControl fullWidth error={Boolean(errors[index])}>
                <InputLabel id={`failover-llm-${index}`}>Fallback LLM</InputLabel>
                <Select
                  labelId={`failover-llm-${index}`}
                  label="Fallback LLM"
                  value={t.llm_id ? String(t.llm_id) : ""}
                  inputProps={{ "data-testid": `failover-llm-select-${index}` }}
                  onChange={(e) => {
                    const llm = llmsById[e.target.value];
                    setTarget(index, {
                      llm_id: Number(e.target.value),
                      model: llm?.default_model || "",
                    });
                  }}
                >
                  {candidates.map((l) => (
                    <MenuItem key={l.id} value={String(l.id)}>
                      {l.name}
                      {l.active === false ? " (inactive)" : ""}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12} sm={5}>
              <Autocomplete
                freeSolo
                options={modelOptions(t.llm_id)}
                value={t.model || ""}
                inputValue={t.model || ""}
                onInputChange={(_, v) => setTarget(index, { model: v })}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Model"
                    error={Boolean(errors[index])}
                    helperText={
                      errors[index] ||
                      (allowsAll
                        ? "This LLM allows any model"
                        : target
                        ? "Must match one of the LLM's allowed models"
                        : "")
                    }
                    inputProps={{ ...params.inputProps, "data-testid": `failover-model-${index}` }}
                  />
                )}
              />
            </Grid>
            <Grid item xs={12} sm={2}>
              <IconButton size="small" aria-label="move up" disabled={index === 0} onClick={() => move(index, -1)}>
                <ArrowUpwardIcon fontSize="small" />
              </IconButton>
              <IconButton
                size="small"
                aria-label="move down"
                disabled={index === targets.length - 1}
                onClick={() => move(index, 1)}
              >
                <ArrowDownwardIcon fontSize="small" />
              </IconButton>
              <IconButton
                size="small"
                aria-label="remove fallback"
                onClick={() => update(targets.filter((_, i) => i !== index))}
              >
                <DeleteIcon fontSize="small" />
              </IconButton>
            </Grid>
          </Grid>
        );
      })}

      <Box sx={{ mb: 2 }}>
        <IconButton
          aria-label="add fallback"
          disabled={candidates.length === 0 || targets.length >= 10}
          onClick={() => update([...targets, { llm_id: 0, model: "" }])}
        >
          <AddIcon />
        </IconButton>
        <Typography variant="body2" component="span" color="text.secondary">
          {candidates.length === 0 ? "No other LLMs to fall back to" : "Add fallback"}
        </Typography>
      </Box>

      {targets.length > 0 && (
        <StyledAccordion>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Typography>Failover Triggers (advanced)</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Status codes that trigger failover. Defaults to{" "}
              {DEFAULT_FAILOVER_STATUS_CODES.join(", ")}. Only 5xx, 408 and 429
              can be used: other 4xx responses are caller or configuration
              errors that every fallback would repeat.
            </Typography>
            <Box sx={{ display: "flex", gap: 1, mb: 2, alignItems: "center" }}>
              <TextField
                label="Status code"
                size="small"
                value={newCode}
                onChange={(e) => setNewCode(e.target.value)}
                onKeyPress={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addCode();
                  }
                }}
                inputProps={{ "data-testid": "failover-status-code-input" }}
              />
              <IconButton onClick={addCode} aria-label="add status code">
                <AddIcon />
              </IconButton>
            </Box>
            <Stack direction="row" spacing={1} flexWrap="wrap" sx={{ gap: 1, mb: 2 }}>
              {(statusCodes.length > 0 ? statusCodes : DEFAULT_FAILOVER_STATUS_CODES).map((code) => (
                <Chip
                  key={code}
                  label={code}
                  variant={statusCodes.length > 0 ? "filled" : "outlined"}
                  onDelete={
                    statusCodes.length > 0
                      ? () => {
                          const rest = statusCodes.filter((c) => c !== code);
                          const { status_codes, ...others } = triggers;
                          update(targets, rest.length > 0 ? { ...others, status_codes: rest } : others);
                        }
                      : undefined
                  }
                />
              ))}
            </Stack>
            <FormControlLabel
              control={
                <Switch
                  checked={triggers.on_timeout !== false}
                  onChange={(e) => update(targets, { ...triggers, on_timeout: e.target.checked })}
                />
              }
              label="Fail over when the upstream times out"
            />
            <FormControlLabel
              control={
                <Switch
                  checked={triggers.on_connection_error !== false}
                  onChange={(e) => update(targets, { ...triggers, on_connection_error: e.target.checked })}
                />
              }
              label="Fail over when the upstream cannot be reached"
            />
            <TextField
              sx={{ mt: 2 }}
              fullWidth
              type="number"
              label="Per-attempt timeout (seconds)"
              value={triggers.attempt_timeout_seconds || ""}
              onChange={(e) => {
                const n = parseInt(e.target.value, 10);
                const { attempt_timeout_seconds, ...others } = triggers;
                update(targets, n > 0 ? { ...others, attempt_timeout_seconds: n } : others);
              }}
              helperText="How long to wait on each attempt before moving to the next fallback. Leave empty to use the gateway's LLM timeout."
              inputProps={{ min: 1 }}
            />
          </AccordionDetails>
        </StyledAccordion>
      )}
    </Box>
  );
};

export default LLMFailoverSection;
