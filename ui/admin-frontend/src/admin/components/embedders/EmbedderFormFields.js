import React, { useState } from "react";
import {
  Alert,
  FormControl,
  FormHelperText,
  Grid,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from "@mui/material";
import Visibility from "@mui/icons-material/Visibility";
import VisibilityOff from "@mui/icons-material/VisibilityOff";
import PrivacyLevelInput from "../common/privacy/PrivacyLevelInput";
import { getEmbedderDefaultModel, getEmbedderDefaultUrl, getEmbedderName } from "../../utils/vendorUtils";
import { MODE_LINKED, MODE_STANDALONE, endpointField } from "./embedderModel";

/**
 * The fields of an embedder, controlled by the caller. Used by the embedder
 * page and by the inline create dialog, so both ask the same questions.
 *
 * - `llms`: JSON:API LLM rows; only those whose vendor can embed are offered.
 * - `vendors`: vendor codes that can embed (GET /embedders/vendors).
 * - `lockedReason`: set when datasources use the embedder; the model and
 *   API compatibility are then read-only (the server refuses the change too).
 */
const EmbedderFormFields = ({ draft, onChange, errors = {}, llms = [], vendors = [], lockedReason = "", idPrefix = "embedder" }) => {
  const [showKey, setShowKey] = useState(false);
  const set = (field) => (e) => onChange({ ...draft, [field]: e.target.value });
  const embeddingLLMs = llms.filter(
    (llm) => vendors.includes(llm.attributes?.vendor) || String(llm.id) === String(draft.llm_id),
  );
  const locked = Boolean(lockedReason);
  const endpoint = endpointField(draft.vendor);

  const setMode = (_e, mode) => {
    if (!mode || mode === draft.mode) return;
    // Clear what belongs to the other mode so it is not sent by accident.
    onChange({ ...draft, mode, llm_id: "", vendor: "", endpoint: "", api_key: "" });
  };

  const setVendor = (e) => {
    const vendor = e.target.value;
    const oldModel = getEmbedderDefaultModel(draft.vendor);
    const oldUrl = getEmbedderDefaultUrl(draft.vendor);
    onChange({
      ...draft,
      vendor,
      // Fill defaults unless the user already typed their own.
      model: !draft.model || draft.model === oldModel ? getEmbedderDefaultModel(vendor) : draft.model,
      endpoint: !draft.endpoint || draft.endpoint === oldUrl ? getEmbedderDefaultUrl(vendor) : draft.endpoint,
    });
  };

  return (
    <Grid container spacing={2}>
      <Grid item xs={12} md={6}>
        <TextField
          fullWidth
          required
          label="Name"
          value={draft.name}
          onChange={set("name")}
          error={Boolean(errors.name)}
          helperText={errors.name}
          inputProps={{ "data-testid": `${idPrefix}-name` }}
        />
      </Grid>
      <Grid item xs={12} md={6}>
        <TextField fullWidth label="Description" value={draft.description} onChange={set("description")} />
      </Grid>

      <Grid item xs={12}>
        <Typography variant="subtitle2" gutterBottom>
          Connection
        </Typography>
        <ToggleButtonGroup
          exclusive
          size="small"
          value={draft.mode}
          onChange={setMode}
          disabled={locked}
          aria-label="Connection mode"
        >
          <ToggleButton value={MODE_LINKED}>Use an LLM provider</ToggleButton>
          <ToggleButton value={MODE_STANDALONE}>Standalone</ToggleButton>
        </ToggleButtonGroup>
        <FormHelperText>
          {draft.mode === MODE_LINKED
            ? "Reuses the provider's vendor, endpoint, API key and privacy level. Changes to the provider apply here too."
            : "Its own client, endpoint and key: for a dedicated embedding service, such as a local model behind an OpenAI-compatible API."}
        </FormHelperText>
      </Grid>

      {locked && (
        <Grid item xs={12}>
          <Alert severity="info">{lockedReason}</Alert>
        </Grid>
      )}

      {draft.mode === MODE_LINKED ? (
        <Grid item xs={12} md={6}>
          <FormControl fullWidth required error={Boolean(errors.llm_id)} disabled={locked}>
            <InputLabel id={`${idPrefix}-llm-label`}>LLM provider</InputLabel>
            <Select
              labelId={`${idPrefix}-llm-label`}
              label="LLM provider"
              value={draft.llm_id ? String(draft.llm_id) : ""}
              onChange={set("llm_id")}
            >
              {embeddingLLMs.map((llm) => (
                <MenuItem key={llm.id} value={String(llm.id)}>
                  {llm.attributes?.name} ({getEmbedderName(llm.attributes?.vendor)})
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>
              {errors.llm_id ||
                (embeddingLLMs.length === 0
                  ? "No LLM provider uses a vendor that serves embeddings. Use a standalone embedder instead."
                  : "Only providers whose vendor serves embeddings are listed.")}
            </FormHelperText>
          </FormControl>
        </Grid>
      ) : (
        <>
          <Grid item xs={12} md={6}>
            <FormControl fullWidth required error={Boolean(errors.vendor)} disabled={locked}>
              <InputLabel id={`${idPrefix}-vendor-label`}>API compatibility</InputLabel>
              <Select
                labelId={`${idPrefix}-vendor-label`}
                label="API compatibility"
                value={draft.vendor}
                onChange={setVendor}
              >
                {vendors.map((code) => (
                  <MenuItem key={code} value={code}>
                    {getEmbedderName(code)}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText>
                {errors.vendor || "The client AI Studio uses to call the endpoint, not necessarily who serves the model."}
              </FormHelperText>
            </FormControl>
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label={endpoint.label}
              placeholder={endpoint.placeholder}
              value={draft.endpoint}
              onChange={set("endpoint")}
              helperText={endpoint.helper}
            />
          </Grid>
          <Grid item xs={12}>
            <TextField
              fullWidth
              label="API key"
              type={showKey ? "text" : "password"}
              value={draft.api_key}
              onChange={set("api_key")}
              helperText="Enter the key, or reference a stored secret as $SECRET/name or an environment variable as $ENV/NAME."
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton aria-label={showKey ? "Hide key" : "Show key"} onClick={() => setShowKey(!showKey)} edge="end">
                      {showKey ? <VisibilityOff /> : <Visibility />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
          </Grid>
        </>
      )}

      <Grid item xs={12} md={6}>
        <TextField
          fullWidth
          required
          label="Model"
          value={draft.model}
          onChange={set("model")}
          disabled={locked}
          error={Boolean(errors.model)}
          helperText={errors.model || "The embedding model, e.g. text-embedding-3-small"}
          inputProps={{ "data-testid": `${idPrefix}-model` }}
        />
      </Grid>

      {draft.mode === MODE_STANDALONE && (
        <Grid item xs={12}>
          <PrivacyLevelInput
            value={draft.privacy_score}
            onChange={(score) => onChange({ ...draft, privacy_score: score })}
            error={Boolean(errors.privacy_score)}
            helperText={
              errors.privacy_score ||
              "The embedder sees the text it embeds: data sources with a higher privacy level cannot use it."
            }
          />
        </Grid>
      )}
    </Grid>
  );
};

export default EmbedderFormFields;
