import React from "react";
import { FormControlLabel, Switch, Typography, Box } from "@mui/material";
import { usePermissions } from "../../context/PermissionsContext";

// The per-asset "Allow internal host" switch (epic w4e). Outbound calls
// refuse internal network addresses by default; on, this asset's own
// host(s) may be internal, and blocking stays on for every other host.
// Only an administrator may change it (the API answers 403 to anyone
// else), so others see it read-only. Admin console only: never in the
// portal, and submissions never carry it.
const AllowInternalHostSwitch = ({ checked, onChange, target = "this resource's host" }) => {
  const { isFullAdmin } = usePermissions();
  return (
    <Box>
      <FormControlLabel
        control={
          <Switch
            checked={Boolean(checked)}
            onChange={(e) => onChange(e.target.checked)}
            disabled={!isFullAdmin}
            inputProps={{ "aria-label": "Allow internal host" }}
          />
        }
        label="Allow internal host"
      />
      <Typography variant="body2" color="text.secondary">
        Lets {target} be an internal network address (private, loopback or
        link-local). Calls to any other internal address stay blocked.
        {!isFullAdmin && " Only an administrator can change this."}
      </Typography>
    </Box>
  );
};

export default AllowInternalHostSwitch;
