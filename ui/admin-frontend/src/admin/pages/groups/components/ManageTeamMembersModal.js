import React, { useState, useMemo } from "react";
import ActionModal from "../../../components/common/ActionModal";
import RelationshipPicker from "../../../components/common/relationship-picker";
import { Box, CircularProgress } from "@mui/material";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../utils/transferListConfig";
import { teamsService } from "../../../services/teamsService";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import {
  createTeamMembersSource,
  teamMemberName,
  teamMemberEmail,
} from "../../../components/groups/utils/teamMembersSource";

const ManageTeamMembersModal = ({
  open,
  onClose,
  group,
  onSuccess,
  onError
}) => {
  const [saving, setSaving] = useState(false);

  // Current members come from the server once; from then on the picker is the
  // source of truth and Save sends whatever it holds.
  const {
    members: selectedUsers,
    setMembers: setSelectedUsers,
    loading: membersLoading,
  } = useTransferListSelectedUsers({ groupId: group?.id });

  const source = useMemo(() => createTeamMembersSource(group?.id), [group?.id]);

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

  const isLoading = membersLoading;

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
        <RelationshipPicker
          variant="dual"
          label="Team members"
          itemLabel="user"
          value={selectedUsers}
          onChange={setSelectedUsers}
          source={source}
          columns={TEAM_MEMBERS_TRANSFER_LIST_COLUMNS}
          getOptionLabel={teamMemberName}
          getOptionSecondary={teamMemberEmail}
          disabled={saving}
        />
      )}
    </ActionModal>
  );
};

export default ManageTeamMembersModal;
