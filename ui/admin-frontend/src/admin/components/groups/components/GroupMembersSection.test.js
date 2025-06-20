import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupMembersSection from './GroupMembersSection';

jest.mock('@mui/styled-engine', () => require('../../../../test-utils/mui-mocks').muiStyledEngineMock);
jest.mock('@mui/material/styles', () => require('../../../../test-utils/mui-mocks').muiStylesMock);

jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('../../common/CollapsibleSection', () => require('../../../../test-utils/component-mocks').collapsibleSectionMock);
jest.mock('../../common/CustomSelectBadge', () => require('../../../../test-utils/component-mocks').customSelectBadgeMock);
jest.mock('../../common/transfer-list/TransferList', () => require('../../../../test-utils/component-mocks').transferListMock);

jest.mock('../../../hooks/useTransferListSelectedUsers', () => ({
  useTransferListSelectedUsers: ({ groupId }) => ({
    members: [{ id: '3', attributes: { name: 'Bob Johnson', email: 'bob@example.com', role: 'Chat user' } }],
    addMember: jest.fn(),
    removeMember: jest.fn()
  })
}));

jest.mock('../../../hooks/useTransferListAvailableUsers', () => ({
  useTransferListAvailableUsers: () => {
    return {
      items: [
        { id: '1', attributes: { name: 'John Doe', email: 'john@example.com', role: 'Admin' } },
        { id: '2', attributes: { name: 'Jane Smith', email: 'jane@example.com', role: 'Developer' } }
      ],
      isSearching: false,
      hasMore: true,
      isLoadingMore: false,
      searchTerm: '',
      loadMore: jest.fn(),
      search: jest.fn(),
      addItem: jest.fn(),
      removeItem: jest.fn()
    };
  }
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

describe('GroupMembersSection Component', () => {
  const mockOnSelectedUsersChange = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
    require('../../../../test-utils/component-mocks').transferListMock.clearLastProps();
  });
  
  test('renders TransferList with correct props', () => {
    render(
      <GroupMembersSection
        groupId="group-123"
        onSelectedUsersChange={mockOnSelectedUsersChange}
      />
    );
    
    const lastProps = require('../../../../test-utils/component-mocks').transferListMock.getLastProps();
    expect(lastProps.leftTitle).toBe("Current members");
    expect(lastProps.rightTitle).toBe("Add members");
    expect(lastProps.enableSearch).toBe(true);
    expect(lastProps.columns).toBeDefined();
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
  
  test('receives items from hooks and passes them to TransferList', () => {
    render(
      <GroupMembersSection
        groupId="group-123"
        onSelectedUsersChange={mockOnSelectedUsersChange}
      />
    );
    
    const expectedAvailableItems = [
      { id: '1', attributes: { name: 'John Doe', email: 'john@example.com', role: 'Admin' } },
      { id: '2', attributes: { name: 'Jane Smith', email: 'jane@example.com', role: 'Developer' } }
    ];
    
    const expectedSelectedItems = [
      { id: '3', attributes: { name: 'Bob Johnson', email: 'bob@example.com', role: 'Chat user' } }
    ];
    
    const lastProps = require('../../../../test-utils/component-mocks').transferListMock.getLastProps();
    expect(lastProps.availableItems).toEqual(expectedAvailableItems);
    expect(lastProps.selectedItems).toEqual(expectedSelectedItems);
  });
});