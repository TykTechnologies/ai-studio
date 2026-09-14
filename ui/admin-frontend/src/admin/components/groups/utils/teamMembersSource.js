import { getUsers } from "../../../services/userService";

export const TEAM_MEMBERS_PAGE_SIZE = 10;

/**
 * Paged, searchable user source for the team-members RelationshipPicker
 * (`variant="dual"`). Same query the old transfer-list hook issued: users not
 * already in `groupId`, ten per page, filtered by `search`. The picker owns
 * the debounce, the paging state and the selected/available split; this only
 * fetches one page.
 */
export const createTeamMembersSource = (groupId, pageSize = TEAM_MEMBERS_PAGE_SIZE) => ({
  search: async (term, page) => {
    const { data = [], totalPages = 0 } = await getUsers(page, {
      exclude_group_id: groupId,
      page_size: pageSize,
      search: term,
    });
    return { items: data, hasMore: page < totalPages };
  },
});

export const teamMemberName = (user) => user?.attributes?.name ?? "";
export const teamMemberEmail = (user) => user?.attributes?.email;
