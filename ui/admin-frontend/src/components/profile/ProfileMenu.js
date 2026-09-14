import React, { useCallback, useEffect, useState } from "react";
import {
  Avatar,
  Box,
  Chip,
  Divider,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from "@mui/material";
import VpnKeyIcon from "@mui/icons-material/VpnKey";
import NotificationsActiveIcon from "@mui/icons-material/NotificationsActive";
import LogoutIcon from "@mui/icons-material/Logout";
import { usePermissions } from "../../admin/context/PermissionsContext";
import { logout as defaultLogout } from "../../admin/utils/pubClient";
import { accountTypeLabel, authSourceLabel, getMe, initialsFor } from "../../admin/services/meService";
import ApiKeyDialog from "./ApiKeyDialog";
import NotificationPreferencesDialog from "./NotificationPreferencesDialog";

/**
 * The avatar button at the right of the top bar and its account menu:
 * who is signed in (name, email, account type, roles), "My API key",
 * "Notification preferences" and "Log out". Replaces the one-click logout.
 *
 * Identity comes from the app-wide /common/me fetch; opening the menu
 * refetches it so API-key state and preferences are current.
 */
const ProfileMenu = ({ onLogout = defaultLogout }) => {
  const { identity } = usePermissions();
  const [anchorEl, setAnchorEl] = useState(null);
  const [me, setMe] = useState(null);
  const [apiKeyOpen, setApiKeyOpen] = useState(false);
  const [prefsOpen, setPrefsOpen] = useState(false);
  const open = Boolean(anchorEl);

  const refetchMe = useCallback(async () => {
    try {
      const fresh = await getMe();
      setMe(fresh);
    } catch (error) {
      // The identity store still has the boot-time answer; keep using it.
    }
  }, []);

  useEffect(() => {
    if (open) refetchMe();
  }, [open, refetchMe]);

  const attributes = me?.attributes || identity?.raw?.attributes || {};
  const name = attributes.name || identity?.name || "";
  const email = attributes.email || identity?.email || "";
  const roles = Array.isArray(attributes.roles) ? attributes.roles : identity?.roles || [];
  const initials = initialsFor(name, email);
  const accountType = accountTypeLabel(attributes);
  const authSource = authSourceLabel(attributes.auth_source);

  const handleOpen = (event) => setAnchorEl(event.currentTarget);
  const handleClose = () => setAnchorEl(null);

  const openApiKey = () => {
    handleClose();
    setApiKeyOpen(true);
  };
  const openPrefs = () => {
    handleClose();
    setPrefsOpen(true);
  };
  const handleLogout = () => {
    handleClose();
    onLogout();
  };

  return (
    <>
      <Tooltip title={name ? `${name} (${email})` : "Account"}>
        <IconButton
          onClick={handleOpen}
          size="small"
          aria-label="Account menu"
          aria-haspopup="menu"
          aria-expanded={open}
          aria-controls={open ? "account-menu" : undefined}
          data-testid="account-menu-button"
          sx={{ ml: 1 }}
        >
          <Avatar sx={{ width: 32, height: 32, bgcolor: "#23E2C2", color: "#03031C", fontSize: 14, fontWeight: 600 }}>
            {initials}
          </Avatar>
        </IconButton>
      </Tooltip>
      <Menu
        id="account-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
        slotProps={{ paper: { sx: { mt: 1, minWidth: 280, maxWidth: 360 } } }}
      >
        <Box sx={{ px: 2, pt: 1, pb: 1.5 }} data-testid="account-menu-header">
          {/* The custom typography variants render as spans; force block so the lines stack. */}
          <Typography component="div" variant="bodyMediumSemiBold" noWrap>
            {name || email}
          </Typography>
          {name && (
            <Typography component="div" variant="bodySmallDefault" color="text.defaultSubdued" noWrap>
              {email}
            </Typography>
          )}
          <Typography component="div" variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mt: 0.5 }}>
            {accountType}
            {authSource ? ` · ${authSource}` : ""}
          </Typography>
          {roles.length > 0 && (
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 1 }} data-testid="account-menu-roles">
              {roles.map((role) => (
                <Chip
                  key={role.id || role.slug || role.name}
                  label={role.name || role.slug}
                  size="small"
                  variant="outlined"
                  title={role.via === "group" ? "Granted through a team" : undefined}
                />
              ))}
            </Box>
          )}
        </Box>
        <Divider />
        <MenuItem onClick={openApiKey}>
          <ListItemIcon>
            <VpnKeyIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>My API key</ListItemText>
        </MenuItem>
        <MenuItem onClick={openPrefs}>
          <ListItemIcon>
            <NotificationsActiveIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Notification preferences</ListItemText>
        </MenuItem>
        <Divider />
        <MenuItem onClick={handleLogout} data-testid="account-menu-logout">
          <ListItemIcon>
            <LogoutIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Log out</ListItemText>
        </MenuItem>
      </Menu>
      <ApiKeyDialog open={apiKeyOpen} onClose={() => setApiKeyOpen(false)} attributes={attributes} onChanged={refetchMe} />
      <NotificationPreferencesDialog
        open={prefsOpen}
        onClose={() => setPrefsOpen(false)}
        attributes={attributes}
        onChanged={refetchMe}
      />
    </>
  );
};

export default ProfileMenu;
