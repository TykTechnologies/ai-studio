import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import SuccessBanner from './SuccessBanner';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock theme for testing
const theme = createTheme({
  palette: {
    border: {
      successDefaultSubdued: '#c8e6c9',
    },
    background: {
      surfaceSuccessDefault: '#e8f5e9',
      iconSuccessDefault: '#4caf50',
    },
    text: {
      successDefault: '#2e7d32',
      defaultSubdued: '#757575',
    },
    primary: {
      main: '#2196f3',
    },
  },
});

// Mock the Icon component
jest.mock('../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid={`icon-${props.name}`} {...props} />;
  };
});

// Mock CloseIcon
jest.mock('@mui/icons-material/Close', () => {
  return function MockCloseIcon(props) {
    return <div data-testid="CloseIcon" {...props} />;
  };
});

const TestWrapper = ({ children }) => (
  <ThemeProvider theme={theme}>{children}</ThemeProvider>
);

describe('SuccessBanner', () => {
  const defaultProps = {
    title: 'Success!',
    message: 'Operation completed successfully.',
    onClose: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders title and message', () => {
    render(
      <TestWrapper>
        <SuccessBanner {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByText('Success!')).toBeInTheDocument();
    expect(screen.getByText('Operation completed successfully.')).toBeInTheDocument();
  });

  test('renders success icon', () => {
    render(
      <TestWrapper>
        <SuccessBanner {...defaultProps} />
      </TestWrapper>
    );
    
    expect(screen.getByTestId('icon-hexagon-check')).toBeInTheDocument();
  });

  test('calls onClose when close button is clicked', () => {
    render(
      <TestWrapper>
        <SuccessBanner {...defaultProps} />
      </TestWrapper>
    );
    
    const closeButton = screen.getByRole('button', { name: '' });
    fireEvent.click(closeButton);
    
    expect(defaultProps.onClose).toHaveBeenCalledTimes(1);
  });

  test('renders link when linkText and linkUrl are provided', () => {
    const propsWithLink = {
      ...defaultProps,
      linkText: 'View details',
      linkUrl: 'https://example.com/details',
    };
    
    render(
      <TestWrapper>
        <SuccessBanner {...propsWithLink} />
      </TestWrapper>
    );
    
    const link = screen.getByText('View details');
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', 'https://example.com/details');
  });

  test('does not render link when linkText or linkUrl is missing', () => {
    render(
      <TestWrapper>
        <SuccessBanner {...defaultProps} />
      </TestWrapper>
    );
    
    // No link should be rendered with the text "View details"
    expect(screen.queryByText('View details')).not.toBeInTheDocument();
    // No links should be rendered at all
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
  });

  test('applies correct styling to components', () => {
    render(
      <TestWrapper>
        <SuccessBanner {...defaultProps} />
      </TestWrapper>
    );
    
    // Title should have success text color
    const title = screen.getByText('Success!');
    expect(title).toHaveClass('MuiTypography-headingSmall');
    
    // Message should have subdued text color
    const message = screen.getByText('Operation completed successfully.');
    expect(message).toHaveClass('MuiTypography-bodyMediumDefault');
    
    // Check that the success icon is rendered
    expect(screen.getByTestId('icon-hexagon-check')).toBeInTheDocument();
    
    // Check that the close button is rendered
    expect(screen.getByRole('button')).toBeInTheDocument();
  });
});