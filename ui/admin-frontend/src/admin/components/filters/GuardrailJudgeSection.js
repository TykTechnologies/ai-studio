import React, { useEffect, useState } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  FormControl,
  FormHelperText,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  Typography,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import apiClient from "../../utils/apiClient";
import { listAll } from "../../utils/listAll";
import { chatCapableLLMs } from "../../utils/vendorLogos";
import RowSelect from "../common/RowSelect";
import { JUDGE_EVENT_TYPE, hasOpinion, judgeOf } from "../compliance/judgeOpinions";

// The guardrail's second opinion (guardrails.Judge): a gateway LLM asked
// whether what the filter blocks or quarantines is in fact benign for the app.
// Stored under config.judge; absent means off.
export const JUDGE_MODES = [
  { value: "off", label: "Off" },
  { value: "record", label: "Record only" },
  { value: "soften", label: "Soften blocks" },
];

export const DEFAULT_JUDGE_TIMEOUT_MS = 3000;
export const MAX_JUDGE_TIMEOUT_MS = 30000;

// judgeErrors mirrors guardrails.Judge.normalize, so the form can say what is
// wrong before the server does.
export const judgeErrors = (judge) => {
  const errors = {};
  if (!judge || !judge.mode || judge.mode === "off") return errors;
  if (!judge.llm_id) errors.llm_id = "Choose the LLM that gives the second opinion";
  const t = judge.timeout_ms;
  if (t !== undefined && t !== null && t !== "" && (Number(t) < 0 || Number(t) > MAX_JUDGE_TIMEOUT_MS)) {
    errors.timeout_ms = `Between 0 and ${MAX_JUDGE_TIMEOUT_MS} ms`;
  }
  return errors;
};

// MIN_JUDGE_OPINIONS is how many opinions a filter's judge must have
// recorded (Record only, last OPINION_DAYS days) before Soften blocks can be
// picked: the evidence to review first. An advisory gate in the UI only; the
// server's guarantees are the floor, full coverage and benign-only.
export const MIN_JUDGE_OPINIONS = 20;
export const OPINION_DAYS = 30;

// countOpinions is the filter's recorded judge opinions (with a verdict) and
// how many said benign, among guardrail.judge events.
export const countOpinions = (events, filterName) => {
  let judged = 0;
  let benign = 0;
  (events || []).forEach((e) => {
    if (e?.event_type !== JUDGE_EVENT_TYPE || e.filter_name !== filterName) return;
    const j = judgeOf(e.metadata);
    if (!hasOpinion(j)) return;
    judged += 1;
    if (j.verdict === "benign") benign += 1;
  });
  return { judged, benign };
};

const day = (d) => d.toISOString().slice(0, 10);

