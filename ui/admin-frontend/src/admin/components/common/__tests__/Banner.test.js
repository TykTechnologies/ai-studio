import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import Banner from '../Banner';

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name, sx, ...props }) => (
    <div data-testid="mocked-icon" data-icon-name={name} style={sx} {...props} />
  )
}));

// Mock CloseIcon from MUI
jest.mock('@mui/icons-material/Close', () => ({
  __esModule: true,
  default: () => <div data-testid="close-icon" />
}));

describe('Banner Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      text: {
        defaultSubdued: '#666666',
        warningDefault: '#F57C00',
        successDefault: '#388E3C'
      },
      background: {
        iconWarningDefault: '#FFA500',
        surfaceWarningDefault: '#FFF3E0',
        iconSuccessDefault: '#4CAF50',
        surfaceSuccessDefault: '#E8F5E9'
      },
      border: {
        warningDefaultSubdued: '#FFE0B2',
        successDefaultSubdued: '#C8E6C9'
      },
      primary: {
        main: '#0066CC'
      }
    }
  });

  // Helper function to render the Banner with theme
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders with required props', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByTestId('mocked-icon')).toBeInTheDocument();
    expect(screen.getByTestId('mocked-icon')).toHaveAttribute('data-icon-name', 'triangle-exclamation');
  });

  test('renders with message', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
  });

  test('renders with link when linkText and linkUrl are provided', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        linkText="Click here"
        linkUrl="https://example.com"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    const link = screen.getByRole('link', { name: 'Click here' });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', 'https://example.com');
  });

  test('does not render link if only linkText is provided without linkUrl', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        linkText="Click here"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.queryByText('Click here')).not.toBeInTheDocument();
  });

  test('does not render link if only linkUrl is provided without linkText', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        linkUrl="https://example.com"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  test('renders close button when onClose is provided', () => {
    const handleClose = jest.fn();
    renderWithTheme(
      <Banner
        title="Test Title"
        onClose={handleClose}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    const closeButton = screen.getByRole('button');
    expect(closeButton).toBeInTheDocument();
    expect(screen.getByTestId('close-icon')).toBeInTheDocument();
    
    fireEvent.click(closeButton);
    expect(handleClose).toHaveBeenCalledTimes(1);
  });

  test('does not render close button when showCloseButton is false', () => {
    const handleClose = jest.fn();
    renderWithTheme(
      <Banner
        title="Test Title"
        onClose={handleClose}
        showCloseButton={false}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.queryByTestId('close-icon')).not.toBeInTheDocument();
  });

  test('renders with vertical layout by default', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    const titleElement = screen.getByText('Test Title');
    const messageElement = screen.getByText('Test Message');
    
    // We can't directly test CSS, but we're verifying the elements are rendered
    expect(titleElement).toBeInTheDocument();
    expect(messageElement).toBeInTheDocument();
  });

  test('renders with horizontal layout when horizontalLayout is true', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        message="Test Message"
        horizontalLayout={true}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    // We can only verify elements are rendered, as the layout style is harder to test
    const titleElement = screen.getByText('Test Title');
    const messageElement = screen.getByText('Test Message');
    
    expect(titleElement).toBeInTheDocument();
    expect(messageElement).toBeInTheDocument();
  });

  test('renders with button in vertical layout', () => {
    const mockButton = <button data-testid="test-button">Test Button</button>;
    renderWithTheme(
      <Banner
        title="Test Title"
        button={mockButton}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.getByTestId('test-button')).toBeInTheDocument();
    expect(screen.getByText('Test Button')).toBeInTheDocument();
  });

  test('renders button in horizontal layout', () => {
    const mockButton = <button data-testid="test-button">Test Button</button>;
    renderWithTheme(
      <Banner
        title="Test Title"
        horizontalLayout={true}
        button={mockButton}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
      />
    );
    
    expect(screen.getByTestId('test-button')).toBeInTheDocument();
    expect(screen.getByText('Test Button')).toBeInTheDocument();
  });

  test('applies custom sx styles', () => {
    renderWithTheme(
      <Banner
        title="Test Title"
        sx={{ maxWidth: '500px', padding: '20px' }}
        iconName="triangle-exclamation"
        iconColor="background.iconWarningDefault"
        borderColor="border.warningDefaultSubdued"
        backgroundColor="background.surfaceWarningDefault"
        titleColor="text.warningDefault"
        data-testid="custom-banner"
      />
    );
    
    // Testing that the component renders with our custom props
    expect(screen.getByText('Test Title')).toBeInTheDocument();
  });
});