import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import AdvancedSettingsSection from './AdvancedSettingsSection';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock theme for testing
const theme = createTheme({
  palette: {
    text: {
      defaultSubdued: '#757575',
    },
  },
});

// Mock KeyboardArrowDownIcon and KeyboardArrowUpIcon
jest.mock('@mui/icons-material/KeyboardArrowDown', () => {
  return function MockKeyboardArrowDownIcon(props) {
    return <div data-testid="KeyboardArrowDownIcon" {...props} />;
  };
});

jest.mock('@mui/icons-material/KeyboardArrowUp', () => {
  return function MockKeyboardArrowUpIcon(props) {
    return <div data-testid="KeyboardArrowUpIcon" {...props} />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('AdvancedSettingsSection', () => {
  const testContent = <div data-testid="test-content">Test Content</div>;

  test('renders collapsed by default', () => {
    render(
      <TestWrapper>
        <AdvancedSettingsSection>{testContent}</AdvancedSettingsSection>
      </TestWrapper>
    );
    
    // Should show "Advance settings" text
    expect(screen.getByText('Advance settings')).toBeInTheDocument();
    
    // Content should not be visible
    expect(screen.queryByTestId('test-content')).not.toBeInTheDocument();
    
    // Should show down arrow icon
    expect(screen.getByTestId('KeyboardArrowDownIcon')).toBeInTheDocument();
  });

  test('expands when clicked', () => {
    render(
      <TestWrapper>
        <AdvancedSettingsSection>{testContent}</AdvancedSettingsSection>
      </TestWrapper>
    );
    
    // Click to expand
    fireEvent.click(screen.getByText('Advance settings'));
    
    // Content should now be visible
    expect(screen.getByTestId('test-content')).toBeInTheDocument();
    
    // Should show up arrow icon
    expect(screen.getByTestId('KeyboardArrowUpIcon')).toBeInTheDocument();
  });

  test('collapses when clicked again', () => {
    render(
      <TestWrapper>
        <AdvancedSettingsSection>{testContent}</AdvancedSettingsSection>
      </TestWrapper>
    );
    
    // Click to expand
    fireEvent.click(screen.getByText('Advance settings'));
    
    // Content should be visible
    expect(screen.getByTestId('test-content')).toBeInTheDocument();
    
    // Click again to collapse
    fireEvent.click(screen.getByText('Advance settings'));
    
    // Content should not be visible
    expect(screen.queryByTestId('test-content')).not.toBeInTheDocument();
    
    // Should show down arrow icon again
    expect(screen.getByTestId('KeyboardArrowDownIcon')).toBeInTheDocument();
  });
  test('applies correct styling', () => {
    render(
      <TestWrapper>
        <AdvancedSettingsSection>
          {testContent}
        </AdvancedSettingsSection>
      </TestWrapper>
    );
    
    // Text should have correct color and variant
    const text = screen.getByText('Advance settings');
    expect(text).toHaveClass('MuiTypography-bodyLargeMedium');
    
    // Check that the down arrow icon is rendered initially
    expect(screen.getByTestId('KeyboardArrowDownIcon')).toBeInTheDocument();
  });
});