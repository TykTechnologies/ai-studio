import React, { useState } from "react";
import ActionModal from "../../../components/common/ActionModal";
import TransferList from "../../../components/common/transfer-list/TransferList";
import { Box, CircularProgress } from "@mui/material";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../utils/transferListConfig";
import { teamsService } from "../../../services/teamsService";
import { useTeamMembersModal } from "../utils/useTeamMembersModal";

const ManageTeamMembersModal = ({ 
  open, 
  onClose, 
  group, 
  onSuccess,
  onError 
}) => {
  const {
    availableUsers,
    selectedUsers,
    isLoadingMore,
    loading,
    hasMore,
    handleUsersChange,
    handleSearch,
    handleLoadMore,
    handleUserAdded,
    handleUserRemoved,
  } = useTeamMembersModal(group?.id);

  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!group) return;
    
    setSaving(true);
    try {
      const userIds = selectedUsers.map(user => parseInt(user.id, 10));
      await teamsService.updateGroupUsers(group.id, userIds);
      onSuccess(`Team members for "${group.attributes.name}" updated successfully!`);
      onClose();
    } catch (error) {
      console.error("Error updating team members:", error);
      onError("Failed to update team members. Please try again.");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <ActionModal
        open={open}
        title="Manage Team Members"
        onClose={onClose}
        onPrimaryAction={() => {}}
        onSecondaryAction={onClose}
        disabled={true}
      >
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      </ActionModal>
    );
  }

  return (
    <ActionModal
      open={open}
      title="Manage Team Members"
      onClose={onClose}
      onPrimaryAction={handleSave}
      onSecondaryAction={onClose}
      disabled={saving}
    >
      <TransferList
        availableItems={availableUsers}
        selectedItems={selectedUsers}
        columns={TEAM_MEMBERS_TRANSFER_LIST_COLUMNS}
        leftTitle="Current members"
        leftSubtitle="Users currently on this team"
        rightTitle="Add members"
        rightSubtitle="Add users to this team"
        onChange={handleUsersChange}
        enableSearch={true}
        onSearch={handleSearch}
        onLoadMore={handleLoadMore}
        hasMore={hasMore}
        isLoadingMore={isLoadingMore}
        onItemAdded={handleUserAdded}
        onItemRemoved={handleUserRemoved}
      />
    </ActionModal>
  );
};

export default ManageTeamMembersModal; 