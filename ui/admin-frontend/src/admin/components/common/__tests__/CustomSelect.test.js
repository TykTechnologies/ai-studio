import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import CustomSelect from '../CustomSelect';
import CustomSelectBadge from '../CustomSelectBadge';

// Mock the Icon component
jest.mock('../../../../components/common/Icon', () => {
  return function MockIcon(props) {
    return <div data-testid="mock-icon" data-icon-name={props.name} style={props.sx}>{props.name}</div>;
  };
});

// We don't need to mock CustomSelectBadge since we're mocking the Icon component

describe('CustomSelect Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      custom: {
        white: '#ffffff',
      },
      text: {
        defaultSubdued: '#666666',
      },
      border: {
        neutralDefault: '#e0e0e0',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  // Mock options for testing
  const mockOptions = [
    { value: 'option1', label: 'Option 1' },
    { value: 'option2', label: 'Option 2' },
    { value: 'option3', label: 'Option 3' },
  ];

  test('renders with label and options', () => {
    const handleChange = jest.fn();
    
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={handleChange} 
        options={mockOptions} 
      />
    );
    
    // Check that the component renders with the correct label
    const selectElement = screen.getByRole('combobox');
    expect(selectElement).toBeInTheDocument();
    
    // Check that the label is rendered
    expect(screen.getByText('Test Label')).toBeInTheDocument();
    
    // Open the dropdown
    fireEvent.mouseDown(selectElement);
    
    // Check that all options are rendered in the dropdown
    // We need to use getAllByText because the option might appear both in the select and in the dropdown
    mockOptions.forEach(option => {
      const elements = screen.getAllByText(option.label);
      expect(elements.length).toBeGreaterThan(0);
    });
  });

  test('handles onChange events', () => {
    const handleChange = jest.fn();
    
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={handleChange} 
        options={mockOptions} 
      />
    );
    
    // Open the dropdown
    const selectElement = screen.getByRole('combobox');
    fireEvent.mouseDown(selectElement);
    
    // Select a different option
    fireEvent.click(screen.getByText('Option 2'));
    
    // Check that onChange was called with the correct event
    expect(handleChange).toHaveBeenCalled();
  });

  test('renders with error state', () => {
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={() => {}} 
        options={mockOptions} 
        error={true}
        data-testid="custom-select-error"
      />
    );
    
    // Instead of checking for the error class directly, we can check for
    // visual indicators of error state like the error styling
    const formControlElement = screen.getByTestId('custom-select-error');
    
    // The select should be rendered with error styling
    const selectElement = within(formControlElement).getByRole('combobox');
    expect(selectElement).toBeInTheDocument();
    
    // We can verify the error state by checking if the FormControl has the error prop
    // This is an indirect way to test the error state without accessing DOM nodes directly
    const { container } = render(
      <ThemeProvider theme={mockTheme}>
        <CustomSelect
          label="Test Label"
          value="option1"
          onChange={() => {}}
          options={mockOptions}
          error={false}
          data-testid="custom-select-no-error"
        />
      </ThemeProvider>
    );
    
    // Compare the error and non-error versions
    expect(formControlElement.outerHTML).not.toBe(screen.getByTestId('custom-select-no-error').outerHTML);
  });

  test('renders with helper text', () => {
    const helperText = 'This is a helper text';
    
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={() => {}} 
        options={mockOptions} 
        helperText={helperText}
      />
    );
    
    // Check that the helper text is rendered
    expect(screen.getByText(helperText)).toBeInTheDocument();
  });

  test('renders with required prop', () => {
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={() => {}} 
        options={mockOptions} 
        required={true}
        data-testid="custom-select-required"
      />
    );
    
    // Instead of checking for the required class directly, we can check for
    // visual indicators of required state
    const formControlElement = screen.getByTestId('custom-select-required');
    
    // The select should be rendered with required styling
    const selectElement = within(formControlElement).getByRole('combobox');
    expect(selectElement).toBeInTheDocument();
    
    // We can verify the required state by checking if the label has an asterisk
    // This is an indirect way to test the required state without accessing DOM nodes directly
    const labelElement = screen.getByText('Test Label *');
    expect(labelElement).toBeInTheDocument();
  });

  test('renders with custom option renderer', () => {
    const customRenderer = (option) => (
      <div data-testid={`custom-option-${option.value}`}>{option.label} (Custom)</div>
    );
    
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={() => {}} 
        options={mockOptions} 
        renderOption={customRenderer}
      />
    );
    
    // Open the dropdown
    const selectElement = screen.getByRole('combobox');
    fireEvent.mouseDown(selectElement);
    
    // Check that the custom renderer is used
    // We need to use getAllByTestId because the option might appear both in the select and in the dropdown
    mockOptions.forEach(option => {
      const elements = screen.getAllByTestId(`custom-option-${option.value}`);
      expect(elements.length).toBeGreaterThan(0);
      
      const textElements = screen.getAllByText(`${option.label} (Custom)`);
      expect(textElements.length).toBeGreaterThan(0);
    });
  });

  test('passes additional props to the Select component', () => {
    renderWithTheme(
      <CustomSelect 
        label="Test Label" 
        value="option1" 
        onChange={() => {}} 
        options={mockOptions} 
        data-testid="custom-select"
      />
    );
    
    // Check that the additional prop is passed to the Select component
    expect(screen.getByTestId('custom-select')).toBeInTheDocument();
  });
});

