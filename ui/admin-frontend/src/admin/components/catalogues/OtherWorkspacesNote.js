import React from "react";
import { Typography } from "@mui/material";
import { usePermissions } from "../../context/PermissionsContext";

/**
 * OtherWorkspacesNote explains a list of linked objects that may be
 * incomplete (Workspaces E4): the server leaves out the ones in Workspaces
 * the caller cannot see, and a save keeps those links. It shows only in
 * Enterprise (rbacEnabled) for a caller without a global read of the linked
 * kind (`permission`, e.g. P.CATALOGUES_READ), who may not be seeing them all.
 */
const OtherWorkspacesNote = ({ permission, children, sx }) => {
  const { rbacEnabled, can } = usePermissions();
  if (!rbacEnabled || can(permission)) return null;
  return (
    <Typography
      variant="bodySmallDefault"
      color="text.defaultSubdued"
      component="p"
      data-testid="other-workspaces-note"
      sx={sx}
    >
      {children}
    </Typography>
  );
};

export default OtherWorkspacesNote;
