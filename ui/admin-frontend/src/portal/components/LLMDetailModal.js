import React from "react";
import { Box, Typography, Chip, IconButton, Button, Tooltip } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import DetailModal from "./DetailModal";
import GovernedMetadataBadges from "./GovernedMetadataBadges";
import { getVendorName, getVendorLogo } from "../../admin/utils/vendorLogos";
import { generateSlug } from "../../admin/components/wizards/quick-start/utils";
import { getConfig } from "../../config";
import { PrimaryButton } from "../../admin/styles/sharedStyles";
import PrivacyLevelChip from "../../admin/components/common/privacy/PrivacyLevelChip";

// The "More" modal on a portal LLM card. It used to show the heading and the
// vendor and nothing else (UX review F-22 / Q16), so a developer deciding
// whether to build on an LLM had to create an App first to learn which
// models it serves, how private it is, or what URL to call. Everything the
// catalogue entry carries that helps that decision is shown here, with the
// OpenAI-compatible base URL ready to copy and the Build App action to hand.

const FieldLabel = ({ children }) => (
  <Typography variant="subtitle2" color="text.secondary">
    {children}
  </Typography>
);

/**
 * The OpenAI-compatible base URL for an LLM slug: `<proxyURL>/ai/<slug>/v1`.
 * proxyURL comes from /auth/config; the fallback mirrors AppDetailView so the
 * two pages never disagree about the host.
 */
export const openAICompatibleBaseUrl = (llmName) => {
  const config = getConfig();
  const proxyUrl =
    config.proxyURL || `${window.location.protocol}//${window.location.hostname}:9090`;
  return `${proxyUrl.replace(/\/+$/, "")}/ai/${generateSlug(llmName)}/v1`;
};

const LLMDetailModal = ({ llm, open, handleClose, onBuildApp }) => {
  if (!llm) return null;
  const attrs = llm.attributes || {};
  const allowedModels = Array.isArray(attrs.allowed_models) ? attrs.allowed_models : [];
  const baseUrl = openAICompatibleBaseUrl(attrs.name || "");
  const hasPrivacy = typeof attrs.privacy_score === "number";

  const copyBaseUrl = () => {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(baseUrl).catch(() => undefined);
    }
  };

  return (
    <DetailModal open={open} handleClose={handleClose} title={attrs.name}>
      {attrs.short_description && (
        <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
          {attrs.short_description}
        </Typography>
      )}
      {attrs.long_description && (
        <Typography variant="body1" sx={{ mt: 1 }}>
          {attrs.long_description}
        </Typography>
      )}

      <Box sx={{ display: "flex", alignItems: "center", mt: 2 }}>
        <FieldLabel>Vendor:</FieldLabel>
        <img
          src={getVendorLogo(attrs.vendor)}
          alt={getVendorName(attrs.vendor)}
          style={{
            width: 24,
            height: 24,
            marginLeft: 8,
            marginRight: 8,
            objectFit: "contain",
          }}
        />
        <Typography>{getVendorName(attrs.vendor)}</Typography>
      </Box>

      <Box sx={{ mt: 2 }}>
        <FieldLabel>Default model:</FieldLabel>
        <Typography>{attrs.default_model || "Not set"}</Typography>
      </Box>

      <Box sx={{ mt: 2 }}>
        <FieldLabel>Allowed models:</FieldLabel>
        {allowedModels.length > 0 ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 0.5 }}>
            {allowedModels.map((pattern) => (
              <Chip key={pattern} label={pattern} size="small" />
            ))}
          </Box>
        ) : (
          <Typography variant="body2">
            Any model the vendor serves (no restriction configured).
          </Typography>
        )}
      </Box>

      <Box sx={{ mt: 2 }}>
        <FieldLabel>Privacy level:</FieldLabel>
        <Box sx={{ mt: 0.5 }}>
          <PrivacyLevelChip score={hasPrivacy ? attrs.privacy_score : null} />
        </Box>
      </Box>

      <Box sx={{ mt: 2 }}>
        <FieldLabel>OpenAI-compatible base URL:</FieldLabel>
        <Box sx={{ display: "flex", alignItems: "center", mt: 0.5 }}>
          <Typography
            component="code"
            variant="body2"
            sx={{
              fontFamily: "monospace",
              bgcolor: "action.hover",
              p: 1,
              borderRadius: 1,
              flexGrow: 1,
              wordBreak: "break-all",
            }}
          >
            {baseUrl}
          </Typography>
          <Tooltip title="Copy base URL">
            <IconButton aria-label="Copy base URL" size="small" onClick={copyBaseUrl}>
              <ContentCopyIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
        <Typography variant="caption" color="text.secondary">
          Requests need an App credential; build an App to get one.
        </Typography>
      </Box>

      <GovernedMetadataBadges items={llm.governed_metadata} sx={{ mt: 2 }} />

      <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 3 }}>
        <Button onClick={handleClose}>Close</Button>
        {onBuildApp && (
          <PrimaryButton variant="contained" onClick={() => onBuildApp(llm.id)}>
            Build App
          </PrimaryButton>
        )}
      </Box>
    </DetailModal>
  );
};

export default LLMDetailModal;
