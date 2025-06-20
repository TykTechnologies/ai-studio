import React, { useCallback, useEffect } from "react";
import CollapsibleSection from "../../common/CollapsibleSection";
import TransferList from "../../common/transfer-list/TransferList";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../../../pages/groups/utils/transferListConfig";
import { useTransferListSelectedUsers } from "../../../hooks/useTransferListSelectedUsers";
import { useTransferListAvailableUsers } from "../../../hooks/useTransferListAvailableUsers";

const GroupMembersSection = ({
  groupId,
  onSelectedUsersChange,
}) => {
  const {
    members: selectedUsers,
    addMember: addUser,
    removeMember: removeUser,
  } = useTransferListSelectedUsers({ groupId });

  const { 
    items: availableUsers, 
    isSearching, 
    hasMore, 
    isLoadingMore,
    searchTerm,
    loadMore, 
    search,
    addItem,
    removeItem
  } = useTransferListAvailableUsers({
    groupId,
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

  useEffect(() => {
    onSelectedUsersChange?.(selectedUsers);
  }, [selectedUsers, onSelectedUsersChange]);

  return (
    <CollapsibleSection title="Manage team members" defaultExpanded={false}>
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
    </CollapsibleSection>
  );
};

export default GroupMembersSection;