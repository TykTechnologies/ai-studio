import React from 'react';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import UserFormBasicInfo from './UserFormBasicInfo';

jest.mock('react-router-dom', () => ({
  useParams: jest.fn()
}));
jest.mock('@mui/material', () => require('../../../../test-utils/mui-mocks').muiMaterialMock);
jest.mock('../../common/CollapsibleSection', () => require('../../../../test-utils/component-mocks').collapsibleSectionMock.default);
jest.mock('../../common/InfoTooltip', () => ({
  __esModule: true,
  default: () => {
    const React = require('react');
    return React.createElement('div', { 'data-testid': 'info-tooltip' });
  }
}));
jest.mock('../../../styles/sharedStyles', () => require('../../../../test-utils/styled-component-mocks').sharedStylesMock);
jest.mock('../../../../portal/styles/authStyles', () => require('../../../../test-utils/styled-component-mocks').authStylesMock);
jest.mock('../hooks/useFormValidation', () => ({
  useFormValidation: (value, isPassword, isEmail) => ({
    error: value === 'invalid@email' && isEmail ? 'Invalid email' : 
           value === 'weak' && isPassword ? 'Password too weak' : '',
    handleChange: jest.fn()
  })
}));

describe('UserFormBasicInfo', () => {
  const defaultProps = {
    name: '',
    setName: jest.fn(),
    email: '',
    setEmail: jest.fn(),
    password: '',
    setPassword: jest.fn(),
    emailVerified: false,
    setEmailVerified: jest.fn(),
    setBasicInfoValid: jest.fn()
  };

  beforeEach(() => {
    jest.clearAllMocks();
    require('react-router-dom').useParams.mockReturnValue({ id: undefined });
  });

  const renderComponent = (props = {}) => {
    const mergedProps = { ...defaultProps, ...props };
    return renderWithTheme(<UserFormBasicInfo {...mergedProps} />);
  };

  const renderComponentWithParams = (props = {}, params = {}) => {
    const mergedProps = { ...defaultProps, ...props };
    require('react-router-dom').useParams.mockReturnValue(params);
    return renderWithTheme(<UserFormBasicInfo {...mergedProps} />);
  };

  it('renders basic information form fields', () => {
    renderComponent();
    
    expect(screen.getByTestId('collapsible-section')).toBeInTheDocument();
    expect(screen.getAllByTestId('styled-text-field')).toHaveLength(3);
    expect(screen.getByTestId('styled-checkbox')).toBeInTheDocument();
    expect(screen.getByTestId('info-tooltip')).toBeInTheDocument();
  });

  it('renders with default expanded collapsible section', () => {
    renderComponent();
    
    const collapsibleSection = screen.getByTestId('collapsible-section');
    expect(collapsibleSection).toHaveAttribute('data-title', 'Basic information*');
    expect(collapsibleSection).toHaveAttribute('data-default-expanded', 'true');
  });

  it('updates name field value', async () => {
    const setName = jest.fn();
    renderComponent({ setName });
    
    const nameInput = screen.getAllByTestId('styled-text-field')[0];
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, 'John Doe');
    
    expect(setName).toHaveBeenCalled();
    expect(setName).toHaveBeenCalledWith('e');
  });

  it('updates email field value and triggers validation', async () => {
    const setEmail = jest.fn();
    renderComponent({ setEmail });
    
    const emailInput = screen.getAllByTestId('styled-text-field')[1];
    fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
    
    expect(setEmail).toHaveBeenCalledWith('test@example.com');
  });

  it('updates password field value and triggers validation', async () => {
    const setPassword = jest.fn();
    renderComponent({ setPassword });
    
    const passwordInput = screen.getAllByTestId('styled-text-field')[2];
    fireEvent.change(passwordInput, { target: { value: 'newpassword' } });
    
    expect(setPassword).toHaveBeenCalledWith('newpassword');
  });

  it('toggles email verified checkbox', async () => {
    const setEmailVerified = jest.fn();
    renderComponent({ emailVerified: false, setEmailVerified });
    
    const checkbox = screen.getByTestId('styled-checkbox');
    fireEvent.click(checkbox);
    
    expect(setEmailVerified).toHaveBeenCalledWith(true);
  });

  it('shows email verification checkbox as checked when emailVerified is true', () => {
    renderComponent({ emailVerified: true });
    
    const checkbox = screen.getByTestId('styled-checkbox');
    expect(checkbox).toHaveAttribute('data-checked', 'true');
    expect(checkbox).toHaveAttribute('data-label', 'Email address verified');
  });

  it('displays email error when email is invalid', () => {
    renderComponent({ email: 'invalid@email' });
    
    const emailInput = screen.getAllByTestId('styled-text-field')[1];
    expect(emailInput).toHaveAttribute('data-error', 'true');
  });

  it('displays password error when password is weak', () => {
    renderComponent({ password: 'weak' });
    
    const passwordInput = screen.getAllByTestId('styled-text-field')[2];
    expect(passwordInput).toHaveAttribute('data-error', 'true');
  });

  it('sets basic info as valid when all required fields are filled for new user', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponent({
      name: 'John Doe',
      email: 'test@example.com',
      password: 'password123',
      setBasicInfoValid
    });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(true);
    });
  });

  it('sets basic info as invalid when name is empty', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponent({
      name: '',
      email: 'test@example.com',
      password: 'password123',
      setBasicInfoValid
    });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(false);
    });
  });

  it('sets basic info as invalid when email is empty', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponent({
      name: 'John Doe',
      email: '',
      password: 'password123',
      setBasicInfoValid
    });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(false);
    });
  });

  it('sets basic info as invalid when password is empty for new user', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponent({
      name: 'John Doe',
      email: 'test@example.com',
      password: '',
      setBasicInfoValid
    });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(false);
    });
  });

  it('allows empty password for existing user (edit mode)', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponentWithParams({
      name: 'John Doe',
      email: 'test@example.com',
      password: '',
      setBasicInfoValid
    }, { id: '123' });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(true);
    });
  });

  it('sets basic info as invalid when there are validation errors', async () => {
    const setBasicInfoValid = jest.fn();
    renderComponent({
      name: 'John Doe',
      email: 'invalid@email',
      password: 'password123',
      setBasicInfoValid
    });
    
    await waitFor(() => {
      expect(setBasicInfoValid).toHaveBeenCalledWith(false);
    });
  });

  it('has proper autocomplete attributes on email field', () => {
    renderComponent();
    
    const emailInput = screen.getAllByTestId('styled-text-field')[1];
    expect(emailInput).toHaveAttribute('autoComplete', 'new-email');
    expect(emailInput).toHaveAttribute('data-input-props');
  });

  it('has proper autocomplete attributes on password field', () => {
    renderComponent();
    
    const passwordInput = screen.getAllByTestId('styled-text-field')[2];
    expect(passwordInput).toHaveAttribute('autoComplete', 'new-password');
    expect(passwordInput).toHaveAttribute('data-input-props');
  });

  it('has proper field types', () => {
    renderComponent();
    
    const inputs = screen.getAllByTestId('styled-text-field');
    expect(inputs[1]).toHaveAttribute('type', 'email');
    expect(inputs[2]).toHaveAttribute('type', 'password');
  });

  it('has required attribute on name and email fields', () => {
    renderComponent();
    
    const inputs = screen.getAllByTestId('styled-text-field');
    expect(inputs[0]).toHaveAttribute('required');
    expect(inputs[1]).toHaveAttribute('required');
  });

  it('has autoComplete off on name field', () => {
    renderComponent();
    
    const nameInput = screen.getAllByTestId('styled-text-field')[0];
    expect(nameInput).toHaveAttribute('autoComplete', 'off');
  });
}); 