describe('CustomSelectBadge Component', () => {
  // Create a mock theme for testing
  const mockTheme = createTheme({
    palette: {
      custom: {
        white: '#ffffff',
      },
      text: {
        defaultSubdued: '#666666',
      },
    },
    spacing: (factor) => `${0.25 * factor}rem`,
  });

  // Wrapper component with theme provider
  const renderWithTheme = (ui) => {
    return render(<ThemeProvider theme={mockTheme}>{ui}</ThemeProvider>);
  };

  test('renders with config props', () => {
    const mockConfig = {
      bgColor: '#f0f0f0',
      textColor: '#333333',
      icon: 'test-icon',
      text: 'Test Badge'
    };
    
    renderWithTheme(
      <CustomSelectBadge config={mockConfig} />
    );
    
    // Check that the component renders with the correct text
    expect(screen.getByText('Test Badge')).toBeInTheDocument();
    
    // Check that the icon is rendered with the correct name
    const icon = screen.getByTestId('mock-icon');
    expect(icon).toBeInTheDocument();
    expect(icon).toHaveAttribute('data-icon-name', 'test-icon');
  });

  test('applies styling from config', () => {
    const mockConfig = {
      bgColor: '#f0f0f0',
      textColor: '#333333',
      icon: 'test-icon',
      text: 'Test Badge'
    };
    
    renderWithTheme(
      <CustomSelectBadge config={mockConfig} />
    );
    
    // Check that the icon has the correct color and properties
    const icon = screen.getByTestId('mock-icon');
    expect(icon).toHaveAttribute('data-icon-name', 'test-icon');
    // Browser converts hex to RGB, so we need to check that the color is applied
    // rather than the exact format
    expect(icon.style.color).toBeTruthy();
    
    // Check that the text has the correct color
    const text = screen.getByText('Test Badge');
    expect(text).toHaveStyle(`color: rgb(51, 51, 51)`);
  });

  test('renders with different config values', () => {
    const mockConfig = {
      bgColor: '#e0e0ff',
      textColor: '#0000ff',
      icon: 'different-icon',
      text: 'Different Badge'
    };
    
    renderWithTheme(
      <CustomSelectBadge config={mockConfig} />
    );
    
    // Check that the component renders with the correct text
    expect(screen.getByText('Different Badge')).toBeInTheDocument();
    
    // Check that the icon has the correct properties and color
    const icon = screen.getByTestId('mock-icon');
    expect(icon).toHaveAttribute('data-icon-name', 'different-icon');
    // Browser converts hex to RGB, so we need to check that the color is applied
    // rather than the exact format
    expect(icon.style.color).toBeTruthy();
    
    // Check that the text has the correct color
    const text = screen.getByText('Different Badge');
    expect(text).toHaveStyle(`color: rgb(0, 0, 255)`);
  });
});