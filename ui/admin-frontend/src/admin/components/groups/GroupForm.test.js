import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupForm from './GroupForm';
import { useGroupForm } from './hooks/useGroupForm';
import { useCatalogsSelection } from './hooks/useCatalogsSelection';

jest.mock('@mui/material', () => require('../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('@mui/icons-material/ChevronLeft', () => () => 'ChevronLeftIcon');
jest.mock('../../styles/sharedStyles', () => require('../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('react-router-dom', () => ({
  useParams: jest.fn().mockImplementation(() => ({})),
  Link: ({ children, to, ...props }) => (
    <a href={to} {...props} data-testid="mock-link">
      {children}
    </a>
  ),
  useNavigate: () => jest.fn()
}));

jest.mock('./hooks/useGroupForm', () => ({
  useGroupForm: jest.fn()
}));

jest.mock('./hooks/useCatalogsSelection', () => ({
  useCatalogsSelection: jest.fn()
}));

jest.mock('../../hooks/useSnackbarState', () => ({
  useSnackbarState: () => ({
    snackbarState: { open: false, message: '', severity: 'success' },
    showSnackbar: jest.fn(),
    hideSnackbar: jest.fn()
  })
}));

jest.mock('../../hooks/useSystemFeatures', () => ({
  __esModule: true,
  default: () => ({
    features: {}
  })
}));

jest.mock('../../utils/featureUtils', () => ({
  getFeatureFlags: () => ({
    isGatewayOnly: false
  })
}));

jest.mock('../../hooks/useOverviewData', () => ({
  __esModule: true,
  default: () => ({
    getDocsLink: jest.fn()
  })
}));

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
        onClick={() => props.onSelectedUsersChange([{ id: '999', name: 'New Test User' }])}
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

jest.mock('../../components/common/ConfirmationDialog', () => ({
  __esModule: true,
  default: ({ 
    open, 
    onConfirm, 
    onCancel, 
    title,
    message,
    buttonLabel,
    iconName,
    iconColor,
    titleColor,
    backgroundColor,
    borderColor,
    primaryButtonComponent,
    ...props 
  }) => (
    open ? (
      <div 
        data-testid="confirmation-dialog" 
        data-title={title}
        data-message={message}
        data-button-label={buttonLabel}
      >
        <button data-testid="confirm-button" onClick={onConfirm}>Confirm</button>
        <button data-testid="cancel-button" onClick={onCancel}>Cancel</button>
      </div>
    ) : null
  )
}));

jest.mock('../../components/common/AlertSnackbar', () => ({
  __esModule: true,
  default: ({ open, message, severity, onClose }) => (
    <div data-testid="alert-snackbar" data-open={open} data-message={message} data-severity={severity}>
      <button onClick={onClose} data-testid="close-snackbar">Close</button>
    </div>
  )
}));

describe('GroupForm Component', () => {
  const mockHandleSubmit = jest.fn(e => e.preventDefault());
  const mockHandleDeleteClick = jest.fn();
  const mockHandleCancelDelete = jest.fn();
  const mockHandleConfirmDelete = jest.fn();
  const mockSetName = jest.fn();
  const mockSetSelectedUsers = jest.fn();
  const mockSetSelectedCatalogs = jest.fn();
  const mockSetSelectedDataCatalogs = jest.fn();
  const mockSetSelectedToolCatalogs = jest.fn();
  
  const defaultGroupFormValues = {
    name: 'Test Team',
    setName: mockSetName,
    loading: false,
    setSelectedUsers: mockSetSelectedUsers,
    selectedCatalogs: [{ value: '1', label: 'Catalog 1' }],
    setSelectedCatalogs: mockSetSelectedCatalogs,
    selectedDataCatalogs: [{ value: '2', label: 'Data Catalog 1' }],
    setSelectedDataCatalogs: mockSetSelectedDataCatalogs,
    selectedToolCatalogs: [{ value: '3', label: 'Tool Catalog 1' }],
    setSelectedToolCatalogs: mockSetSelectedToolCatalogs,
    handleSubmit: mockHandleSubmit,
    warningDialogOpen: false,
    handleDeleteClick: mockHandleDeleteClick,
    handleCancelDelete: mockHandleCancelDelete,
    handleConfirmDelete: mockHandleConfirmDelete
  };
  
  const defaultCatalogsSelectionValues = {
    catalogs: [{ value: '1', label: 'Catalog 1' }, { value: '4', label: 'Catalog 2' }],
    dataCatalogs: [{ value: '2', label: 'Data Catalog 1' }, { value: '5', label: 'Data Catalog 2' }],
    toolCatalogs: [{ value: '3', label: 'Tool Catalog 1' }, { value: '6', label: 'Tool Catalog 2' }],
    loading: false
  };

  beforeEach(() => {
    jest.clearAllMocks();
    
    useGroupForm.mockImplementation(() => defaultGroupFormValues);
    useCatalogsSelection.mockImplementation(() => defaultCatalogsSelectionValues);
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
  });

  test('renders the component in create mode', () => {
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const titleElement = screen.getAllByTestId('typography')
      .find(el => el.textContent === 'Create team');
    expect(titleElement).toBeInTheDocument();
    
    expect(screen.getByTestId('mock-basic-info')).toBeInTheDocument();
    expect(screen.getByTestId('mock-members-section')).toBeInTheDocument();
    expect(screen.getByTestId('mock-catalogs-section')).toBeInTheDocument();
    
    expect(screen.getByTestId('primary-button')).toHaveTextContent('Create team');
    
    const dangerButtons = screen.queryAllByTestId('danger-outline-button');
    expect(dangerButtons.length).toBe(0);
  });

  test('renders the component in edit mode', () => {
    require('react-router-dom').useParams.mockImplementation(() => ({ id: '123' }));
    
    render(<GroupForm />);
    
    const titleElement = screen.getAllByTestId('typography')
      .find(el => el.textContent === 'Edit team');
    expect(titleElement).toBeInTheDocument();
    
    expect(screen.getByTestId('primary-button')).toHaveTextContent('Update team');
    
    const deleteButton = screen.getByTestId('danger-outline-button');
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
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('circular-progress')).toBeInTheDocument();
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
    
    expect(screen.getByTestId('circular-progress')).toBeInTheDocument();
    expect(screen.queryByTestId('mock-basic-info')).not.toBeInTheDocument();
  });

  test('submits the form when the submit button is clicked', () => {
    render(<GroupForm />);
    
    const submitButton = screen.getByTestId('primary-button');
    fireEvent.click(submitButton);
    
    expect(mockHandleSubmit).toHaveBeenCalledWith(expect.objectContaining({
      preventDefault: expect.any(Function)
    }));
  });

  test('disables submit button when name is empty', () => {
    useGroupForm.mockImplementation(() => ({
      ...defaultGroupFormValues,
      name: ''
    }));
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const submitButton = screen.getByTestId('primary-button');
    expect(submitButton).toHaveAttribute('disabled', '');
  });

  test('opens confirmation dialog when delete button is clicked', () => {
    require('react-router-dom').useParams.mockImplementation(() => ({ id: '123' }));
    
    render(<GroupForm />);
    
    const deleteButton = screen.getByTestId('danger-outline-button');
    expect(deleteButton).toHaveTextContent('Delete team');
    fireEvent.click(deleteButton);
    
    expect(mockHandleDeleteClick).toHaveBeenCalled();
  });

  test('shows confirmation dialog when warningDialogOpen is true', () => {
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    expect(screen.getByTestId('confirmation-dialog')).toBeInTheDocument();
  });

  test('calls handleConfirmDelete when confirm button in dialog is clicked', () => {
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const confirmButton = screen.getByTestId('confirm-button');
    fireEvent.click(confirmButton);
    
    expect(mockHandleConfirmDelete).toHaveBeenCalled();
  });

  test('calls handleCancelDelete when cancel button in dialog is clicked', () => {
    useGroupForm.mockImplementation(() => ({ ...defaultGroupFormValues, warningDialogOpen: true }));
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const cancelButton = screen.getByTestId('cancel-button');
    fireEvent.click(cancelButton);
    
    expect(mockHandleCancelDelete).toHaveBeenCalled();
  });

  test('handles catalog selection changes', () => {
    mockSetSelectedCatalogs.mockClear();
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const addCatalogButton = screen.getByTestId('mock-change-catalogs');
    fireEvent.click(addCatalogButton);
    
    expect(mockSetSelectedCatalogs).toHaveBeenCalled();
  });

  test('handles data catalog selection changes', () => {
    mockSetSelectedDataCatalogs.mockClear();
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const addDataCatalogButton = screen.getByTestId('mock-change-data-catalogs');
    fireEvent.click(addDataCatalogButton);
    
    expect(mockSetSelectedDataCatalogs).toHaveBeenCalled();
  });

  test('handles tool catalog selection changes', () => {
    mockSetSelectedToolCatalogs.mockClear();
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const addToolCatalogButton = screen.getByTestId('mock-change-tool-catalogs');
    fireEvent.click(addToolCatalogButton);
    
    expect(mockSetSelectedToolCatalogs).toHaveBeenCalled();
  });

  test('handles users selection changes', () => {
    mockSetSelectedUsers.mockClear();
    
    require('react-router-dom').useParams.mockImplementation(() => ({}));
    
    render(<GroupForm />);
    
    const addUserButton = screen.getByTestId('mock-change-users');
    fireEvent.click(addUserButton);
    
    expect(mockSetSelectedUsers).toHaveBeenCalledWith([{ id: '999', name: 'New Test User' }]);
  });
});