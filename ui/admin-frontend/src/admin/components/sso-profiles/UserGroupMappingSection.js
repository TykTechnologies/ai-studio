import React from "react";
import { Box, Typography, Alert } from "@mui/material";
import DataTable from "../common/DataTable";
import {
  TwoColumnLayout,
  FieldGroup,
  FieldLabel,
  FieldValue,
  BreakableFieldValue,
} from "./styles";

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
        User team mapping is how you assign users to AI studio teams after Single Sign-On.
        If you don't specify a user team mapping,
        users will be automatically assigned to the default team.
      </Typography>

      {groupsError && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {groupsError}
        </Alert>
      )}

      <Box sx={{ py: 3, borderBottom: "1px solid", borderColor: "border.neutralDefaultSubdued" }}>
        <TwoColumnLayout>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ width: '40%' }}>Default user team</FieldLabel>
            <FieldValue variant="bodyLargeDefault" ml={1}>
              {getGroupNameById(profileData.DefaultUserGroupID) || "Default team"}
            </FieldValue>
          </FieldGroup>
          <FieldGroup>
            <FieldLabel variant="bodyLargeBold" sx={{ minWidth: '40%', width: '40%' }}>
              Custom user team claim name
            </FieldLabel>
            <BreakableFieldValue variant="bodyLargeDefault" ml={1}>
              {profileData.CustomUserGroupField || "group"}
            </BreakableFieldValue>
          </FieldGroup>
        </TwoColumnLayout>
      </Box>

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