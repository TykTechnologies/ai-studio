import React, { useState, useCallback } from "react";
import ActionModal from "../../../components/common/ActionModal";
import TransferList from "../../../components/common/transfer-list/TransferList";
import { Box, CircularProgress } from "@mui/material";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../utils/transferListConfig";
import { teamsService } from "../../../services/teamsService";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import { useTransferListAvailableUsers } from "../../../hooks/useTransferListAvailableUsers";

const ManageTeamMembersModal = ({ 
  open, 
  onClose, 
  group, 
  onSuccess,
  onError 
}) => {
  const [saving, setSaving] = useState(false);

  const {
    members: selectedUsers,
    addMember: addUser,
    removeMember: removeUser,
    loading: membersLoading,
  } = useTransferListSelectedUsers({ groupId: group?.id });

  const { 
    items: availableUsers, 
    loading, 
    isSearching, 
    hasMore, 
    isLoadingMore,
    searchTerm,
    loadMore, 
    search,
    addItem,
    removeItem
  } = useTransferListAvailableUsers({
    groupId: group?.id,
    pageSize: 10,
    searchDebounceMs: 500,
    excludeIds: selectedUsers.map(u => u.id)
  });

  const handleSearchChange = useCallback((searchTerm) => {
    search(searchTerm);
  }, [search]);

  const handleAddUser = useCallback((user) => {
    addUser(user);
    removeItem(user);
  }, [addUser, removeItem]);

  const handleRemoveUser = useCallback((user) => {
    removeUser(user);
    addItem(user);
  }, [removeUser, addItem]);

  const handleSave = async () => {
    if (!group) return;
    
    setSaving(true);
    try {
      const userIds = selectedUsers.map(user => parseInt(user.id, 10));
      await teamsService.updateGroupUsers(group.id, userIds);
      onSuccess(`Team members for "${group.attributes.name}" updated successfully!`);
      onClose();
    } catch (error) {
      onError("Failed to update team members. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  const isLoading = membersLoading || (loading && availableUsers.length === 0);

  return (
    <ActionModal
      open={open}
      title="Manage Team Members"
      onClose={onClose}
      onPrimaryAction={isLoading ? () => {} : handleSave}
      onSecondaryAction={onClose}
      disabled={saving || isLoading}
    >
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : (
        <TransferList
          availableItems={availableUsers}
          selectedItems={selectedUsers}
          columns={TEAM_MEMBERS_TRANSFER_LIST_COLUMNS}
          leftTitle="Current members"
          leftSubtitle="Users currently on this team"
          rightTitle="Add members"
          rightSubtitle="Add users to this team"
          enableSearch={true}
          searchTerm={searchTerm}
          onSearchTermChange={handleSearchChange}
          isSearching={isSearching}
          onAdd={handleAddUser}
          onRemove={handleRemoveUser}
          onLoadMore={loadMore}
          hasMore={hasMore}
          isLoadingMore={isLoadingMore}
        />
      )}
    </ActionModal>
  );
};

export default ManageTeamMembersModal; 