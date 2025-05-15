import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import IconBadge from '../IconBadge';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return {
    __esModule: true,
    default: ({ name, ...props }) => (
      <div data-testid="mocked-icon" data-icon-name={name} {...props}>
        {name}
      </div>
    ),
  };
});

// Mock the SvgIcon component to add testId for testing
jest.mock('@mui/material', () => {
  const actual = jest.requireActual('@mui/material');
  return {
    ...actual,
    SvgIcon: ({ children, style, ...props }) => (
      <div data-testid="svg-icon" style={style} {...props}>
        {children}
      </div>
    ),
    Box: ({ children, ...props }) => (
      <div data-testid="badge-container" {...props}>
        {children}
      </div>
    ),
  };
});

// Create a mock theme for testing
const mockTheme = createTheme({
  palette: {
    primary: {
      main: '#23E2C2',
    },
    background: {
      surfaceNeutralHover: '#F8F8F9',
    },
    custom: {
      purpleExtraDark: '#5900CB',
    },
  },
});

// Wrap component with ThemeProvider for styled components
const renderWithTheme = (ui) => {
  return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
};

describe('IconBadge Component', () => {
  test('renders with the correct icon name', () => {
    renderWithTheme(<IconBadge iconName="house" />);
    
    const iconElement = screen.getByTestId('mocked-icon');
    expect(iconElement).toBeInTheDocument();
    expect(iconElement).toHaveAttribute('data-icon-name', 'house');
  });

  test('renders with a different icon name', () => {
    renderWithTheme(<IconBadge iconName="users" />);
    
    const iconElement = screen.getByTestId('mocked-icon');
    expect(iconElement).toBeInTheDocument();
    expect(iconElement).toHaveAttribute('data-icon-name', 'users');
  });

  test('renders the badge container', () => {
    renderWithTheme(<IconBadge iconName="house" />);
    
    const badgeContainer = screen.getByTestId('badge-container');
    expect(badgeContainer).toBeInTheDocument();
  });

  test('renders the SVG icon with correct style', () => {
    renderWithTheme(<IconBadge iconName="house" />);
    
    // Check for the presence of the SVG icon
    const svgIcon = screen.getByTestId('svg-icon');
    expect(svgIcon).toBeInTheDocument();
    
    // Check that the SVG icon has the expected style
    expect(svgIcon).toHaveStyle({
      position: 'absolute',
      width: 0,
      height: 0,
    });
  });
  
  test('passes the correct icon name to the Icon component', () => {
    renderWithTheme(<IconBadge iconName="users" />);
    
    // Verify that the Icon component receives the correct name prop
    const iconElement = screen.getByTestId('mocked-icon');
    expect(iconElement).toHaveAttribute('data-icon-name', 'users');
  });
});