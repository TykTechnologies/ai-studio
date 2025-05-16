import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import BasicCard from '../BasicCard';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Create a simple mock theme for testing
const mockTheme = createTheme({
  palette: {
    background: {
      paper: '#ffffff',
    },
    border: {
      neutralDefault: '#cccccc',
      neutralDefaultSubdued: '#eeeeee',
      neutralHovered: '#999999',
    },
    text: {
      defaultSubdued: '#666666',
      primary: '#000000',
    },
  },
  spacing: (factor) => `${0.25 * factor}rem`,
});

// Wrap component with ThemeProvider for styled components
const renderWithTheme = (ui) => {
  return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
};

describe('BasicCard Component', () => {
  test('renders children content', () => {
    renderWithTheme(
      <BasicCard>
        <div data-testid="child-content">Test Content</div>
      </BasicCard>
    );
    
    expect(screen.getByTestId('child-content')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders primary action button when provided', () => {
    const primaryAction = {
      label: 'Primary Action',
      onClick: jest.fn()
    };
    
    renderWithTheme(
      <BasicCard primaryAction={primaryAction}>
        <div>Test Content</div>
      </BasicCard>
    );
    
    expect(screen.getByText('Primary Action')).toBeInTheDocument();
  });

  test('renders secondary action button when provided', () => {
    const secondaryAction = {
      label: 'Secondary Action',
      onClick: jest.fn()
    };
    
    renderWithTheme(
      <BasicCard secondaryAction={secondaryAction}>
        <div>Test Content</div>
      </BasicCard>
    );
    
    expect(screen.getByText('Secondary Action')).toBeInTheDocument();
  });

  test('renders both primary and secondary action buttons when provided', () => {
    const primaryAction = {
      label: 'Primary Action',
      onClick: jest.fn()
    };
    
    const secondaryAction = {
      label: 'Secondary Action',
      onClick: jest.fn()
    };
    
    renderWithTheme(
      <BasicCard 
        primaryAction={primaryAction}
        secondaryAction={secondaryAction}
      >
        <div>Test Content</div>
      </BasicCard>
    );
    
    expect(screen.getByText('Primary Action')).toBeInTheDocument();
    expect(screen.getByText('Secondary Action')).toBeInTheDocument();
  });

  test('calls onClick handler when primary action button is clicked', () => {
    const handleClick = jest.fn();
    const primaryAction = {
      label: 'Primary Action',
      onClick: handleClick
    };
    
    renderWithTheme(
      <BasicCard primaryAction={primaryAction}>
        <div>Test Content</div>
      </BasicCard>
    );
    
    fireEvent.click(screen.getByText('Primary Action'));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  test('calls onClick handler when secondary action button is clicked', () => {
    const handleClick = jest.fn();
    const secondaryAction = {
      label: 'Secondary Action',
      onClick: handleClick
    };
    
    renderWithTheme(
      <BasicCard secondaryAction={secondaryAction}>
        <div>Test Content</div>
      </BasicCard>
    );
    
    fireEvent.click(screen.getByText('Secondary Action'));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  test('disables action buttons when disabled prop is set on each action', () => {
    const primaryAction = {
      label: 'Primary Action',
      onClick: jest.fn(),
      disabled: true
    };
    
    const secondaryAction = {
      label: 'Secondary Action',
      onClick: jest.fn(),
      disabled: true
    };
    
    renderWithTheme(
      <BasicCard 
        primaryAction={primaryAction}
        secondaryAction={secondaryAction}
      >
        <div>Test Content</div>
      </BasicCard>
    );
    
    const primaryButton = screen.getByText('Primary Action');
    const secondaryButton = screen.getByText('Secondary Action');
    
    expect(primaryButton).toBeDisabled();
    expect(secondaryButton).toBeDisabled();
    
    fireEvent.click(primaryButton);
    fireEvent.click(secondaryButton);
    
    expect(primaryAction.onClick).not.toHaveBeenCalled();
    expect(secondaryAction.onClick).not.toHaveBeenCalled();
  });

  test('does not render action buttons when no actions are provided', () => {
    renderWithTheme(
      <BasicCard>
        <div data-testid="child-content">Test Content</div>
      </BasicCard>
    );
    
    // Check that no buttons are rendered
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});