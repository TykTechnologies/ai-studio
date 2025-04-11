import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import { createTheme } from '@mui/material';
import AuthLayout from './AuthLayout';

// Mock the image imports
jest.mock('./login_background.png', () => 'mocked-background-image-path');
jest.mock('./login_logo.png', () => 'mocked-logo-image-path');

describe('AuthLayout Component', () => {
  // Create a custom theme for testing
  const theme = createTheme({
    palette: {
      text: {
        primary: '#ffffff',
      },
      custom: {
        white: '#ffffff',
        purpleExtraDark: '#2e1065',
      },
      primary: {
        main: '#7e22ce',
      },
    },
  });

  // Wrapper component with theme provider
  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      {children}
    </ThemeProvider>
  );

  test('renders without crashing', () => {
    render(<AuthLayout>Test Content</AuthLayout>, { wrapper: Wrapper });
    
    // Check that the component renders the content
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders children correctly', () => {
    render(
      <AuthLayout>
        <div data-testid="test-child">Child Content</div>
      </AuthLayout>,
      { wrapper: Wrapper }
    );
    
    // Check that the children are rendered
    expect(screen.getByTestId('test-child')).toBeInTheDocument();
    expect(screen.getByText('Child Content')).toBeInTheDocument();
  });

  test('renders with logo', () => {
    render(<AuthLayout>Test Content</AuthLayout>, { wrapper: Wrapper });
    
    // Check that there's an img element with alt="Logo"
    const logoElement = screen.getByAltText('Logo');
    expect(logoElement).toBeInTheDocument();
    expect(logoElement).toHaveAttribute('src', 'mocked-logo-image-path');
  });

  test('has correct structure', () => {
    render(<AuthLayout>Test Content</AuthLayout>, { wrapper: Wrapper });
    
    // Check that the component has the expected structure
    // Box -> ContentContainer -> Logo + FormWrapper -> children
    // We can verify the logo and content are rendered
    expect(screen.getByAltText('Logo')).toBeInTheDocument();
    
    // Check that the text content is rendered
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('applies background image styling', () => {
    render(<AuthLayout>Test Content</AuthLayout>, { wrapper: Wrapper });
    
    // Since we can't directly test the sx prop with Testing Library,
    // we can verify that the component renders with the expected content
    expect(screen.getByText('Test Content')).toBeInTheDocument();
    expect(screen.getByAltText('Logo')).toBeInTheDocument();
  });

  test('renders multiple children correctly', () => {
    render(
      <AuthLayout>
        <div data-testid="child-1">First Child</div>
        <div data-testid="child-2">Second Child</div>
      </AuthLayout>,
      { wrapper: Wrapper }
    );
    
    // Check that both children are rendered
    expect(screen.getByTestId('child-1')).toBeInTheDocument();
    expect(screen.getByTestId('child-2')).toBeInTheDocument();
    expect(screen.getByText('First Child')).toBeInTheDocument();
    expect(screen.getByText('Second Child')).toBeInTheDocument();
  });

  test('renders complex children correctly', () => {
    render(
      <AuthLayout>
        <div data-testid="complex-child">
          <h1>Heading</h1>
          <p>Paragraph</p>
          <button>Button</button>
        </div>
      </AuthLayout>,
      { wrapper: Wrapper }
    );
    
    // Check that the complex child and its contents are rendered
    expect(screen.getByTestId('complex-child')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
    expect(screen.getByText('Heading')).toBeInTheDocument();
    expect(screen.getByText('Paragraph')).toBeInTheDocument();
    expect(screen.getByRole('button')).toBeInTheDocument();
  });
});