const GuardrailJudgeSection = ({ value, onChange, shadow = false, error = "", filterName = "" }) => {
  const judge = value || {};
  const mode = judge.mode || "off";
  const [llms, setLLMs] = useState([]);
  // The filter's recorded opinions, null until known (or unknowable: a new
  // filter, no compliance access, Community Edition).
  const [opinions, setOpinions] = useState(null);
  const [loadError, setLoadError] = useState("");
  const [expanded, setExpanded] = useState(mode !== "off" || !!error);

  useEffect(() => {
    if (!expanded) return undefined;
    let cancelled = false;
    listAll(apiClient, "/llms")
      .then((response) => {
        if (!cancelled) setLLMs(chatCapableLLMs(response.data.data).filter((l) => l.attributes?.active !== false));
      })
      .catch(() => {
        if (!cancelled) setLoadError("Failed to load the LLM list");
      });
    return () => {
      cancelled = true;
    };
  }, [expanded]);

  useEffect(() => {
    if (error) setExpanded(true);
  }, [error]);

  useEffect(() => {
    if (!expanded || !filterName) return undefined;
    let cancelled = false;
    const now = new Date();
    const from = new Date(now.getTime() - OPINION_DAYS * 24 * 3600 * 1000);
    apiClient
      .get("/compliance/events", {
        params: { start_date: day(from), end_date: day(now), event_type: JUDGE_EVENT_TYPE, limit: 500, offset: 0 },
      })
      .then((response) => {
        if (!cancelled) setOpinions(countOpinions(response?.data?.events, filterName));
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [expanded, filterName]);

  const softenAllowed = !shadow && (mode === "soften" || (opinions && opinions.judged >= MIN_JUDGE_OPINIONS));

  const set = (patch) => {
    const next = { ...judge, ...patch };
    if (!next.mode || next.mode === "off") {
      onChange(undefined);
      return;
    }
    Object.keys(next).forEach((k) => {
      if (next[k] === undefined || next[k] === "") delete next[k];
    });
    onChange(next);
  };

  const pickLLM = (id) => {
    const row = llms.find((l) => String(l.id) === String(id));
    set({ llm_id: id ? Number(id) : undefined, model: row?.attributes?.default_model || undefined });
  };

  const errs = judgeErrors(judge);
  const on = mode !== "off";

  return (
    <Accordion
      disableGutters
      variant="outlined"
      expanded={expanded}
      onChange={(_, open) => setExpanded(open)}
      data-testid="guardrail-judge"
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />} aria-controls="guardrail-judge-content">
        <Typography variant="subtitle2">
          Second opinion (LLM judge){on ? `: ${JUDGE_MODES.find((m) => m.value === mode)?.label}` : ""}
        </Typography>
      </AccordionSummary>
      <AccordionDetails id="guardrail-judge-content">
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          An LLM is asked whether what this filter blocks or quarantines is in fact harmless for the app that sent it
          (for example a code review quoting an attack). Record only stores its opinion beside each verdict and changes
          nothing: the agreement it reaches in Filter Events is the evidence to look at before letting it soften blocks.
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="guardrail-judge-error">
            {error}
          </Alert>
        )}
        {loadError && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            {loadError}
          </Alert>
        )}
        <Grid container spacing={2}>
          <Grid item xs={12} md={4}>
            <FormControl fullWidth>
              <InputLabel id="guardrail-judge-mode-label">Mode</InputLabel>
              <Select
                labelId="guardrail-judge-mode-label"
                label="Mode"
                value={mode}
                onChange={(e) => set({ mode: e.target.value })}
                inputProps={{ "data-testid": "guardrail-judge-mode" }}
              >
                {JUDGE_MODES.map((m) => (
                  <MenuItem key={m.value} value={m.value} disabled={m.value === "soften" && !softenAllowed}>
                    {m.label}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText data-testid="guardrail-judge-mode-help">
                {shadow
                  ? "A shadow filter changes no traffic, so its judge can only record, and has nothing to judge until the filter blocks or quarantines."
                  : opinions
                  ? `${opinions.judged} opinions recorded in the last ${OPINION_DAYS} days (benign on ${opinions.benign}). Soften blocks needs at least ${MIN_JUDGE_OPINIONS}.`
                  : `Soften blocks needs at least ${MIN_JUDGE_OPINIONS} opinions recorded with Record only first.`}
              </FormHelperText>
            </FormControl>
          </Grid>
          {on && (
            <>
              <Grid item xs={12} md={4}>
                <RowSelect
                  id="guardrail-judge-llm"
                  label="Judge LLM"
                  value={llms.some((l) => String(l.id) === String(judge.llm_id)) ? judge.llm_id : ""}
                  onChange={pickLLM}
                  rows={llms}
                  error={errs.llm_id}
                  helperText={
                    judge.llm_id && llms.length > 0 && !llms.some((l) => String(l.id) === String(judge.llm_id))
                      ? `LLM #${judge.llm_id} is no longer an active chat LLM; choose another.`
                      : "A chat LLM with a privacy score at least that of the LLMs this filter guards."
                  }
                />
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField
                  fullWidth
                  label="Model"
                  value={judge.model || ""}
                  onChange={(e) => set({ model: e.target.value || undefined })}
                  placeholder="The LLM's default model"
                  InputLabelProps={{ shrink: true }}
                  helperText="Defaults to the judge LLM's default model."
                  inputProps={{ "data-testid": "guardrail-judge-model" }}
                />
              </Grid>
              {mode === "soften" && (
                <Grid item xs={12}>
                  <Alert severity="warning" data-testid="guardrail-judge-soften-warning">
                    Soften blocks lets the judge turn a block into the softest action the app's attested capability
                    allows, when it finds the flagged text harmless and was shown all of it. The judge reads text an
                    attacker may have written, and the agreement figures you reviewed are only as trustworthy as the
                    gateways (including edges) that reported them. Requests wait for the judge, up to its timeout.
                  </Alert>
                </Grid>
              )}
              <Grid item xs={12}>
                <Alert severity="info">
                  The judge reads the flagged text, which may be written by an attacker. It is told to treat it as data,
                  only answers in a fixed form, and its calls are charged to the judge LLM, never to the app. It is
                  only asked about prompt-injection findings: segments with secrets, personal data, moderation or other
                  policy findings are never sent to it, and Studio chat is not judged.
                </Alert>
              </Grid>
              <Grid item xs={12} md={4}>
                <TextField
                  fullWidth
                  type="number"
                  label="Advanced: judge timeout (ms)"
                  value={judge.timeout_ms ?? ""}
                  onChange={(e) => set({ timeout_ms: e.target.value === "" ? undefined : Number(e.target.value) })}
                  placeholder={String(DEFAULT_JUDGE_TIMEOUT_MS)}
                  InputLabelProps={{ shrink: true }}
                  error={!!errs.timeout_ms}
                  helperText={errs.timeout_ms || `Empty uses ${DEFAULT_JUDGE_TIMEOUT_MS}. A judge that runs out gives no opinion.`}
                  inputProps={{ min: 0, max: MAX_JUDGE_TIMEOUT_MS, "data-testid": "guardrail-judge-timeout" }}
                />
              </Grid>
            </>
          )}
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
};

export default GuardrailJudgeSection;
