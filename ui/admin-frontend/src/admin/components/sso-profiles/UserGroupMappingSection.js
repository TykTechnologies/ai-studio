import React from "react";
import { Box, Typography, Stack, Alert } from "@mui/material";
import DataTable from "../common/DataTable";

/**
 * Component for displaying the user group mapping section
 * 
 * @param {Object} props - Component props
 * @param {Object} props.profileData - The profile data to display
 * @param {Array} props.groups - The list of available groups
 * @param {string} props.groupsError - Error message for groups loading
 * @param {Function} props.getGroupNameById - Function to get group name by ID
 * @returns {React.ReactElement}
 */
const UserGroupMappingSection = ({ profileData, groups, groupsError, getGroupNameById }) => {
  // Prepare data for the user group mapping table
  const prepareUserGroupMappingData = () => {
    if (!profileData?.UserGroupMapping) {
      return [];
    }

    return Object.entries(profileData.UserGroupMapping).map(([providerGroupId, tykGroupId], index) => ({
      id: index.toString(),
      providerGroupId,
      tykGroupId,
      tykGroupName: getGroupNameById(tykGroupId),
    }));
  };

  // Define columns for the user group mapping table
  const userGroupMappingColumns = [
    {
      field: "providerGroupId",
      headerName: "Identity Provider group ID",
      sortable: false,
      renderCell: (item) => item.providerGroupId,
    },
    {
      field: "tykGroupName",
      headerName: "Tyk AI studio team",
      sortable: false,
      renderCell: (item) => item.tykGroupName || item.tykGroupId,
    },
  ];

  return (
    <>
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mb: 3 }}>
        User group mapping is how you assign users to AI studio teams after Single Sign-On. 
        If you don't specify a user group mapping, 
        users will be automatically assigned to the default team.
      </Typography>

      {groupsError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {groupsError}
        </Alert>
      )}

      <Stack spacing={2} sx={{ py: 3, borderBottom: "1px solid", borderColor: "border.neutralDefaultSubdued" }}>
        <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center" }}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Default user group
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {getGroupNameById(profileData.DefaultUserGroupID) || "Default group"}
              </Typography>
            </Box>
          </Box>
          
          <Box sx={{ width: { xs: '100%', md: '50%' }, display: 'flex', alignItems: "center"}}>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Custom user group claim name
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '50%', md: '50%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {profileData.CustomUserGroupField || "group"}
              </Typography>
            </Box>
          </Box>
        </Stack>
      </Stack>

      {/* User Group Mapping Table */}
      {Object.keys(profileData.UserGroupMapping || {}).length > 0 ? (
        <Box sx={{ mt: 3 }}>
          <DataTable
            columns={userGroupMappingColumns}
            data={prepareUserGroupMappingData()}
            actions={[]}
          />
        </Box>
      ) : null}
    </>
  );
};

export default UserGroupMappingSection;