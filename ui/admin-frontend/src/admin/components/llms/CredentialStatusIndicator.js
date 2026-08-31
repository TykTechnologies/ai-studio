import React from "react";
import { Box, Tooltip, Typography } from "@mui/material";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import WarningAmberIcon from "@mui/icons-material/WarningAmber";
import KeyIcon from "@mui/icons-material/Key";
import LockIcon from "@mui/icons-material/Lock";

/**
 * Shows whether a provider's credential can actually be used.
 *
 * A brand-new instance bootstraps OPENAI_KEY and ANTHROPIC_KEY as secrets with
 * *empty values*, then seeds providers pointing at them with active: true. The
 * list rendered a green "Proxied" dot regardless, so the instance looked fully
 * configured, the first real call failed, and nothing in the UI pointed at the
 * dangling reference. This is the signpost that was missing.
 *
 * It also distinguishes a vault-backed key from an inline one, which nothing
 * previously did: an instance could look like it used the vault while holding
 * inline keys, and rotating the secret would silently do nothing.
 */

export const CREDENTIAL_UNSET = "unset";
export const CREDENTIAL_INLINE = "inline";
export const CREDENTIAL_SECRET = "secret";
export const CREDENTIAL_UNRESOLVED = "unresolved";

export const credentialPresentation = (status, reference) => {
  switch (status) {
    case CREDENTIAL_UNRESOLVED:
      return {
        severity: "warning",
        icon: WarningAmberIcon,
        color: "warning.main",
        label: "Credential unresolved",
        detail: reference
          ? `This provider points at the secret ${reference}, which has no value. Calls through it will fail until you set it.`
          : "This provider points at a secret that has no value. Calls through it will fail until you set it.",
      };
    case CREDENTIAL_UNSET:
      return {
        severity: "warning",
        icon: WarningAmberIcon,
        color: "warning.main",
        label: "No credential",
        detail:
          "This provider has no API key configured. Calls through it will fail.",
      };
    case CREDENTIAL_SECRET:
      return {
        severity: "ok",
        icon: LockIcon,
        color: "success.main",
        label: reference ? `Key from vault: ${reference}` : "Key from vault",
        detail: reference
          ? `The API key resolves from the secret ${reference}. Rotating that secret rotates this provider's key.`
          : "The API key resolves from a stored secret.",
      };
    case CREDENTIAL_INLINE:
    default:
      return {
        severity: "ok",
        icon: KeyIcon,
        color: "success.main",
        label: "Inline key",
        detail:
          "The API key is stored directly on this provider, not in the vault. Rotating a vault secret will not change it.",
      };
  }
};

/**
 * Compact indicator for table rows: the existing active dot, plus a warning
 * marker when the credential cannot resolve.
 */
export const CredentialStatusDot = ({ active, status, reference }) => {
  const presentation = credentialPresentation(status, reference);
  const showWarning = presentation.severity === "warning";

  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
      <Tooltip title={active ? "Proxied" : "Not proxied"}>
        <FiberManualRecordIcon
          sx={{ color: active ? "green" : "red" }}
          titleAccess={active ? "Proxied" : "Not proxied"}
        />
      </Tooltip>
      {showWarning && (
        <Tooltip title={presentation.detail}>
          <WarningAmberIcon
            fontSize="small"
            sx={{ color: presentation.color }}
            titleAccess={presentation.label}
          />
        </Tooltip>
      )}
    </Box>
  );
};

/**
 * Fuller statement for the provider detail page.
 */
export const CredentialStatusNotice = ({ status, reference }) => {
  const presentation = credentialPresentation(status, reference);
  const Icon = presentation.icon;

  return (
    <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
      <Icon fontSize="small" sx={{ color: presentation.color, mt: 0.25 }} />
      <Box>
        <Typography variant="body2" sx={{ color: presentation.color }}>
          {presentation.label}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {presentation.detail}
        </Typography>
      </Box>
    </Box>
  );
};

export default CredentialStatusDot;
