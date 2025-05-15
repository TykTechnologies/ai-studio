import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';

// Mock Material-UI components
jest.mock('@mui/material', () => ({
  Table: ({ children, style, ...props }) => (
    <table data-testid="table" style={style} {...props}>
      {children}
    </table>
  ),
  TableHead: ({ children, ...props }) => (
    <thead data-testid="table-head" {...props}>
      {children}
    </thead>
  ),
  TableBody: ({ children, ...props }) => (
    <tbody data-testid="table-body" {...props}>
      {children}
    </tbody>
  ),
  TableCell: ({ children, width, align, style, colSpan, ...props }) => (
    <td 
      data-testid="table-cell" 
      data-width={width}
      data-align={align}
      style={style}
      colSpan={colSpan}
      {...props}
    >
      {children}
    </td>
  ),
  Typography: ({ children, variant, color, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} {...props}>
      {children}
    </div>
  ),
}));

// Mock icons
jest.mock('@mui/icons-material/Add', () => () => <div data-testid="add-icon" />);
jest.mock('@mui/icons-material/Close', () => () => <div data-testid="close-icon" />);

// Mock styled components
jest.mock('../../../styles/sharedStyles', () => ({
  StyledTableCell: ({ children, width, style, ...props }) => (
    <td 
      data-testid="styled-table-cell" 
      data-width={width}
      style={style}
      {...props}
    >
      {children}
    </td>
  ),
  StyledTableRow: ({ children, ...props }) => (
    <tr data-testid="styled-table-row" {...props}>
      {children}
    </tr>
  ),
}));

jest.mock('../transfer-list/styles', () => ({
  AddButton: ({ children, onClick, ...props }) => (
    <button data-testid="add-button" onClick={onClick} {...props}>
      {children}
    </button>
  ),
  RemoveButton: ({ children, onClick, ...props }) => (
    <button data-testid="remove-button" onClick={onClick} {...props}>
      {children}
    </button>
  ),
  TableHeaderRow: ({ children, ...props }) => (
    <tr data-testid="table-header-row" {...props}>
      {children}
    </tr>
  ),
}));

// Import the component under test
const TransferListTable = require('../transfer-list/TransferListTable').default;

describe('TransferListTable Component', () => {
  // Mock data
  const mockItems = [
    { id: '1', name: 'Item 1', description: 'Description 1' },
    { id: '2', name: 'Item 2', description: 'Description 2' },
  ];
  
  const mockColumns = [
    { field: 'name', headerName: 'Name', width: '50%' },
    { field: 'description', headerName: 'Description', width: '40%' },
  ];

  const mockColumnsWithRenderCell = [
    {
      field: 'name',
      headerName: 'Name',
      width: '50%',
      renderCell: (item) => <span data-testid="custom-cell">{`Custom ${item.name}`}</span>
    },
    { field: 'description', headerName: 'Description', width: '40%' },
  ];
  
  test('renders table structure correctly', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
      />
    );
    
    expect(screen.getByTestId('table')).toBeInTheDocument();
    expect(screen.getByTestId('table-head')).toBeInTheDocument();
    expect(screen.getByTestId('table-body')).toBeInTheDocument();
    expect(screen.getByTestId('table-header-row')).toBeInTheDocument();
  });

  test('renders column headers correctly', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
      />
    );
    
    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Description')).toBeInTheDocument();
    expect(screen.getByText('Actions')).toBeInTheDocument();
    
    // Check header styling
    const headerCells = screen.getAllByTestId('table-cell');
    expect(headerCells[0]).toHaveAttribute('data-width', '50%');
    expect(headerCells[1]).toHaveAttribute('data-width', '40%');
  });

  test('renders rows with correct data', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
      />
    );
    
    expect(screen.getByText('Item 1')).toBeInTheDocument();
    expect(screen.getByText('Item 2')).toBeInTheDocument();
    expect(screen.getByText('Description 1')).toBeInTheDocument();
    expect(screen.getByText('Description 2')).toBeInTheDocument();
    
    // Check number of rows
    expect(screen.getAllByTestId('styled-table-row')).toHaveLength(2);
  });

  test('renders custom cell content when renderCell is provided', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumnsWithRenderCell}
        idField="id"
        isLeftSide={true}
      />
    );
    
    expect(screen.getAllByTestId('custom-cell')).toHaveLength(2);
    expect(screen.getByText('Custom Item 1')).toBeInTheDocument();
    expect(screen.getByText('Custom Item 2')).toBeInTheDocument();
  });

  test('renders remove buttons when isLeftSide is true', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
        onRemoveItem={jest.fn()}
      />
    );
    
    const removeButtons = screen.getAllByTestId('remove-button');
    expect(removeButtons).toHaveLength(2);
    expect(screen.getAllByTestId('close-icon')).toHaveLength(2);
    expect(screen.queryByTestId('add-button')).not.toBeInTheDocument();
  });

  test('renders add buttons when isLeftSide is false', () => {
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={false}
        onAddItem={jest.fn()}
      />
    );
    
    const addButtons = screen.getAllByTestId('add-button');
    expect(addButtons).toHaveLength(2);
    expect(screen.getAllByTestId('add-icon')).toHaveLength(2);
    expect(screen.queryByTestId('remove-button')).not.toBeInTheDocument();
  });

  test('calls onAddItem with the correct item when add button is clicked', () => {
    const onAddItemMock = jest.fn();
    
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={false}
        onAddItem={onAddItemMock}
      />
    );
    
    const addButtons = screen.getAllByTestId('add-button');
    fireEvent.click(addButtons[0]);
    
    expect(onAddItemMock).toHaveBeenCalledWith(mockItems[0]);
  });

  test('calls onRemoveItem with the correct item when remove button is clicked', () => {
    const onRemoveItemMock = jest.fn();
    
    render(
      <TransferListTable
        items={mockItems}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
        onRemoveItem={onRemoveItemMock}
      />
    );
    
    const removeButtons = screen.getAllByTestId('remove-button');
    fireEvent.click(removeButtons[1]);
    
    expect(onRemoveItemMock).toHaveBeenCalledWith(mockItems[1]);
  });

  test('displays "No items to display" message when items array is empty', () => {
    render(
      <TransferListTable
        items={[]}
        columns={mockColumns}
        idField="id"
        isLeftSide={true}
      />
    );
    
    expect(screen.getByText('No items to display')).toBeInTheDocument();
    // When items array is empty, there should be exactly one row for the "No items to display" message
    expect(screen.getAllByTestId('styled-table-row')).toHaveLength(1);
  });

  test('uses custom idField when provided', () => {
    const customIdItems = [
      { customId: 'a1', name: 'Item A' },
      { customId: 'a2', name: 'Item B' },
    ];
    
    render(
      <TransferListTable
        items={customIdItems}
        columns={[{ field: 'name', headerName: 'Name', width: '90%' }]}
        idField="customId"
        isLeftSide={true}
      />
    );
    
    // Check that it renders without errors - if it's using the wrong ID field, it would likely
    // cause React key warnings or rendering issues
    expect(screen.getByText('Item A')).toBeInTheDocument();
    expect(screen.getByText('Item B')).toBeInTheDocument();
  });
});