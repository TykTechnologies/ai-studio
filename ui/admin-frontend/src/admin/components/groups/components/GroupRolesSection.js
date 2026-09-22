import React from "react";
import { Alert, Box, Typography } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RoleSelect from "../../roles/RoleSelect";

export const DEFAULT_TEAM_ROLE_WARNING =
  "Every new user joins the Default team automatically. Any role you attach here is granted to all users, including the Administration tab if the role carries admin permissions.";

/**
 * Role assignment for a team (Enterprise). Every member of the team inherits
 * the selected roles on top of any roles assigned to them directly.
 *
 * `isDefaultTeam` flags the built-in Default team, which every new user joins
 * automatically: a role bound to it is effectively granted to everyone, so the
 * picker carries a warning.
 */
const GroupRolesSection = ({ value, onChange, isDefaultTeam = false }) => (
  <CollapsibleSection title="Roles" defaultExpanded>
    <Box sx={{ px: 2, pb: 2 }} data-testid="group-roles-section">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Every member of this team inherits these roles in the administration UI and API.
      </Typography>
      {isDefaultTeam && (
        <Alert severity="warning" sx={{ mb: 2 }} data-testid="default-team-role-warning">
          {DEFAULT_TEAM_ROLE_WARNING}
        </Alert>
      )}
      <RoleSelect value={value} onChange={onChange} label="Team roles" id="group-role-select" />
    </Box>
  </CollapsibleSection>
);

export default GroupRolesSection;
