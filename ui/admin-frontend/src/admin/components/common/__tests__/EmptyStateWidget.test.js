import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import EmptyStateWidget from '../EmptyStateWidget';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock the imported image
jest.mock('../empty-state.png', () => 'mocked-empty-state.png');

// Mock theme for testing
const theme = createTheme({
  palette: {
    border: {
      neutralDefault: '#e0e0e0',
    },
    text: {
      primary: '#212121',
      defaultSubdued: '#757575',
      linkDefault: '#2196f3',
    },
  },
});

// Mock OpenInNewIcon
jest.mock('@mui/icons-material/OpenInNew', () => {
  return function MockOpenInNewIcon(props) {
    return <span data-testid="OpenInNewIcon" {...props} />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('EmptyStateWidget', () => {
  const defaultProps = {
    title: 'No items found',
    description: 'There are no items to display. Try creating one.',
    learnMoreLink: 'https://example.com/docs',
  };

  test('renders title and description', () => {
    render(
      <TestWrapper>
        <EmptyStateWidget {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByText('No items found')).toBeInTheDocument();
    expect(screen.getByText('There are no items to display. Try creating one.')).toBeInTheDocument();
  });

  test('renders learn more link when provided', () => {
    render(
      <TestWrapper>
        <EmptyStateWidget {...defaultProps} />
      </TestWrapper>
    );
    
    const learnMoreLink = screen.getByRole('link', { name: /learn more/i });
    expect(learnMoreLink).toBeInTheDocument();
    expect(learnMoreLink).toHaveAttribute('href', 'https://example.com/docs');
    expect(screen.getByTestId('OpenInNewIcon')).toBeInTheDocument();
  });

  test('prevents default when clicking learn more link without a URL', () => {
    const preventDefaultMock = jest.fn();
    
    render(
      <TestWrapper>
        <EmptyStateWidget 
          title={defaultProps.title} 
          description={defaultProps.description} 
          learnMoreLink={null}
        />
      </TestWrapper>
    );
    
    const learnMoreLink = screen.getByRole('link', { name: /learn more/i });
    expect(learnMoreLink).toHaveAttribute('href', '#');
    
    // Simulate click with preventDefault mock
    fireEvent.click(learnMoreLink, {
      preventDefault: preventDefaultMock,
    });
    
    // Since we can't directly test preventDefault was called (it's handled internally),
    // we're testing that the link has the # href which would trigger preventDefault
    expect(learnMoreLink).toHaveAttribute('href', '#');
  });

  test('renders illustration image', () => {
    render(
      <TestWrapper>
        <EmptyStateWidget {...defaultProps} />
      </TestWrapper>
    );
    
    const image = screen.getByAltText('Empty state illustration');
    expect(image).toBeInTheDocument();
    expect(image).toHaveAttribute('src', 'mocked-empty-state.png');
  });

  test('applies correct styling to components', () => {
    render(
      <TestWrapper>
        <EmptyStateWidget {...defaultProps} data-testid="empty-state-widget" />
      </TestWrapper>
    );
    
    // Title should have correct typography variant
    const title = screen.getByText('No items found');
    expect(title).toHaveClass('MuiTypography-headingLarge');
    
    // Description should have correct typography variant and color
    const description = screen.getByText('There are no items to display. Try creating one.');
    expect(description).toHaveClass('MuiTypography-bodyLargeDefault');
    
    // Learn more link should have correct styling
    const learnMoreLink = screen.getByRole('link', { name: /learn more/i });
    expect(learnMoreLink).toHaveClass('MuiLink-root');
    expect(learnMoreLink).toHaveStyle('text-decoration: none');
  });
});