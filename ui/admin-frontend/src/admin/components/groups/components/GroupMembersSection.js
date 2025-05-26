import React from "react";
import CollapsibleSection from "../../common/CollapsibleSection";
import TransferList from "../../common/transfer-list/TransferList";
import { TEAM_MEMBERS_TRANSFER_LIST_COLUMNS } from "../../../pages/groups/utils/transferListConfig";

const GroupMembersSection = ({
  availableUsers,
  selectedUsers,
  handleUsersChange,
  handleSearch,
  handleLoadMore,
  currentPage,
  totalPages,
  isLoadingMore,
  onUserAdded,
  onUserRemoved,
}) => {  
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
        onChange={handleUsersChange}
        enableSearch={true}
        onSearch={(term) => handleSearch(term, 1)}
        onLoadMore={handleLoadMore}
        hasMore={currentPage < totalPages}
        isLoadingMore={isLoadingMore}
        onItemAdded={onUserAdded}
        onItemRemoved={onUserRemoved}
      />
    </CollapsibleSection>
  );
};

export default GroupMembersSection;