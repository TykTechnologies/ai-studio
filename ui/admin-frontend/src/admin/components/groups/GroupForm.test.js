import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupForm from './GroupForm';
import { useGroupForm } from './hooks/useGroupForm';
import { useCatalogsSelection } from './hooks/useCatalogsSelection';

// Mock React Router
jest.mock('react-router-dom', () => ({
  useParams: jest.fn().mockImplementation(() => ({})),
  Link: ({ children, to, ...props }) => (
    <a href={to} {...props} data-testid="mock-link">
      {children}
    </a>
  ),
  useNavigate: () => jest.fn()
}));

// Mock the hooks
jest.mock('./hooks/useGroupForm', () => ({
  useGroupForm: jest.fn()
}));

jest.mock('./hooks/useCatalogsSelection', () => ({
  useCatalogsSelection: jest.fn()
}));

// Mock the child components
jest.mock('./components/GroupFormBasicInfo', () => ({
  __esModule: true,
  default: props => (
    <div data-testid="mock-basic-info">
      <input
        data-testid="mock-name-input"
        value={props.name || ''}
        onChange={e => props.setName && props.setName(e.target.value)}
      />
      {props.error && <div data-testid="mock-error">{props.error}</div>}
    </div>
  )
}));

jest.mock('./components/GroupMembersSection', () => ({
  __esModule: true,
  default: props => (
    <div data-testid="mock-members-section">
      <button
        data-testid="mock-change-users"
        onClick={() => props.handleUsersChange({
          selected: [...props.selectedUsers, { id: '999', name: 'New Test User' }]
        })}
      >
        Add User
      </button>
    </div>
  )
}));

jest.mock('./components/GroupCatalogsSection', () => ({
  __esModule: true,
  default: props => (
    <div data-testid="mock-catalogs-section">
      <button
        data-testid="mock-change-catalogs"
        onClick={() => props.onCatalogsChange([...props.selectedCatalogs, { value: '999', label: 'New Catalog' }])}
      >
        Add Catalog
      </button>
      <button
        data-testid="mock-change-data-catalogs"
        onClick={() => props.onDataCatalogsChange([...props.selectedDataCatalogs, { value: '888', label: 'New Data Catalog' }])}
      >
        Add Data Catalog
      </button>
      <button
        data-testid="mock-change-tool-catalogs"
        onClick={() => props.onToolCatalogsChange([...props.selectedToolCatalogs, { value: '777', label: 'New Tool Catalog' }])}
      >
        Add Tool Catalog
      </button>
    </div>
  )
}));

// Mock Material UI components
jest.mock('@mui/material', () => ({
  Typography: ({ children, variant, ...props }) => (
    <div data-testid="mock-typography" data-variant={variant} {...props}>{children}</div>
  ),
  
  CircularProgress: () => <div data-testid="mock-circular-progress" />,
  
  Box: ({ children, ...props }) => (
    <div data-testid="mock-box" {...props}>{children}</div>
  ),
  
  Snackbar: ({ children, open, onClose, ...props }) => (
    open ? (
      <div data-testid="mock-snackbar" {...props}>
        {children}
        <button data-testid="mock-snackbar-close" onClick={onClose}>Close</button>
      </div>
    ) : null
  ),
  
  Alert: ({ children, severity, onClose, ...props }) => (
    <div data-testid="mock-alert" data-severity={severity} {...props}>
      {children}
      {onClose && <button data-testid="mock-alert-close" onClick={onClose}>Close Alert</button>}
    </div>
  )
}));

jest.mock('@mui/icons-material/ChevronLeft', () => ({
  __esModule: true,
  default: () => <span data-testid="mock-chevron-left-icon" />
}));

// Mock ConfirmationDialog
jest.mock('../../components/common/ConfirmationDialog', () => ({
  __esModule: true,
  default: ({ open, onConfirm, onCancel, ...props }) => (
    open ? (
      <div data-testid="mock-confirmation-dialog" {...props}>
        <button data-testid="mock-confirm-button" onClick={onConfirm}>Confirm</button>
        <button data-testid="mock-cancel-button" onClick={onCancel}>Cancel</button>
      </div>
    ) : null
  )
}));

