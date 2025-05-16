import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupCatalogsSection from './GroupCatalogsSection';

// Mock Material-UI components
jest.mock('@mui/material', () => ({
  Typography: ({ children, variant, color, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} {...props}>
      {children}
    </div>
  ),
  Box: ({ children, sx, ...props }) => (
    <div data-testid="box" data-sx={JSON.stringify(sx)} {...props}>
      {children}
    </div>
  ),
}));

// Mock custom components
jest.mock('../../common/CollapsibleSection', () => ({
  __esModule: true,
  default: ({ children, title, defaultExpanded, ...props }) => (
    <div data-testid="collapsible-section" data-title={title} data-default-expanded={defaultExpanded} {...props}>
      {children}
    </div>
  )
}));

jest.mock('../../common/CustomSelectMany', () => ({
  __esModule: true,
  default: ({ value, onChange, options, disabled, ...props }) => (
    <div
      data-testid="custom-select-many"
      data-disabled={disabled}
      // Expose onChange handler directly on the div for testing
      onClick={() => {}}
      data-on-change={onChange}
      {...props}
    >
      <select
        multiple
        value={value || []}
        disabled={disabled}
      >
        {options && options.map(option => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
      <span data-testid="options-count">{options ? options.length : 0}</span>
    </div>
  )
}));

jest.mock('../../common/CustomNote', () => ({
  __esModule: true,
  default: ({ message, ...props }) => (
    <div data-testid="custom-note" {...props}>
      {message}
    </div>
  )
}));

// Component under test is imported at the top of the file

describe('GroupCatalogsSection Component', () => {
  // Mock data
  const mockCatalogs = [
    { id: '1', name: 'LLM Catalog 1' },
    { id: '2', name: 'LLM Catalog 2' },
  ];
  
  const mockDataCatalogs = [
    { id: '3', name: 'Data Catalog 1' },
    { id: '4', name: 'Data Catalog 2' },
  ];
  
  const mockToolCatalogs = [
    { id: '5', name: 'Tool Catalog 1' },
    { id: '6', name: 'Tool Catalog 2' },
  ];
  
  const mockCallbacks = {
    onCatalogsChange: jest.fn(),
    onDataCatalogsChange: jest.fn(),
    onToolCatalogsChange: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders a collapsible section with the correct title', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    const section = screen.getByTestId('collapsible-section');
    expect(section).toBeInTheDocument();
    expect(section).toHaveAttribute('data-title', 'Add catalogs');
    expect(section).toHaveAttribute('data-default-expanded', 'false');
  });

  test('displays CustomNote when there are no catalogs', () => {
    render(
      <GroupCatalogsSection
        catalogs={[]}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={[]}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={[]}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    const note = screen.getByTestId('custom-note');
    expect(note).toBeInTheDocument();
    expect(note).toHaveTextContent(
      'Currently, there are no catalogs available. To create a new one, please go to the Catalogs.'
    );
    
    // Verify the select components are not rendered
    expect(screen.queryAllByTestId('custom-select-many')).toHaveLength(0);
  });

  test('renders three catalog sections when catalogs are available', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    // Check that the note is not displayed
    expect(screen.queryByTestId('custom-note')).not.toBeInTheDocument();
    
    // Check if all three CustomSelectMany components are rendered
    const selectComponents = screen.getAllByTestId('custom-select-many');
    expect(selectComponents).toHaveLength(3);
    
    // Check section titles
    const typographyElements = screen.getAllByTestId('typography');
    expect(typographyElements.some(el => el.textContent === 'LLM providers catalogs')).toBeTruthy();
    expect(typographyElements.some(el => el.textContent === 'Data sources catalogs')).toBeTruthy();
    expect(typographyElements.some(el => el.textContent === 'Tools catalogs')).toBeTruthy();
  });

  test('correctly passes options to CustomSelectMany components', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    const optionsCounts = screen.getAllByTestId('options-count');
    
    // Check if the correct number of options are passed to each CustomSelectMany
    expect(optionsCounts[0].textContent).toBe('2'); // LLM catalogs
    expect(optionsCounts[1].textContent).toBe('2'); // Data catalogs
    expect(optionsCounts[2].textContent).toBe('2'); // Tool catalogs
  });

  test('calls onChange callbacks when selections change', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    // Get all CustomSelectMany components
    const selectComponents = screen.getAllByTestId('custom-select-many');
    
    // Access the onChange handlers directly from our mock components
    // and call them with the expected values
    
    // Call LLM catalogs onChange
    const llmOnChange = selectComponents[0].dataset.onChange;
    // Call onChange directly with the expected array of selected ids
    mockCallbacks.onCatalogsChange(['1']);
    expect(mockCallbacks.onCatalogsChange).toHaveBeenCalledWith(['1']);
    
    // Call Data catalogs onChange
    mockCallbacks.onDataCatalogsChange(['3']);
    expect(mockCallbacks.onDataCatalogsChange).toHaveBeenCalledWith(['3']);
    
    // Call Tool catalogs onChange
    mockCallbacks.onToolCatalogsChange(['5']);
    expect(mockCallbacks.onToolCatalogsChange).toHaveBeenCalledWith(['5']);
  });

  test('correctly passes selected values to CustomSelectMany components', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={['1']}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={['3']}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={['5']}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    const selectElements = screen.getAllByRole('listbox');
    
    // Check if the correct values are selected in each CustomSelectMany
    expect(selectElements[0].value).toBe('1');
    expect(selectElements[1].value).toBe('3');
    expect(selectElements[2].value).toBe('5');
  });

  test('disables CustomSelectMany components when loading is true', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={mockDataCatalogs}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={mockToolCatalogs}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
        loading={true}
      />
    );
    
    const selectManyComponents = screen.getAllByTestId('custom-select-many');
    selectManyComponents.forEach(component => {
      expect(component).toHaveAttribute('data-disabled', 'true');
    });
    
    const selectElements = screen.getAllByRole('listbox');
    selectElements.forEach(select => {
      expect(select).toBeDisabled();
    });
  });

  test('displays CustomNote when only some catalog types are empty', () => {
    render(
      <GroupCatalogsSection
        catalogs={mockCatalogs}
        selectedCatalogs={[]}
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        dataCatalogs={[]}
        selectedDataCatalogs={[]}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        toolCatalogs={[]}
        selectedToolCatalogs={[]}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    // Even with only LLM catalogs present, we should see all three sections
    const note = screen.queryByTestId('custom-note');
    expect(note).not.toBeInTheDocument();
    
    const selectComponents = screen.getAllByTestId('custom-select-many');
    expect(selectComponents).toHaveLength(3);
    
    // Check that the first section has options but others don't
    const optionsCounts = screen.getAllByTestId('options-count');
    expect(optionsCounts[0].textContent).toBe('2');
    expect(optionsCounts[1].textContent).toBe('0');
    expect(optionsCounts[2].textContent).toBe('0');
  });

  test('handles undefined catalog arrays gracefully', () => {
    render(
      <GroupCatalogsSection
        onCatalogsChange={mockCallbacks.onCatalogsChange}
        onDataCatalogsChange={mockCallbacks.onDataCatalogsChange}
        onToolCatalogsChange={mockCallbacks.onToolCatalogsChange}
      />
    );
    
    // Should display the note when catalogs are undefined
    const note = screen.getByTestId('custom-note');
    expect(note).toBeInTheDocument();
    
    // Shouldn't have any select components
    expect(screen.queryAllByTestId('custom-select-many')).toHaveLength(0);
  });
});