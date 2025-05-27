import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import GroupsTable from './GroupsTable';
import '@testing-library/jest-dom';


jest.mock('./CatalogueBadges', () => {
  return ({ catalogues, dataCatalogues, toolCatalogues }) => {
    const allCatalogues = [...catalogues, ...dataCatalogues, ...toolCatalogues].join(',');
    return <div data-testid="mocked-catalogue-badges">{allCatalogues}</div>;
  };
});

jest.mock('../../../components/common/DataTable', () => {
  const MockedCatalogueBadges = require('./CatalogueBadges');
  return ({ columns, data, pagination, onRowClick, enableSearch, onSearch, searchPlaceholder, sortConfig, onSortChange, actions }) => {
    const mockedColumns = columns.map(col => col.headerName).join(',');
    const mockedData = data.map(row => row.id).join(',');
    const mockedActions = actions.map(action => action.label).join(',');
    return (
      <div data-testid="mocked-datatable">
        <div>Columns: {mockedColumns}</div>
        <div>Data: {mockedData}</div>
        <div>Pagination: Page {pagination.page}, Size {pagination.pageSize}, Total {pagination.totalPages}</div>
        <div>Enable Search: {enableSearch ? 'true' : 'false'}</div>
        <div>Search Placeholder: {searchPlaceholder}</div>
        <div>Sort Config: {sortConfig ? `${sortConfig.field}-${sortConfig.direction}` : 'none'}</div>
        <div>Actions: {mockedActions}</div>
        {data.map(row => (
          <div key={row.id} data-testid={`row-${row.id}`} onClick={() => onRowClick(row)}>
            <div data-testid={`row-${row.id}-name`}>{row.attributes.name}</div>
            <div data-testid={`row-${row.id}-member-count`}>{row.attributes.user_count || 0}</div>
            <MockedCatalogueBadges
              catalogues={row.attributes.catalogue_names || []}
              dataCatalogues={row.attributes.data_catalogue_names || []}
              toolCatalogues={row.attributes.tool_catalogue_names || []}
            />
            {actions.map(action => (
              <button key={action.label} data-testid={`row-${row.id}-${action.label.replace(/\s+/g, '-').toLowerCase()}`} onClick={(e) => { e.stopPropagation(); action.onClick(row); }}>
                {action.label}
              </button>
            ))}
          </div>
        ))}
        <button onClick={pagination.onPageChange}>Change Page</button>
        <button onClick={pagination.onPageSizeChange}>Change Page Size</button>
        <input data-testid="search-input" onChange={(e) => onSearch(e.target.value)} />
        <button onClick={() => onSortChange({ field: 'id', direction: 'asc' })}>Sort</button>
      </div>
    );
  };
});

