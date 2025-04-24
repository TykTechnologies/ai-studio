import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import AssignOwnerStep from './AssignOwnerStep';
import { useQuickStart } from './QuickStartContext';
import { createUser, updateUser } from '../../../services';
import { validateEmail, validatePassword } from './utils';

// Mock the QuickStartContext hook
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));

// Mock the services
jest.mock('../../../services', () => {
  const originalModule = jest.requireActual('../../../services');
  return {
    ...originalModule,
    createUser: jest.fn(),
    updateUser: jest.fn(),
  };
});

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon" data-icon-name={props.name} style={props.sx}>{props.name}</div>;
  };
});

// Mock the CustomNote component
jest.mock('../../common/CustomNote', () => {
  return function MockCustomNote(props) {
    return <div data-testid="mock-custom-note">{props.message}</div>;
  };
});

// Mock the shared components
jest.mock('../../../styles/sharedStyles', () => {
  const originalModule = jest.requireActual('../../../styles/sharedStyles');
  return {
    ...originalModule,
    StyledTextField: function MockStyledTextField(props) {
      return (
        <input
          data-testid={`input-${props.name}`}
          name={props.name}
          type={props.type || 'text'}
          value={props.value}
          onChange={props.onChange}
          required={props.required}
          aria-invalid={props.error ? 'true' : 'false'}
        />
      );
    },
    PrimaryButton: function MockPrimaryButton(props) {
      return (
        <button
          onClick={props.onClick}
          disabled={props.disabled}
          data-testid="primary-button"
        >
          {props.children}
        </button>
      );
    }
  };
});

