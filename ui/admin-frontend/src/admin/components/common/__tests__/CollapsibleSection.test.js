import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import CollapsibleSection from '../CollapsibleSection';

// Mock the MUI icons
jest.mock('@mui/icons-material/KeyboardArrowDown', () => {
  return function MockKeyboardArrowDownIcon() {
    return <div data-testid="KeyboardArrowDownIcon" />;
  };
});

jest.mock('@mui/icons-material/KeyboardArrowUp', () => {
  return function MockKeyboardArrowUpIcon() {
    return <div data-testid="KeyboardArrowUpIcon" />;
  };
});

// Mock KeyboardArrowDownIcon and KeyboardArrowUpIcon
jest.mock('@mui/icons-material/KeyboardArrowDown', () => {
  return function MockKeyboardArrowDownIcon() {
    return <div data-testid="KeyboardArrowDownIcon" />;
  };
});

jest.mock('@mui/icons-material/KeyboardArrowUp', () => {
  return function MockKeyboardArrowUpIcon() {
    return <div data-testid="KeyboardArrowUpIcon" />;
  };
});

// Create a mock for styled components
jest.mock('@mui/material/styles', () => {
  const originalModule = jest.requireActual('@mui/material/styles');
  
  return {
    ...originalModule,
    styled: (Component) => (styleFunction) => {
      return function StyledComponent(props) {
        // Extract any props that would cause DOM warnings
        const { isExpanded, ...rest } = props;
        return <Component data-testid={`styled-${Component.displayName || 'component'}`} {...rest} />;
      };
    }
  };
});

// Mock theme for testing
const theme = createTheme({
  palette: {
    border: {
      neutralDefault: '#e0e0e0',
      neutralHovered: '#c0c0c0',
    },
    text: {
      primary: '#000000',
    },
  },
  spacing: (value) => value * 8,
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('CollapsibleSection', () => {
  const defaultProps = {
    title: 'Test Section',
    children: <div data-testid="section-content">Content</div>,
  };

  test('renders with title', () => {
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByText('Test Section')).toBeInTheDocument();
  });

  test('renders content when expanded by default', () => {
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByTestId('section-content')).toBeInTheDocument();
  });

  test('hides content when defaultExpanded is false', () => {
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} defaultExpanded={false} />
      </TestWrapper>
    );
    
    expect(screen.queryByTestId('section-content')).not.toBeInTheDocument();
  });

  test('toggles content visibility when header is clicked', () => {
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} />
      </TestWrapper>
    );
    
    // Content should be visible initially
    expect(screen.getByTestId('section-content')).toBeInTheDocument();
    
    // Click header to collapse
    fireEvent.click(screen.getByText('Test Section'));
    expect(screen.queryByTestId('section-content')).not.toBeInTheDocument();
    
    // Click header again to expand
    fireEvent.click(screen.getByText('Test Section'));
    expect(screen.getByTestId('section-content')).toBeInTheDocument();
  });

  test('applies custom styles via sx prop', () => {
    const customSx = { marginTop: '20px' };
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} sx={customSx} />
      </TestWrapper>
    );
    
    // Since our mock adds the same data-testid to multiple elements,
    // we need to use getAllByTestId and check the first one (the Paper component)
    const styledComponents = screen.getAllByTestId('styled-component');
    const sectionContainer = styledComponents[0]; // The Paper/SectionContainer is the first one
    
    expect(sectionContainer).toHaveStyle('margin-top: 20px');
  });

  test('shows correct icon based on expanded state', () => {
    render(
      <TestWrapper>
        <CollapsibleSection {...defaultProps} />
      </TestWrapper>
    );
    
    // Initially expanded, should show up arrow
    expect(screen.getByTestId('KeyboardArrowUpIcon')).toBeInTheDocument();
    expect(screen.queryByTestId('KeyboardArrowDownIcon')).not.toBeInTheDocument();
    
    // Click to collapse
    fireEvent.click(screen.getByText('Test Section'));
    
    // Should now show down arrow
    expect(screen.queryByTestId('KeyboardArrowUpIcon')).not.toBeInTheDocument();
    expect(screen.getByTestId('KeyboardArrowDownIcon')).toBeInTheDocument();
  });
});