describe('GroupsTable', () => {
  const mockGroups = [
    { id: '1', attributes: { name: 'Group A', user_count: 5, catalogue_names: ['Cat1'], data_catalogue_names: [], tool_catalogue_names: ['Tool1'] } },
    { id: '2', attributes: { name: 'Group B', user_count: 10, catalogue_names: [], data_catalogue_names: ['DataCat1'], tool_catalogue_names: [] } },
  ];

  const mockHandlePageChange = jest.fn();
  const mockHandlePageSizeChange = jest.fn();
  const mockHandleSearch = jest.fn();
  const mockHandleSortChange = jest.fn();
  const mockHandleGroupClick = jest.fn();
  const mockHandleEdit = jest.fn();
  const mockHandleDelete = jest.fn();

  const defaultProps = {
    groups: mockGroups,
    page: 1,
    pageSize: 10,
    totalPages: 2,
    handlePageChange: mockHandlePageChange,
    handlePageSizeChange: mockHandlePageSizeChange,
    handleSearch: mockHandleSearch,
    sortConfig: { field: 'id', direction: 'asc' },
    handleSortChange: mockHandleSortChange,
    handleGroupClick: mockHandleGroupClick,
    handleEdit: mockHandleEdit,
    handleDelete: mockHandleDelete,
  };

  test('renders DataTable with correct props', () => {
    render(<GroupsTable {...defaultProps} />);

    const dataTable = screen.getByTestId('mocked-datatable');
    expect(dataTable).toBeInTheDocument();

    expect(screen.getByText('Columns: ID,Name,Members,Catalogues')).toBeInTheDocument();
    expect(screen.getByText('Data: 1,2')).toBeInTheDocument();
    expect(screen.getByText('Pagination: Page 1, Size 10, Total 2')).toBeInTheDocument();
    expect(screen.getByText('Enable Search: true')).toBeInTheDocument();
    expect(screen.getByText('Search Placeholder: Search by name')).toBeInTheDocument();
    expect(screen.getByText('Sort Config: id-asc')).toBeInTheDocument();
    expect(screen.getByText('Actions: Edit team,Delete team')).toBeInTheDocument();
  });

  test('renders group names and member counts correctly', () => {
    render(<GroupsTable {...defaultProps} />);

    expect(screen.getByText('Group A')).toBeInTheDocument();
    expect(screen.getByText('Group B')).toBeInTheDocument();

    // Member counts are rendered within Typography, check the mocked DataTable output
    expect(screen.getByTestId('row-1-name')).toHaveTextContent('Group A');
    expect(screen.getByTestId('row-2-name')).toHaveTextContent('Group B');

    expect(screen.getByTestId('row-1-member-count')).toHaveTextContent('5');
    expect(screen.getByTestId('row-2-member-count')).toHaveTextContent('10');
  });

  test('renders CatalogueBadges with correct props', () => {
    render(<GroupsTable {...defaultProps} />);

    const row1 = screen.getByTestId('row-1');
    const row2 = screen.getByTestId('row-2');

    const row1CatalogueBadges = within(row1).getByTestId('mocked-catalogue-badges');
    const row2CatalogueBadges = within(row2).getByTestId('mocked-catalogue-badges');

    expect(row1CatalogueBadges).toHaveTextContent('Cat1,Tool1');
    expect(row2CatalogueBadges).toHaveTextContent('DataCat1');
  });

  test('calls handleGroupClick when a row is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const rowA = screen.getByTestId('row-1');
    fireEvent.click(rowA);

    expect(mockHandleGroupClick).toHaveBeenCalledTimes(1);
    expect(mockHandleGroupClick).toHaveBeenCalledWith(mockGroups[0]);
  });

  test('calls handleEdit when Edit team action is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const rowA = screen.getByTestId('row-1');
    const editButton = screen.getByTestId('row-1-edit-team');
    fireEvent.click(editButton);

    expect(mockHandleEdit).toHaveBeenCalledTimes(1);
    expect(mockHandleEdit).toHaveBeenCalledWith(mockGroups[0]);
  });

  test('calls handleDelete when Delete team action is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const rowA = screen.getByTestId('row-1');
    const deleteButton = screen.getByTestId('row-1-delete-team');
    fireEvent.click(deleteButton);

    expect(mockHandleDelete).toHaveBeenCalledTimes(1);
    expect(mockHandleDelete).toHaveBeenCalledWith(mockGroups[0]);
  });

  test('calls handlePageChange when pagination button is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const changePageButton = screen.getByText('Change Page');
    fireEvent.click(changePageButton);

    expect(mockHandlePageChange).toHaveBeenCalledTimes(1);
  });

  test('calls handlePageSizeChange when page size button is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const changePageSizeButton = screen.getByText('Change Page Size');
    fireEvent.click(changePageSizeButton);

    expect(mockHandlePageSizeChange).toHaveBeenCalledTimes(1);
  });

  test('calls handleSearch when search input value changes', () => {
    render(<GroupsTable {...defaultProps} />);

    const searchInput = screen.getByTestId('search-input');
    fireEvent.change(searchInput, { target: { value: 'test search' } });

    expect(mockHandleSearch).toHaveBeenCalledTimes(1);
    expect(mockHandleSearch).toHaveBeenCalledWith('test search');
  });

  test('calls handleSortChange when sort button is clicked', () => {
    render(<GroupsTable {...defaultProps} />);

    const sortButton = screen.getByText('Sort');
    fireEvent.click(sortButton);

    expect(mockHandleSortChange).toHaveBeenCalledTimes(1);
    expect(mockHandleSortChange).toHaveBeenCalledWith({ field: 'id', direction: 'asc' });
  });
});