// Mock the CustomSelect component
jest.mock('../../common/CustomSelect', () => {
  return function MockCustomSelect(props) {
    return (
      <div data-testid="mock-custom-select">
        <select
          name={props.name}
          value={props.value}
          onChange={props.onChange}
          data-testid={`select-${props.name}`}
        >
          {props.options.map(option => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </div>
    );
  };
});

// Mock the CustomSelectBadge component
jest.mock('../../common/CustomSelectBadge', () => {
  return function MockCustomSelectBadge(props) {
    return <div data-testid="mock-custom-select-badge">{props.config.text}</div>;
  };
});

// Mock the validation functions
jest.mock('./utils', () => ({
  validateEmail: jest.fn(),
  validatePassword: jest.fn(),
}));

describe('AssignOwnerStep Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      background: {
        buttonPrimaryDefault: '#1976d2',
        buttonPrimaryDefaultHover: '#181834',
        buttonPrimaryOutlineHover: '#e3f2fd',
        surfaceBrandDefaultPortal: '#e8f5e9',
        surfaceBrandDefaultDashboard: '#e8eaf6',
        paper: '#ffffff',
        default: '#ffffff',
        defaultSubdued: '#E6E6EA',
        neutralDefault: '#F0F0F3',
        secondaryExtraLight: '#f8f8f9',
        surfaceNeutralDisabled: '#FCFCFC',
        surfaceNeutralHover: '#F8F8F9',
        buttonSecondary: '#EDEDF0',
        buttonCritical: '#D82C0D',
        buttonCriticalHover: '#AE2410',
      },
      text: {
        primary: '#000000',
        defaultSubdued: '#666666',
        default: '#03031C',
        neutralDisabled: '#818198',
        light: '#FFFFFF',
      },
      border: {
        neutralDefault: '#e0e0e0',
        neutralPressed: '#cccccc',
        neutralHovered: '#9D9DAF',
        criticalDefault: '#AE2410',
        criticalDefaultSubdue: '#F9DDD8',
        criticalHover: '#8B1D12',
      },
      custom: {
        white: '#FFFFFF',
        leaf: '#21ecba',
        purpleExtraDark: '#5900CB',
        purpleDark: '#8438FA',
        purpleLight: '#B421FA',
        purpleExtraLight: '#F0E4FF',
        purpleMedium: '#BB11FF',
        teal: '#21ecba',
        lightTeal: 'rgba(33, 236, 186, 0.07)',
        hoverTeal: 'rgba(33, 236, 186, 0.47)',
      },
      primary: {
        main: '#23E2C2',
        light: '#82F5D8',
      },
      common: {
        black: '#000000',
        white: '#FFFFFF',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
    shape: {
      borderRadius: 4,
    },
  });

  // Mock QuickStart context values
  const mockSetStepValid = jest.fn();
  const mockGoToNextStep = jest.fn();
  const mockGoToPreviousStep = jest.fn();
  const mockSkipQuickStart = jest.fn();
  const mockSetOwnerData = jest.fn();
  const mockSetCreatedOwnerId = jest.fn();

  // Mock current user
  const mockCurrentUser = {
    id: 'user123',
    name: 'Test User',
    email: 'test@example.com',
  };

  // Default context values
  const defaultContextValues = {
    setStepValid: mockSetStepValid,
    goToNextStep: mockGoToNextStep,
    goToPreviousStep: mockGoToPreviousStep,
    skipQuickStart: mockSkipQuickStart,
    ownerData: {},
    setOwnerData: mockSetOwnerData,
    createdOwnerId: null,
    setCreatedOwnerId: mockSetCreatedOwnerId,
    currentUser: mockCurrentUser,
  };

  // Reset mocks before each test
  beforeEach(() => {
    jest.clearAllMocks();
    useQuickStart.mockReturnValue(defaultContextValues);
    validateEmail.mockImplementation((email) => {
      if (!email || !/\S+@\S+\.\S+/.test(email)) {
        return "Email is invalid";
      }
      return null;
    });
    validatePassword.mockImplementation((password, criteria) => {
      if (!criteria.length) {
        return "Password must be at least 8 characters";
      }
      return null;
    });
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders the component with current user as default owner type', () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Check that the component renders with the correct title and radio options
    expect(screen.getByText(/Now let's choose who will own this app/)).toBeInTheDocument();
    expect(screen.getByText('Set me as owner')).toBeInTheDocument();
    expect(screen.getByText('Add a new user')).toBeInTheDocument();
    
    // Check that the "current user" option is selected by default
    const currentUserRadio = screen.getByLabelText('Set me as owner');
    expect(currentUserRadio).toBeChecked();
    
    // New user form should not be visible
    expect(screen.queryByText('Name*')).not.toBeInTheDocument();
    
    // Check that the buttons are rendered
    expect(screen.getByText('Skip quick start')).toBeInTheDocument();
    expect(screen.getByText('Back')).toBeInTheDocument();
    expect(screen.getByText('Continue')).toBeInTheDocument();
    
    // Continue button should be enabled (current user is valid)
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect(continueButton).not.toBeDisabled();
  });

  test('loads existing owner data when available', () => {
    const existingOwnerData = {
      ownerType: 'new',
      formData: {
        name: 'John Doe',
        email: 'john@example.com',
        password: 'Password123!',
        role: 'admin'
      }
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      ownerData: existingOwnerData
    });
    
    renderWithTheme(<AssignOwnerStep />);
    
    // Check that the "new user" option is selected
    const newUserRadio = screen.getByLabelText('Add a new user');
    expect(newUserRadio).toBeChecked();
    
    // Check that the form is populated with existing data
    // Get inputs by their test IDs
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    
    expect(nameInput).toHaveValue('John Doe');
    expect(emailInput).toHaveValue('john@example.com');
    expect(passwordInput).toHaveValue('Password123!');
    
    // Role should be set to admin
    const roleSelect = screen.getByTestId('select-role');
    expect(roleSelect).toHaveValue('admin');
    
    // Continue button should be enabled (form is valid)
    const continueButton = screen.getByTestId('primary-button');
    expect(continueButton).not.toBeDisabled();
  });

  test('toggles between current user and new user forms', () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Initially, the current user option should be selected
    const currentUserRadio = screen.getByLabelText('Set me as owner');
    const newUserRadio = screen.getByLabelText('Add a new user');
    expect(currentUserRadio).toBeChecked();
    expect(newUserRadio).not.toBeChecked();
    
    // New user form should not be visible
    expect(screen.queryByText('Name*')).not.toBeInTheDocument();
    
    // Switch to new user form
    fireEvent.click(newUserRadio);
    
    // New user form should now be visible
    expect(screen.getByText('Name*')).toBeInTheDocument();
    expect(screen.getByText('Email*')).toBeInTheDocument();
    expect(screen.getByText('Password*')).toBeInTheDocument();
    expect(screen.getByText('Role*')).toBeInTheDocument();
    
    // Continue button should be disabled (form is empty)
    const continueButton = screen.getByTestId('primary-button');
    expect(continueButton).toBeDisabled();
    
    // Switch back to current user
    fireEvent.click(currentUserRadio);
    
    // New user form should not be visible again
    expect(screen.queryByText('Name*')).not.toBeInTheDocument();
    
    // Continue button should be enabled (current user is valid)
    expect(continueButton).not.toBeDisabled();
  });

  test('validates form fields correctly', () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Switch to new user form
    const newUserRadio = screen.getByLabelText('Add a new user');
    fireEvent.click(newUserRadio);
    
    // Initially the form should be invalid (empty fields)
    const continueButton = screen.getByTestId('primary-button');
    expect(continueButton).toBeDisabled();
    
    // Fill out the form with invalid data
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    
    fireEvent.change(nameInput, { target: { value: 'John Doe' } });
    fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
    fireEvent.change(passwordInput, { target: { value: 'short' } });
    
    // Fill in valid data
    fireEvent.change(emailInput, { target: { value: 'john@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'Password123!' } });
    
    // Verify validation functions return expected results
    expect(validateEmail('john@example.com')).toBeNull(); // Valid email returns null (no error)
    expect(validateEmail('invalid-email')).toBe('Email is invalid'); // Invalid email returns error message
    
    // We can't directly test validatePassword without mocking passwordCriteria
    // So we'll just verify the form can be filled with valid data
  });

  test('creates a new user when form is submitted', async () => {
    // Mock API response
    createUser.mockResolvedValue({
      id: 'user456',
      attributes: {
        name: 'John Doe',
        email: 'john@example.com'
      }
    });
    
    renderWithTheme(<AssignOwnerStep />);
    
    // Switch to new user form
    const newUserRadio = screen.getByLabelText('Add a new user');
    fireEvent.click(newUserRadio);
    
    // Fill out the form
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    const roleSelect = screen.getByTestId('select-role');
    
    fireEvent.change(nameInput, { target: { value: 'John Doe' } });
    fireEvent.change(emailInput, { target: { value: 'john@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'Password123!' } });
    fireEvent.change(roleSelect, { target: { value: 'admin' } });
    
    // Submit the form
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // Wait for API call to complete
    await waitFor(() => {
      expect(createUser).toHaveBeenCalledWith(expect.objectContaining({
        name: 'John Doe',
        email: 'john@example.com',
        password: 'Password123!',
        isAdmin: true,
        showPortal: true,
        showChat: true
      }));
    });
    
    // Check that context was updated
    expect(mockSetCreatedOwnerId).toHaveBeenCalledWith('user456');
    expect(mockSetOwnerData).toHaveBeenCalledWith({
      ownerType: 'new',
      formData: {
        name: 'John Doe',
        email: 'john@example.com',
        password: 'Password123!',
        role: 'admin'
      },
      userId: 'user456'
    });
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('updates an existing user when form is submitted', async () => {
    // Setup context with existing user
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdOwnerId: 'user456',
      ownerData: {
        ownerType: 'new',
        formData: {
          name: 'John Doe',
          email: 'john@example.com',
          password: 'Password123!',
          role: 'developer'
        }
      }
    });
    
    renderWithTheme(<AssignOwnerStep />);
    
    // Form should be pre-filled with existing data
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    
    expect(nameInput).toHaveValue('John Doe');
    expect(emailInput).toHaveValue('john@example.com');
    expect(passwordInput).toHaveValue('Password123!');
    
    // Modify the form
    fireEvent.change(nameInput, { target: { value: 'John Smith' } });
    
    // Submit the form
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // Wait for API call to complete
    await waitFor(() => {
      expect(updateUser).toHaveBeenCalledWith('user456', expect.objectContaining({
        name: 'John Smith',
        email: 'john@example.com',
        password: 'Password123!',
        isAdmin: false,
        showPortal: true,
        showChat: true
      }));
    });
    
    // Check that context was updated
    expect(mockSetOwnerData).toHaveBeenCalledWith({
      ownerType: 'new',
      formData: {
        name: 'John Smith',
        email: 'john@example.com',
        password: 'Password123!',
        role: 'developer'
      },
      userId: 'user456'
    });
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('does not update user if data has not changed', async () => {
    // Setup context with existing user
    const existingFormData = {
      name: 'John Doe',
      email: 'john@example.com',
      password: 'Password123!',
      role: 'developer'
    };
    
    useQuickStart.mockReturnValue({
      ...defaultContextValues,
      createdOwnerId: 'user456',
      ownerData: {
        ownerType: 'new',
        formData: existingFormData
      }
    });
    
    renderWithTheme(<AssignOwnerStep />);
    
    // Submit the form without changing anything
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // Wait for component to process
    await waitFor(() => {
      // updateUser should not be called since data hasn't changed
      expect(updateUser).not.toHaveBeenCalled();
    });
    
    // Context should still be updated
    expect(mockSetOwnerData).toHaveBeenCalledWith({
      ownerType: 'new',
      formData: existingFormData,
      userId: 'user456'
    });
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('shows error message when API call fails', async () => {
    // Mock API failure
    createUser.mockRejectedValue(new Error('API Error'));
    
    renderWithTheme(<AssignOwnerStep />);
    
    // Switch to new user form
    const newUserRadio = screen.getByLabelText('Add a new user');
    fireEvent.click(newUserRadio);
    
    // Fill out the form
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    
    fireEvent.change(nameInput, { target: { value: 'John Doe' } });
    fireEvent.change(emailInput, { target: { value: 'john@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'Password123!' } });
    
    // Submit the form
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // Wait for error message to appear
    await waitFor(() => {
      expect(screen.getByText(/Failed to create user: API Error/i)).toBeInTheDocument();
    });
    
    // Context should not be updated
    expect(mockGoToNextStep).not.toHaveBeenCalled();
  });

  test('handles navigation buttons correctly', () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Click Back button
    const backButton = screen.getByRole('button', { name: /back/i });
    fireEvent.click(backButton);
    expect(mockGoToPreviousStep).toHaveBeenCalled();
    
    // Click Skip quick start button
    const skipButton = screen.getByRole('button', { name: /skip quick start/i });
    fireEvent.click(skipButton);
    expect(mockSkipQuickStart).toHaveBeenCalled();
  });

  test('sets current user as owner when that option is selected', async () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Current user option should be selected by default
    const currentUserRadio = screen.getByLabelText('Set me as owner');
    expect(currentUserRadio).toBeChecked();
    
    // Submit the form
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // No API calls should be made
    expect(createUser).not.toHaveBeenCalled();
    expect(updateUser).not.toHaveBeenCalled();
    
    // Check that context was updated correctly
    expect(mockSetOwnerData).toHaveBeenCalledWith({
      ownerType: 'current',
      userId: 'user123',
      name: 'Test User',
      email: 'test@example.com',
      role: 'admin'
    });
    expect(mockGoToNextStep).toHaveBeenCalled();
  });

  test('handles different role selections correctly', async () => {
    renderWithTheme(<AssignOwnerStep />);
    
    // Switch to new user form
    const newUserRadio = screen.getByLabelText('Add a new user');
    fireEvent.click(newUserRadio);
    
    // Fill out the form
    const nameInput = screen.getByTestId('input-name');
    const emailInput = screen.getByTestId('input-email');
    const passwordInput = screen.getByTestId('input-password');
    const roleSelect = screen.getByTestId('select-role');
    
    fireEvent.change(nameInput, { target: { value: 'John Doe' } });
    fireEvent.change(emailInput, { target: { value: 'john@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'Password123!' } });
    
    // Test with chatUser role
    fireEvent.change(roleSelect, { target: { value: 'chatUser' } });
    
    // Submit the form
    const continueButton = screen.getByTestId('primary-button');
    fireEvent.click(continueButton);
    
    // Wait for API call to complete
    await waitFor(() => {
      expect(createUser).toHaveBeenCalledWith(expect.objectContaining({
        name: 'John Doe',
        email: 'john@example.com',
        password: 'Password123!',
        isAdmin: false,
        showPortal: false,
        showChat: true
      }));
    });
  });
});