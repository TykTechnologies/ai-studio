import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import RadioSelectionGroup from '../RadioSelectionGroup';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Create a mock theme for testing
const mockTheme = createTheme({
  palette: {
    background: {
      paper: '#ffffff',
      buttonPrimaryDefault: '#1976d2',
    },
    border: {
      neutralDefault: '#cccccc',
    },
    text: {
      primary: '#000000',
    },
  },
  spacing: (factor) => `${0.25 * factor}rem`,
});

// Wrap component with ThemeProvider for styled components
const renderWithTheme = (ui) => {
  return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
};

describe('RadioSelectionGroup Component', () => {
  const mockOptions = [
    { value: 'option1', label: 'Option 1' },
    { value: 'option2', label: 'Option 2' },
    { value: 'option3', label: 'Option 3' },
  ];

  const mockOnChange = jest.fn();

  beforeEach(() => {
    mockOnChange.mockClear();
  });

  test('renders all options', () => {
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option1"
        onChange={mockOnChange}
      />
    );
    
    // Check if all options are rendered
    expect(screen.getByText('Option 1')).toBeInTheDocument();
    expect(screen.getByText('Option 2')).toBeInTheDocument();
    expect(screen.getByText('Option 3')).toBeInTheDocument();
    
    // Check if the correct number of radio inputs are rendered
    const radioInputs = screen.getAllByRole('radio');
    expect(radioInputs).toHaveLength(3);
  });

  test('selects the correct option based on value prop', () => {
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option2"
        onChange={mockOnChange}
      />
    );
    
    // Check if the correct radio button is checked
    const radioInputs = screen.getAllByRole('radio');
    expect(radioInputs[0]).not.toBeChecked();
    expect(radioInputs[1]).toBeChecked();
    expect(radioInputs[2]).not.toBeChecked();
  });

  test('calls onChange when a radio button is clicked', () => {
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option1"
        onChange={mockOnChange}
      />
    );
    
    // Click on the second option
    fireEvent.click(screen.getByText('Option 2'));
    
    // Check if onChange was called with the correct event
    expect(mockOnChange).toHaveBeenCalledTimes(1);
    // The first argument to the mock function should be an event object
    expect(mockOnChange.mock.calls[0][0]).toBeTruthy();
    // The event should have a target with a value property equal to 'option2'
    expect(mockOnChange.mock.calls[0][0].target.value).toBe('option2');
  });

  test('renders dividers between options', () => {
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option1"
        onChange={mockOnChange}
      />
    );
    
    // There should be dividers between options (but not after the last option)
    // Use getByRole with a name option to find the separators
    const separators = screen.getAllByRole('separator');
    expect(separators).toHaveLength(mockOptions.length - 1);
  });

  test('renders content when renderContent is provided and option is selected', () => {
    const renderContent = jest.fn(option => (
      <div data-testid={`content-${option.value}`}>
        Content for {option.label}
      </div>
    ));
    
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option2"
        onChange={mockOnChange}
        renderContent={renderContent}
      />
    );
    
    // Check if renderContent was called with the selected option
    expect(renderContent).toHaveBeenCalledWith(mockOptions[1]);
    
    // Check if the content is rendered for the selected option
    expect(screen.getByTestId('content-option2')).toBeInTheDocument();
    expect(screen.getByText('Content for Option 2')).toBeInTheDocument();
    
    // Check that content is not rendered for unselected options
    expect(screen.queryByTestId('content-option1')).not.toBeInTheDocument();
    expect(screen.queryByTestId('content-option3')).not.toBeInTheDocument();
  });

  test('does not render content when renderContent is not provided', () => {
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option2"
        onChange={mockOnChange}
      />
    );
    
    // No content should be rendered
    mockOptions.forEach(option => {
      expect(screen.queryByTestId(`content-${option.value}`)).not.toBeInTheDocument();
    });
  });

  test('does not render content for unselected options even when renderContent is provided', () => {
    const renderContent = jest.fn(option => (
      <div data-testid={`content-${option.value}`}>
        Content for {option.label}
      </div>
    ));
    
    renderWithTheme(
      <RadioSelectionGroup
        options={mockOptions}
        value="option1"
        onChange={mockOnChange}
        renderContent={renderContent}
      />
    );
    
    // Check if renderContent was called with the selected option
    expect(renderContent).toHaveBeenCalledWith(mockOptions[0]);
    
    // Check if the content is rendered for the selected option
    expect(screen.getByTestId('content-option1')).toBeInTheDocument();
    
    // Check that content is not rendered for unselected options
    expect(screen.queryByTestId('content-option2')).not.toBeInTheDocument();
    expect(screen.queryByTestId('content-option3')).not.toBeInTheDocument();
  });
});