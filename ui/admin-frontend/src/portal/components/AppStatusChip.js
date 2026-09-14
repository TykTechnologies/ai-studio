import React from "react";
import { Chip } from "@mui/material";
import { APP_STATUS, getAppStatus, getAppStatusFromApp } from "../../utils/appStatus";

// One status per App, derived the same way everywhere in the portal. The list
// had no status at all and the detail page showed two ("Status: Inactive" in
// the App Information block and "Status: Pending approval" next to the
// credential) for the same thing, so a developer could not tell whether the
// key would work. The words and the derivation live in utils/appStatus.js and
// are shared with the admin list and detail pages (UX review Q3/Q4, M9).
export { APP_STATUS, getAppStatus, getAppStatusFromApp };

// Colours follow admin/components/submissions/StatusChip.js so the two chips
// read as the same family.
const statusStyles = {
  [APP_STATUS.ACTIVE]: { color: "#2e7d32", bg: "#e8f5e9" },
  [APP_STATUS.AWAITING_APPROVAL]: { color: "#e65100", bg: "#fff3e0" },
  [APP_STATUS.NO_CREDENTIAL]: { color: "#757575", bg: "#f5f5f5" },
  [APP_STATUS.DISABLED]: { color: "#757575", bg: "#f5f5f5" },
};

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
