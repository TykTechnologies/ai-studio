import React from "react";
import { Box, Typography } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RoleSelect from "../../roles/RoleSelect";

/**
 * Role assignment for a team (Enterprise). Every member of the team inherits
 * the selected roles on top of any roles assigned to them directly.
 */
const GroupRolesSection = ({ value, onChange }) => (
  <CollapsibleSection title="Roles" defaultExpanded>
    <Box sx={{ px: 2, pb: 2 }} data-testid="group-roles-section">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Every member of this team inherits these roles in the administration UI and API.
      </Typography>
      <RoleSelect value={value} onChange={onChange} label="Team roles" id="group-role-select" />
    </Box>
  </CollapsibleSection>
);

export default GroupRolesSection;
