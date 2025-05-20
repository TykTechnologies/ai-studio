import React from "react";
import CollapsibleSection from "../../common/CollapsibleSection";
import TransferList from "../../common/transfer-list/TransferList";
import { Box, Typography } from "@mui/material";
import CustomSelectBadge from "../../common/CustomSelectBadge";
import { roleBadgeConfigs } from "../utils/roleBadgeConfig";

const GroupMembersSection = ({
  availableUsers,
  selectedUsers,
  handleUsersChange,
  handleSearch,
  handleLoadMore,
  currentPage,
  totalPages,
  isLoadingMore,
  onLoadMoreSelected,
  hasMoreSelected,
  isLoadingMoreSelected
}) => {
  return (
    <CollapsibleSection title="Manage team members" defaultExpanded={false}>
      <TransferList
        availableItems={availableUsers}
        selectedItems={selectedUsers}
        columns={[
          {
            field: "name",
            headerName: "Name",
            width: { md: '35%', lg: '40%' },
            renderCell: (item) => (
              <Box sx={{
                display: 'flex',
                flexDirection: 'column',
                width: '100%',
                pr: 1
              }}>
                <Typography
                  variant="bodyMediumMedium"
                  color="text.defaultSubdued"
                  sx={{
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    width: '100%'
                  }}
                >
                  {item.attributes?.name}
                </Typography>
                <Typography
                  variant="bodySmallDefault"
                  color="text.defaultSubdued"
                  sx={{
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    width: '100%'
                  }}
                >
                  {item.attributes?.email}
                </Typography>
              </Box>
            )
          },
          {
            field: "role",
            headerName: "Role",
            width: { md: '45%', lg: '35%' },
            renderCell: (item) => (
              <CustomSelectBadge config={roleBadgeConfigs[item.attributes?.role] || roleBadgeConfigs["Chat user"]} />
            )
          }
        ]}
        leftTitle="Current members"
        leftSubtitle="Users currently on this team"
        rightTitle="Add members"
        rightSubtitle="Add users to this team"
        onChange={handleUsersChange}
        enableSearch={true}
        onSearch={(term) => handleSearch(term, 1)}
        onLoadMore={handleLoadMore}
        hasMore={currentPage < totalPages}
        isLoadingMore={isLoadingMore}
        onLoadMoreSelected={onLoadMoreSelected}
        hasMoreSelected={hasMoreSelected}
        isLoadingMoreSelected={isLoadingMoreSelected}
      />
    </CollapsibleSection>
  );
};

export default GroupMembersSection;