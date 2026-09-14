import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import CheckIcon from "@mui/icons-material/Check";
import { rollMyApiKey, revokeMyApiKey } from "../../admin/services/meService";

const formatLastUsed = (value) => {
  if (!value) return "never used";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "never used";
  return `last used ${date.toLocaleString()}`;
};

/**
 * "My API key" for the signed-in user. Shows whether a key exists and when it
 * was last used; "Roll key" issues a new one and shows it exactly once with a
 * copy button; "Revoke" asks for confirmation. When the deployment does not
 * allow SSO users to hold keys (`sso_api_keys_allowed` false) the policy
 * message is shown instead of the buttons.
 *
 * Props: open, onClose, attributes (the /common/me attributes), onChanged
 * (called after a successful roll or revoke so the caller can refetch).
 */
const ApiKeyDialog = ({ open, onClose, attributes = {}, onChanged }) => {
  const [newKey, setNewKey] = useState("");
  const [copied, setCopied] = useState(false);
  const [confirmRevoke, setConfirmRevoke] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // A rolled key is shown only while this dialog stays open.
  useEffect(() => {
    if (!open) {
      setNewKey("");
      setCopied(false);
      setConfirmRevoke(false);
      setError("");
      setNotice("");
    }
  }, [open]);

  const hasKey = Boolean(attributes.has_api_key) || Boolean(newKey);
  const keysAllowed = attributes.sso_api_keys_allowed !== false;
  const policyMessage =
    attributes.sso_api_keys_policy_message ||
    "API keys are not available for accounts provisioned by single sign-on. Ask an administrator if you need one.";

  const handleRoll = async () => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const key = await rollMyApiKey();
      setNewKey(key);
      setCopied(false);
      onChanged?.();
    } catch (err) {
      setError(err?.message || "The key could not be rolled.");
    } finally {
      setBusy(false);
    }
  };

  const handleRevoke = async () => {
    setBusy(true);
    setError("");
    try {
      await revokeMyApiKey();
      setNewKey("");
      setConfirmRevoke(false);
      setNotice("Your API key has been revoked. Requests using it will now be rejected.");
      onChanged?.();
    } catch (err) {
      setError(err?.message || "The key could not be revoked.");
    } finally {
      setBusy(false);
    }
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(newKey);
      setCopied(true);
    } catch (err) {
      setError("Copy failed; select the key and copy it by hand.");
    }
  };

  return (
    <Dialog open={open} onClose={busy ? undefined : onClose} maxWidth="sm" fullWidth aria-labelledby="my-api-key-title">
      <DialogTitle id="my-api-key-title">My API key</DialogTitle>
      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        <DialogContentText>
          Your personal key authenticates API and gateway requests made as you. It is separate from any app
          credentials.
        </DialogContentText>

        {error && <Alert severity="error">{error}</Alert>}
        {notice && !newKey && <Alert severity="success" data-testid="api-key-notice">{notice}</Alert>}

        {!keysAllowed ? (
          <Alert severity="info" data-testid="api-key-policy">
            {policyMessage}
          </Alert>
        ) : newKey ? (
          <Box data-testid="api-key-new">
            <Alert severity="warning" sx={{ mb: 1 }}>
              Store it now, it will not be shown again.
            </Alert>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1,
                p: 1,
                border: "1px solid",
                borderColor: "divider",
                borderRadius: 1,
                bgcolor: "action.hover",
              }}
            >
              <Typography
                component="code"
                data-testid="api-key-value"
                sx={{ fontFamily: "monospace", wordBreak: "break-all", flexGrow: 1 }}
              >
                {newKey}
              </Typography>
              <Tooltip title={copied ? "Copied" : "Copy key"}>
                <IconButton onClick={handleCopy} size="small" aria-label="Copy API key">
                  {copied ? <CheckIcon fontSize="small" color="success" /> : <ContentCopyIcon fontSize="small" />}
                </IconButton>
              </Tooltip>
            </Box>
          </Box>
        ) : (
          <Typography variant="bodyMediumDefault" data-testid="api-key-status">
            {hasKey
              ? `You have an API key (${formatLastUsed(attributes.api_key_last_used_at)}). The key itself is not stored in a readable form; roll it to get a new one.`
              : "You do not have an API key yet."}
          </Typography>
        )}

        {keysAllowed && confirmRevoke && (
          <Alert
            severity="warning"
            data-testid="api-key-revoke-confirm"
            action={
              <Box sx={{ display: "flex", gap: 1 }}>
                <Button size="small" onClick={() => setConfirmRevoke(false)} disabled={busy}>
                  Keep key
                </Button>
                <Button size="small" color="error" variant="contained" onClick={handleRevoke} disabled={busy}>
                  Revoke
                </Button>
              </Box>
            }
          >
            Revoke your API key? Anything using it stops working immediately.
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        {keysAllowed && (
          <>
            {hasKey && !confirmRevoke && (
              <Button color="error" onClick={() => setConfirmRevoke(true)} disabled={busy} data-testid="api-key-revoke">
                Revoke
              </Button>
            )}
            <Button variant="outlined" onClick={handleRoll} disabled={busy} data-testid="api-key-roll">
              {hasKey ? "Roll key" : "Create key"}
            </Button>
          </>
        )}
        <Button onClick={onClose} disabled={busy}>
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ApiKeyDialog;