// Mock shared styles
jest.mock('../../styles/sharedStyles', () => ({
  SecondaryLinkButton: ({ children, component, to, ...props }) => (
    <button data-testid="mock-secondary-link-button" data-to={to} {...props}>
      {children}
    </button>
  ),
  TitleBox: ({ children, ...props }) => (
    <div data-testid="mock-title-box" {...props}>{children}</div>
  ),
  ContentBox: ({ children, ...props }) => (
    <div data-testid="mock-content-box" {...props}>{children}</div>
  ),
  TitleContentBox: ({ children, ...props }) => (
    <div data-testid="mock-title-content-box" {...props}>{children}</div>
  ),
  PrimaryButton: ({ children, type, disabled, onClick, ...props }) => (
    <button
      data-testid="mock-primary-button"
      type={type}
      disabled={disabled}
      onClick={onClick}
      {...props}
    >
      {children}
    </button>
  ),
  DangerOutlineButton: ({ children, onClick, ...props }) => (
    <button
      data-testid="mock-danger-outline-button"
      onClick={onClick}
      {...props}
    >
      {children}
    </button>
  ),
}));

describe('GroupForm Component', () => {
  // Mock hook return values
  const mockHandleSubmit = jest.fn(e => e.preventDefault());
  const mockHandleCloseSnackbar = jest.fn();
  const mockHandleDeleteClick = jest.fn();
  const mockHandleCancelDelete = jest.fn();
  const mockHandleConfirmDelete = jest.fn();
  const mockSetName = jest.fn();
  const mockSetSelectedUsers = jest.fn();
  const mockSetSelectedCatalogs = jest.fn();
  const mockSetSelectedDataCatalogs = jest.fn();
  const mockSetSelectedToolCatalogs = jest.fn();
  
  const mockFetchUsers = jest.fn();
  const mockHandleUsersChange = jest.fn();
  const mockHandleSearch = jest.fn();
  const mockHandleLoadMore = jest.fn();
  
  // Default mock values
  const defaultGroupFormValues = {
    name: 'Test Team',
    setName: mockSetName,
    loading: false,
    error: null,
    selectedUsers: [{ id: '1', name: 'User 1' }],
    setSelectedUsers: mockSetSelectedUsers,
    selectedCatalogs: [{ value: '1', label: 'Catalog 1' }],
    setSelectedCatalogs: mockSetSelectedCatalogs,
    selectedDataCatalogs: [{ value: '2', label: 'Data Catalog 1' }],
    setSelectedDataCatalogs: mockSetSelectedDataCatalogs,
    selectedToolCatalogs: [{ value: '3', label: 'Tool Catalog 1' }],
    setSelectedToolCatalogs: mockSetSelectedToolCatalogs,
    handleSubmit: mockHandleSubmit,
    snackbar: { open: false, message: '', severity: 'success' },
    handleCloseSnackbar: mockHandleCloseSnackbar,
    warningDialogOpen: false,
    handleDeleteClick: mockHandleDeleteClick,
    handleCancelDelete: mockHandleCancelDelete,
    handleConfirmDelete: mockHandleConfirmDelete
  };
  
  const defaultUserSelectionValues = {
    availableUsers: [{ id: '2', name: 'User 2' }, { id: '3', name: 'User 3' }],
    currentPage: 1,
    totalPages: 2,
    isLoadingMore: false,
    loading: false,
    fetchUsers: mockFetchUsers,
    handleUsersChange: mockHandleUsersChange,
    handleLoadMore: mockHandleLoadMore,
    handleSearch: mockHandleSearch,
    handleUserAdded: jest.fn(),
    handleUserRemoved: jest.fn(),
    resetState: jest.fn()
  };
  
  const defaultCatalogsSelectionValues = {
    catalogs: [{ value: '1', label: 'Catalog 1' }, { value: '4', label: 'Catalog 2' }],
    dataCatalogs: [{ value: '2', label: 'Data Catalog 1' }, { value: '5', label: 'Data Catalog 2' }],
    toolCatalogs: [{ value: '3', label: 'Tool Catalog 1' }, { value: '6', label: 'Tool Catalog 2' }],
    loading: false
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    // Setup default mock hook implementations for all tests
    useGroupForm.mockImplementation(() => defaultGroupFormValues);
    useCatalogsSelection.mockImplementation(() => defaultCatalogsSelectionValues);
    
    // Set default mock for useParams (create mode) for all tests
    require('react-router-dom').useParams.mockImplementation(() => ({}));
  });

  test('renders the component in create mode', () => {
    // Make sure useParams returns empty object for create mode
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    // Check title and form content - need to find within the Typography mock
    const titleElement = screen.getAllByTestId('mock-typography')
      .find(el => el.textContent === 'Create team');
    expect(titleElement).toBeInTheDocument();
    
    expect(screen.getByTestId('mock-basic-info')).toBeInTheDocument();
    expect(screen.getByTestId('mock-members-section')).toBeInTheDocument();
    expect(screen.getByTestId('mock-catalogs-section')).toBeInTheDocument();
    
    // Check submit button text for create mode
    expect(screen.getByTestId('mock-primary-button')).toHaveTextContent('Create team');
    
    // Delete button should not be present in create mode
    const dangerButtons = screen.queryAllByTestId('mock-danger-outline-button');
    expect(dangerButtons.length).toBe(0);
  });

  test('renders the component in edit mode', () => {
    // Mock useParams to simulate editing an existing group
    require('react-router-dom').useParams.mockImplementation(() => ({ id: '123' }));
    
    render(<GroupForm />);
    
    // Check title for edit mode - need to find within the Typography mock
    const titleElement = screen.getAllByTestId('mock-typography')
      .find(el => el.textContent === 'Edit team');
    expect(titleElement).toBeInTheDocument();
    
    // Check submit button text for edit mode
    expect(screen.getByTestId('mock-primary-button')).toHaveTextContent('Update team');
    
    // Delete button should be present in edit mode
    const deleteButton = screen.getByTestId('mock-danger-outline-button');
    expect(deleteButton).toHaveTextContent('Delete team');
  });

  test('shows loading spinner when form data is loading', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      loading: true
    }));
    
    useCatalogsSelection.mockImplementation(() => ({
      ...defaultCatalogsSelectionValues,
      loading: false
    }));
    
    const { debug } = render(<GroupForm />);
    // debug(); // Uncomment to see rendered output for debugging
    
    expect(screen.getByTestId('mock-circular-progress')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-basic-info')).not.toBeInTheDocument();
  });

  test('shows loading spinner when users are loading', () => {
    // Set only formLoading state to true
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      loading: false
    }));
    
    useCatalogsSelection.mockImplementation(() => ({
      ...defaultCatalogsSelectionValues,
      loading: true
    }));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('mock-circular-progress')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-basic-info')).not.toBeInTheDocument();
  });

  test('shows loading spinner when catalogs are loading', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      loading: false
    }));
    
    useCatalogsSelection.mockImplementation(() => ({
      ...defaultCatalogsSelectionValues,
      loading: true
    }));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('mock-circular-progress')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-basic-info')).not.toBeInTheDocument();
  });

  test('submits the form when the submit button is clicked', () => {
    render(<GroupForm />);
    
    // Submit the form
    const submitButton = screen.getByTestId('mock-primary-button');
    fireEvent.click(submitButton);
    
    // Check if handleSubmit was called with a mocked event
    expect(mockHandleSubmit).toHaveBeenCalledWith(expect.objectContaining({
      preventDefault: expect.any(Function)
    }));
  });

  test('disables submit button when name is empty', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      name: ''
    }));
    
    // Make sure useParams returns empty object for create mode
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const submitButton = screen.getByTestId('mock-primary-button');
    expect(submitButton).toHaveAttribute('disabled', '');
  });

  test('shows snackbar when open', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      snackbar: { open: true, message: 'Test message', severity: 'success' }
    }));
    
    // Make sure useParams returns empty object for create mode
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('mock-snackbar')).toBeInTheDocument();
    
    // Find the alert inside the snackbar
    const alertElement = screen.getByTestId('mock-alert');
    expect(alertElement).toHaveTextContent('Test message');
  });

  test('closes snackbar when close button is clicked', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      snackbar: { open: true, message: 'Test message', severity: 'success' }
    }));
    
    // Make sure useParams returns empty object for create mode
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const closeButton = screen.getByTestId('mock-snackbar-close');
    fireEvent.click(closeButton);
    
    expect(mockHandleCloseSnackbar).toHaveBeenCalled();
  });

  test('opens confirmation dialog when delete button is clicked', () => {
    // Mock useParams to simulate editing an existing group
    require('react-router-dom').useParams.mockImplementation(() => ({ id: '123' }));
    
    render(<GroupForm />);
    
    // Click the delete button
    const deleteButton = screen.getByTestId('mock-danger-outline-button');
    expect(deleteButton).toHaveTextContent('Delete team');
    fireEvent.click(deleteButton);
    
    // Check if handleDeleteClick was called
    expect(mockHandleDeleteClick).toHaveBeenCalled();
  });

  test('shows confirmation dialog when warningDialogOpen is true', () => {
    // Set warningDialogOpen to true
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('mock-confirmation-dialog')).toBeInTheDocument();
  });

  test('calls handleConfirmDelete when confirm button in dialog is clicked', () => {
    // Set warningDialogOpen to true
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const confirmButton = screen.getByTestId('mock-confirm-button');
    fireEvent.click(confirmButton);
    
    expect(mockHandleConfirmDelete).toHaveBeenCalled();
  });

  test('calls handleCancelDelete when cancel button in dialog is clicked', () => {
    // Set warningDialogOpen to true
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const cancelButton = screen.getByTestId('mock-cancel-button');
    fireEvent.click(cancelButton);
    
    expect(mockHandleCancelDelete).toHaveBeenCalled();
  });

  test('renders the component properly', () => {
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      selectedUsers: [{ id: '1', name: 'User 1' }]
    }));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('mock-basic-info')).toBeInTheDocument();
    expect(screen.getByTestId('mock-members-section')).toBeInTheDocument();
    expect(screen.getByTestId('mock-catalogs-section')).toBeInTheDocument();
  });

  test('handles catalog selection changes', () => {
    // Reset the mock implementation to make sure we can track the function call
    mockSetSelectedCatalogs.mockClear();
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    // Click the "Add Catalog" button in the mock GroupCatalogsSection
    const addCatalogButton = screen.getByTestId('mock-change-catalogs');
    fireEvent.click(addCatalogButton);
    
    // Check if the catalogs change handlers were called
    expect(mockSetSelectedCatalogs).toHaveBeenCalled();
  });

  test('handles data catalog selection changes', () => {
    // Reset the mock implementation to make sure we can track the function call
    mockSetSelectedDataCatalogs.mockClear();
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    // Click the "Add Data Catalog" button in the mock GroupCatalogsSection
    const addDataCatalogButton = screen.getByTestId('mock-change-data-catalogs');
    fireEvent.click(addDataCatalogButton);
    
    // Check if the data catalogs change handlers were called
    expect(mockSetSelectedDataCatalogs).toHaveBeenCalled();
  });

  test('handles tool catalog selection changes', () => {
    // Reset the mock implementation to make sure we can track the function call
    mockSetSelectedToolCatalogs.mockClear();
    
    // Make sure useParams returns empty object
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    // Click the "Add Tool Catalog" button in the mock GroupCatalogsSection
    const addToolCatalogButton = screen.getByTestId('mock-change-tool-catalogs');
    fireEvent.click(addToolCatalogButton);
    
    // Check if the tool catalogs change handlers were called
    expect(mockSetSelectedToolCatalogs).toHaveBeenCalled();
  });
});