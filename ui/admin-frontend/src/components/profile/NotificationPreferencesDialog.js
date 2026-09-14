import React, { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControlLabel,
  FormGroup,
  Switch,
  Typography,
} from "@mui/material";
import { getMyPreferences, updateMyPreferences } from "../../admin/services/meService";

const readPreferences = (source = {}) => ({
  notifications_enabled: source.notifications_enabled !== false,
  email_notifications_enabled: source.email_notifications_enabled !== false,
});

/**
 * Notification preferences for the signed-in user: two switches, in-app and
 * email. Each toggle PATCHes /common/me/preferences immediately.
 *
 * Props: open, onClose, attributes (the /common/me attributes, used as the
 * initial values until the preferences endpoint answers), onChanged.
 */
const NotificationPreferencesDialog = ({ open, onClose, attributes = {}, onChanged }) => {
  const [prefs, setPrefs] = useState(() => readPreferences(attributes));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return undefined;
    let cancelled = false;
    setError("");
    setPrefs(readPreferences(attributes));
    getMyPreferences()
      .then((fresh) => {
        if (!cancelled && fresh) setPrefs(readPreferences({ ...attributes, ...fresh }));
      })
      .catch(() => {
        // The /common/me values are already on screen; leave them.
      });
    return () => {
      cancelled = true;
    };
    // attributes is a fresh object on every parent render; only reload on open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const handleToggle = (key) => async (event) => {
    const next = { ...prefs, [key]: event.target.checked };
    const previous = prefs;
    setPrefs(next);
    setSaving(true);
    setError("");
    try {
      const saved = await updateMyPreferences({ [key]: next[key] });
      if (saved && typeof saved === "object") setPrefs(readPreferences({ ...next, ...saved }));
      onChanged?.(next);
    } catch (err) {
      setPrefs(previous);
      setError(err?.message || "The preference could not be saved.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth aria-labelledby="notification-preferences-title">
      <DialogTitle id="notification-preferences-title">Notification preferences</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>
          Choose how you hear about approvals, access requests and other activity. Changes are saved immediately.
        </DialogContentText>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <FormGroup>
          <FormControlLabel
            control={
              <Switch
                checked={prefs.notifications_enabled}
                onChange={handleToggle("notifications_enabled")}
                disabled={saving}
                inputProps={{ "data-testid": "pref-inapp" }}
              />
            }
            label="In-app notifications"
          />
          <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ ml: 6, mb: 1 }}>
            Shown under the bell in the top bar.
          </Typography>
          <FormControlLabel
            control={
              <Switch
                checked={prefs.email_notifications_enabled}
                onChange={handleToggle("email_notifications_enabled")}
                disabled={saving}
                inputProps={{ "data-testid": "pref-email" }}
              />
            }
            label="Email notifications"
          />
          <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ ml: 6 }}>
            A copy of each notification is sent to your email address.
          </Typography>
        </FormGroup>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Done</Button>
      </DialogActions>
    </Dialog>
  );
};

export default NotificationPreferencesDialog;
