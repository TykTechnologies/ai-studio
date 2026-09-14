import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupMembersSection from './GroupMembersSection';

jest.mock('@mui/styled-engine', () => require('../../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../../test-utils/mui-mocks').muiStylesMock);

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('../../common/CollapsibleSection', () => require('../../../../test-utils/component-mocks').collapsibleSectionMock);
jest.mock('../../common/CustomSelectBadge', () => require('../../../../test-utils/component-mocks').customSelectBadgeMock);
jest.mock('../../common/relationship-picker', () => require('../../../../test-utils/component-mocks').relationshipPickerMock);

const mockSetMembers = jest.fn();
const mockMembers = [{ id: '3', attributes: { name: 'Bob Johnson', email: 'bob@example.com', role: 'Chat user' } }];

jest.mock('../../../hooks/useTransferListSelectedUsers', () => ({
  useTransferListSelectedUsers: () => ({
    members: mockMembers,
    setMembers: mockSetMembers,
    addMember: jest.fn(),
    removeMember: jest.fn()
  })
}));

const mockGetUsers = jest.fn();
jest.mock('../../../services/userService', () => ({
  getUsers: (...args) => mockGetUsers(...args)
}));

jest.mock('../../../pages/groups/utils/transferListConfig', () => ({
  TEAM_MEMBERS_TRANSFER_LIST_COLUMNS: [
    { field: 'attributes.name', headerName: 'Name', width: '40%' },
    { field: 'attributes.email', headerName: 'Email', width: '35%' },
    { field: 'attributes.role', headerName: 'Role', width: '25%' }
  ]
}));

jest.mock('../utils/roleBadgeConfig', () => ({
  roleBadgeConfigs: {
    'Admin': { color: 'primary', label: 'Admin' },
    'Developer': { color: 'secondary', label: 'Developer' },
    'Chat user': { color: 'info', label: 'Chat user' }
  }
}));

const pickerMock = require('../../../../test-utils/component-mocks').relationshipPickerMock;

describe('GroupMembersSection Component', () => {
  const mockOnSelectedUsersChange = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    pickerMock.clearLastProps();
    mockGetUsers.mockResolvedValue({ data: [], totalPages: 0 });
  });

  test('renders a dual RelationshipPicker for users, driven by the members hook', () => {
    render(
      <GroupMembersSection
        groupId="group-123"
        onSelectedUsersChange={mockOnSelectedUsersChange}
      />
    );

    const picker = screen.getByTestId('relationship-picker');
    expect(picker).toHaveAttribute('data-variant', 'dual');
    expect(picker).toHaveAttribute('data-item-label', 'user');
    expect(picker).toHaveAttribute('aria-label', 'Team members');

    const lastProps = pickerMock.getLastProps();
    expect(lastProps.value).toEqual(mockMembers);
    expect(lastProps.onChange).toBe(mockSetMembers);
    expect(lastProps.columns).toBeDefined();
    expect(lastProps.getOptionLabel(mockMembers[0])).toBe('Bob Johnson');
    expect(lastProps.getOptionSecondary(mockMembers[0])).toBe('bob@example.com');
  });

  test('feeds the picker a paged source that excludes the team and searches', async () => {
    mockGetUsers.mockResolvedValue({
      data: [{ id: '1', attributes: { name: 'John Doe' } }],
      totalPages: 3
    });

    render(<GroupMembersSection groupId="group-123" />);

    const { source } = pickerMock.getLastProps();
    const result = await source.search('jo', 2);

    expect(mockGetUsers).toHaveBeenCalledWith(2, {
      exclude_group_id: 'group-123',
      page_size: 10,
      search: 'jo'
    });
    expect(result).toEqual({
      items: [{ id: '1', attributes: { name: 'John Doe' } }],
      hasMore: true
    });
  });

  test('renders CollapsibleSection with correct title', () => {
    render(
      <GroupMembersSection
        groupId="group-123"
        onSelectedUsersChange={mockOnSelectedUsersChange}
      />
    );

    const collapsibleSection = screen.getByTestId('collapsible-section');
    expect(collapsibleSection).toHaveAttribute('data-title', 'Manage team members');
    expect(collapsibleSection).toHaveAttribute('data-default-expanded', 'false');
  });

  test('reports the current members to the form and forwards picker changes', () => {
    render(
      <GroupMembersSection
        groupId="group-123"
        onSelectedUsersChange={mockOnSelectedUsersChange}
      />
    );

    expect(mockOnSelectedUsersChange).toHaveBeenCalledWith(mockMembers);

    fireEvent.click(screen.getByTestId('relationship-picker-remove'));
    expect(mockSetMembers).toHaveBeenCalledWith([]);
  });
});
