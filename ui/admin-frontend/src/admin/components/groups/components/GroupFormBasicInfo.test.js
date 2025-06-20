import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupFormBasicInfo from './GroupFormBasicInfo';

jest.mock('../../../utils/docsLinkUtils', () => ({
  createDocsLinkHandler: jest.fn().mockImplementation((getDocsLink, section) => () => {})
}));

// Mock Material-UI components
jest.mock('@mui/material', () => ({
  Typography: ({ children, variant, color, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} {...props}>
      {children}
    </div>
  ),
  Box: ({ children, sx, ...props }) => (
    <div data-testid="box" data-sx={JSON.stringify(sx)} {...props}>
      {children}
    </div>
  ),
}));

// Mock StyledTextField
jest.mock('../../../styles/sharedStyles', () => ({
  StyledTextField: ({ name, value, onChange, error, helperText, required, autoComplete, fullWidth, ...props }) => (
    <input
      data-testid="styled-text-field"
      name={name}
      value={value || ''}
      onChange={onChange}
      data-error={error}
      data-helper-text={helperText}
      data-required={required}
      data-autocomplete={autoComplete}
      data-fullwidth={fullWidth}
      {...props}
    />
  ),
  LearnMoreLink: ({ onClick, ...props }) => (
    <a data-testid="learn-more-link" onClick={onClick} {...props}>
      Learn More
    </a>
  ),
}));

// Mock Section component
jest.mock('../../common/Section', () => ({
  __esModule: true,
  default: ({ children, ...props }) => (
    <div data-testid="section" {...props}>
      {children}
    </div>
  )
}));

describe('GroupFormBasicInfo Component', () => {
  const mockName = 'Test Team';
  const mockSetName = jest.fn();
  const mockGetDocsLink = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders section with descriptive text', () => {
    render(
      <GroupFormBasicInfo
        name={mockName}
        setName={mockSetName}
        getDocsLink={mockGetDocsLink}
      />
    );
    
    // Check that the Section component is rendered
    const section = screen.getByTestId('section');
    expect(section).toBeInTheDocument();
    
    // Check that the descriptive text is rendered
    const description = screen.getByText(/Teams help you organize users and easily manage their access/i);
    expect(description).toBeInTheDocument();
    const learnMoreLink = screen.getByTestId('learn-more-link');
    expect(learnMoreLink).toBeInTheDocument();
  });

  test('renders team name field with correct props', () => {
    render(
      <GroupFormBasicInfo
        name={mockName}
        setName={mockSetName}
        getDocsLink={mockGetDocsLink}
      />
    );
    
    // Check for the label
    const label = screen.getByText(/Team name\*/i);
    expect(label).toBeInTheDocument();
    
    // Check for the text field with correct props
    const textField = screen.getByTestId('styled-text-field');
    expect(textField).toBeInTheDocument();
    expect(textField).toHaveAttribute('name', 'name');
    expect(textField).toHaveAttribute('value', mockName);
    expect(textField).toHaveAttribute('data-required', 'true');
    expect(textField).toHaveAttribute('data-autocomplete', 'off');
    expect(textField).toHaveAttribute('data-fullwidth', 'true');
  });

  test('calls setName when the text field value changes', () => {
    render(
      <GroupFormBasicInfo
        name={mockName}
        setName={mockSetName}
        getDocsLink={mockGetDocsLink}
      />
    );
    
    const textField = screen.getByTestId('styled-text-field');
    fireEvent.change(textField, { target: { value: 'New Team Name' } });
    
    expect(mockSetName).toHaveBeenCalledTimes(1);
    expect(mockSetName).toHaveBeenCalledWith('New Team Name');
  });

  test('displays error message when error prop is provided', () => {
    const errorMessage = 'Team name is required';
    
    render(
      <GroupFormBasicInfo
        name={mockName}
        setName={mockSetName}
        error={errorMessage}
        getDocsLink={mockGetDocsLink}
      />
    );
    
    const textField = screen.getByTestId('styled-text-field');
    expect(textField).toHaveAttribute('data-error', 'true');
    expect(textField).toHaveAttribute('data-helper-text', errorMessage);
  });

  test('does not display error when error prop is not provided', () => {
    render(
      <GroupFormBasicInfo
        name={mockName}
        setName={mockSetName}
        getDocsLink={mockGetDocsLink}
      />
    );
    
    const textField = screen.getByTestId('styled-text-field');
    expect(textField).toHaveAttribute('data-error', 'false');
    expect(textField.getAttribute('data-helper-text')).toBeFalsy();
  });
});