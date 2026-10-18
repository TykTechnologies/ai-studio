import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import Section from "../common/Section";
import UsedBySection from "../common/UsedBySection";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Chip,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
} from "../../styles/sharedStyles";
import {
  DEFAULT_QUARANTINE_PLACEHOLDER,
  effectiveToolAction,
  scansToolResults,
  scansToolCalls,
  effectiveToolCallAction,
} from "./GuardrailConfigForm";

const ACTION_LABELS = { block: "Block", redact: "Redact", quarantine: "Quarantine", log: "Log only" };

// Connection values are references ($SECRET/name, $ENV/NAME) or plain
// endpoints. A value that is neither is a credential typed in directly, and
// is not shown.
const displayConnectionValue = (field, value) => {
  if (!value) return "";
  if (field.startsWith("$")) return value;
  if (value.startsWith("$SECRET/") || value.startsWith("$ENV/")) return value;
  if (/key|secret|token/i.test(field)) return "•••••• (stored directly)";
  return value;
};

const FilterDetails = () => {
  const [filter, setFilter] = useState(null);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    fetchFilterDetails();
  }, [id]);

  const fetchFilterDetails = async () => {
    try {
      const response = await apiClient.get(`/filters/${id}`);
      const filterData = response.data; // Remove .data here
      setFilter({
        ...filterData,
        attributes: {
          ...filterData.attributes,
          script: filterData.attributes.script ? atob(filterData.attributes.script) : "", // Decode base64
        },
      });
      setLoading(false);
    } catch (error) {
      console.error("Error fetching filter details", error);
      setLoading(false);
    }
  };

  if (loading) return <CircularProgress />;
  if (!filter) return <Typography>Filter not found</Typography>;

  const attrs = filter.attributes;
  const isGuardrail = attrs.kind === "guardrail";
  const config = attrs.config || {};

  const row = (label, value) => (
    <>
      <Grid item xs={3}>
        <FieldLabel>{label}:</FieldLabel>
      </Grid>
      <Grid item xs={9}>
        <FieldValue>{value}</FieldValue>
      </Grid>
    </>
  );

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">Filter details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/filters")}
          color="inherit"
        >
          Back to filters
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <Section title="Filter Information">
          <Grid container spacing={2}>
            {row("Name", attrs.name)}
            {row("Description", attrs.description)}
            {row("Kind", isGuardrail ? "Guardrail" : "Script")}
            {row("Type", attrs.response_filter ? "Response Filter" : "Request Filter")}
          </Grid>
        </Section>

        <UsedBySection resourcePath="filters" id={id} objectLabel="filter" />

        {isGuardrail ? (
          <Section title="Guardrail">
            <Grid container spacing={2}>
              {row("Provider", config.provider)}
              {row(
                "Detectors",
                <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                  {(config.detectors || []).map((d) => (
                    <Chip
                      key={d.name}
                      size="small"
                      label={d.threshold !== undefined && d.threshold !== null ? `${d.name} ≥ ${d.threshold}` : d.name}
                    />
                  ))}
                </Box>
              )}
              {config.exclude && config.exclude.length > 0 && row("Excluded patterns", config.exclude.join(", "))}
              {row(
                "On detection",
                attrs.response_filter && config.on_detect === "redact"
                  ? "Redact (logs only on responses: the response is not changed)"
                  : ACTION_LABELS[config.on_detect] || config.on_detect
              )}
              {config.on_detect === "redact" && !attrs.response_filter && config.redaction &&
                row("Redaction", `${config.redaction.style || "placeholder"}${config.redaction.placeholder ? ` (${config.redaction.placeholder})` : ""}`)}
              {!attrs.response_filter && row("Messages inspected", config.scope || "all_user")}
              {!attrs.response_filter &&
                row("Tool results", scansToolResults(config) ? "Scanned across the whole history" : "Not scanned (only under all_messages)")}
              {!attrs.response_filter && scansToolResults(config) &&
                row(
                  "Action on tool results",
                  `${ACTION_LABELS[effectiveToolAction(config)] || effectiveToolAction(config)}${
                    config.tool_results?.on_detect ? "" : " (default)"
                  }`
                )}
              {row("Tool calls", scansToolCalls(config) ? "Scanned" : "Not scanned")}
              {scansToolCalls(config) &&
                row(
                  "Action on tool calls",
                  `${ACTION_LABELS[effectiveToolCallAction(config)] || effectiveToolCallAction(config)}${
                    config.tool_calls?.on_detect ? "" : " (default)"
                  }${config.tool_calls?.include_system ? "; system prompt sent with each call" : ""}`
                )}
              {!attrs.response_filter &&
                (config.on_detect === "quarantine" || (scansToolResults(config) && effectiveToolAction(config) === "quarantine")) &&
                row("Quarantine placeholder", config.quarantine?.placeholder || DEFAULT_QUARANTINE_PLACEHOLDER)}
              {!attrs.response_filter && config.cache === false && row("Verdict cache", "Off")}
              {!attrs.response_filter && config.provider !== "builtin" && config.budget &&
                Object.keys(config.budget).length > 0 &&
                row(
                  "Budgets",
                  Object.entries(config.budget)
                    .map(([k, v]) => `${k} ${v}`)
                    .join(", ")
                )}
              {row("If the provider fails", config.fail_mode === "open" ? "Let it through" : config.fail_mode === "closed" ? "Block" : "Default")}
              {row("Timeout", `${config.timeout_ms || 2000} ms`)}
              {attrs.response_filter && config.stream?.evaluate_every_chars > 0 &&
                row("Streaming cadence", `every ${config.stream.evaluate_every_chars} characters`)}
              {row("Block message", config.block_message || "(default)")}
              {config.connection && Object.keys(config.connection).length > 0 &&
                row(
                  "Connection",
                  <Box component="dl" sx={{ m: 0 }}>
                    {Object.entries(config.connection).map(([k, v]) => (
                      <Typography key={k} variant="body2">
                        {k}: {displayConnectionValue(k, v)}
                      </Typography>
                    ))}
                  </Box>
                )}
            </Grid>
          </Section>
        ) : (
          <Section title="Script">
            <Box
              sx={{
                backgroundColor: "#f5f5f5",
                padding: 2,
                borderRadius: 1,
                whiteSpace: "pre-wrap",
                fontFamily: "monospace",
              }}
            >
              {attrs.script}
            </Box>
          </Section>
        )}

        <Box mt={4} display="flex" justifyContent="flex-end">
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/filters/edit/${id}`)}
          >
            Edit filter
          </PrimaryButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default FilterDetails;
