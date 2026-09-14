import React from "react";
import { Chip } from "@mui/material";

// One status per App, derived the same way everywhere in the portal. The list
// had no status at all and the detail page showed two ("Status: Inactive" in
// the App Information block and "Status: Pending approval" next to the
// credential) for the same thing, so a developer could not tell whether the
// key would work. The three values line up with the admin list (UX review
// Q3/Q4): a credential that has been approved is Active, a credential that
// still awaits an administrator is Awaiting approval, and an App that an
// administrator has switched off is Disabled whatever its credential says.
export const APP_STATUS = {
  ACTIVE: "Active",
  AWAITING_APPROVAL: "Awaiting approval",
  DISABLED: "Disabled",
};

// Colours follow admin/components/submissions/StatusChip.js so the two chips
// read as the same family.
const statusStyles = {
  [APP_STATUS.ACTIVE]: { color: "#2e7d32", bg: "#e8f5e9" },
  [APP_STATUS.AWAITING_APPROVAL]: { color: "#e65100", bg: "#fff3e0" },
  [APP_STATUS.DISABLED]: { color: "#757575", bg: "#f5f5f5" },
};

/**
 * @param {object} args
 * @param {boolean|undefined} args.isActive App's is_active flag; undefined is
 *   treated as active because the portal's detail endpoint does not return it.
 * @param {boolean|undefined} args.credentialActive credential.active
 */
export const getAppStatus = ({ isActive, credentialActive }) => {
  if (isActive === false) {
    return APP_STATUS.DISABLED;
  }
  return credentialActive ? APP_STATUS.ACTIVE : APP_STATUS.AWAITING_APPROVAL;
};

/** Convenience for API objects: `{ attributes: { is_active, credential } }`. */
export const getAppStatusFromApp = (app) =>
  getAppStatus({
    isActive: app?.attributes?.is_active,
    credentialActive: app?.attributes?.credential?.active,
  });

const AppStatusChip = ({ status, size = "small", ...rest }) => {
  const style = statusStyles[status] || statusStyles[APP_STATUS.DISABLED];
  return (
    <Chip
      label={status}
      size={size}
      data-testid="app-status"
      sx={{
        backgroundColor: style.bg,
        color: style.color,
        fontWeight: "bold",
        fontSize: "0.75rem",
      }}
      {...rest}
    />
  );
};

export default AppStatusChip;
