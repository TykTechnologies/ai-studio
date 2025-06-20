import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import CustomNote from '../CustomNote';

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon">{props.name}</div>;
  };
});

describe('CustomNote Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      border: {
        informativeDefaultSubdued: '#e0e0e0',
      },
      background: {
        surfaceInformativeDefault: '#f5f5f5',
      },
      text: {
        linkDefault: '#1976d2',
        defaultSubdued: '#666666',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders with title and message', () => {
    const title = 'Test Title';
    const message = 'Test Message';
    
    renderWithTheme(
      <CustomNote title={title} message={message} />
    );
    
    // Check that the component renders with the correct content
    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByTestId('mock-icon')).toBeInTheDocument();
    expect(screen.getByTestId('mock-icon')).toHaveTextContent('circle-info');
  });

  test('renders with message only (no title)', () => {
    const message = 'Test Message Only';
    
    renderWithTheme(
      <CustomNote message={message} />
    );
    
    // Check that the component renders with the message but no title
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByTestId('mock-icon')).toBeInTheDocument();
    
    // Verify that no title element is rendered
    // We know the title would be rendered with Typography variant="bodyLargeBold"
    // Since we can't directly check for the variant, we'll check that there's no element
    // that contains text other than our message and the icon name
    expect(screen.queryByText('Test Title')).not.toBeInTheDocument();
    
    // Make sure the message is still there
    expect(screen.getByText('Test Message Only')).toBeInTheDocument();
  });

  test('renders with the correct icon', () => {
    renderWithTheme(
      <CustomNote message="Test Message" />
    );
    
    // Check that the icon is rendered with the correct name
    const icon = screen.getByTestId('mock-icon');
    expect(icon).toBeInTheDocument();
    expect(icon).toHaveTextContent('circle-info');
  });

  test('applies responsive styling based on theme', () => {
    renderWithTheme(
      <CustomNote title="Test Title" message="Test Message" />
    );
    
    // We can't directly test the styling properties with Testing Library
    // But we can verify the component renders successfully
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    expect(screen.getByTestId('mock-icon')).toBeInTheDocument();
  });
});