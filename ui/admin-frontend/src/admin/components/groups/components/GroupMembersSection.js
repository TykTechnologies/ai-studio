import React, { useEffect, useMemo } from "react";
import CollapsibleSection from "../../common/CollapsibleSection";
import RelationshipPicker from "../../common/relationship-picker";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../../../pages/groups/utils/transferListConfig";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import {
  createTeamMembersSource,
  teamMemberName,
  teamMemberEmail,
} from "../utils/teamMembersSource";

const GroupMembersSection = ({
  groupId,
  onSelectedUsersChange,
}) => {
  // Current members come from the server once; from then on the picker is the
  // source of truth and the form saves whatever it holds.
  const {
    members: selectedUsers,
    setMembers: setSelectedUsers,
  } = useTransferListSelectedUsers({ groupId });

  const source = useMemo(() => createTeamMembersSource(groupId), [groupId]);

  useEffect(() => {
    onSelectedUsersChange?.(selectedUsers);
  }, [selectedUsers, onSelectedUsersChange]);

  return (
    <CollapsibleSection title="Manage team members" defaultExpanded={false}>
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
      />
    </CollapsibleSection>
  );
};

export default GroupMembersSection;
