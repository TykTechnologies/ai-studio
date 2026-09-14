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

// Mock featureUtils
jest.mock('../../../utils/featureUtils', () => ({
  getFeatureFlags: () => ({
    isPortalEnabled: true,
    isChatEnabled: true
  })
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

jest.mock('../../common/relationship-picker', () => require('../../../../test-utils/component-mocks').relationshipPickerMock);

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
  // Mock data: catalogs are { value, label } pairs, as useCatalogsSelection shapes them.
  const mockCatalogs = [
    { value: '1', label: 'LLM Catalog 1' },
    { value: '2', label: 'LLM Catalog 2' },
  ];

  const mockDataCatalogs = [
    { value: '3', label: 'Data Catalog 1' },
    { value: '4', label: 'Data Catalog 2' },
  ];

  const mockToolCatalogs = [
    { value: '5', label: 'Tool Catalog 1' },
    { value: '6', label: 'Tool Catalog 2' },
  ];

  const mockCallbacks = {
    onCatalogsChange: jest.fn(),
    onDataCatalogsChange: jest.fn(),
    onToolCatalogsChange: jest.fn(),
  };

  const renderSection = (overrides = {}) =>
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
        features={{ feature_portal: true, feature_chat: true }}
        {...overrides}
      />
    );

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders a collapsible section with the correct title', () => {
    renderSection();

    const section = screen.getByTestId('collapsible-section');
    expect(section).toBeInTheDocument();
    expect(section).toHaveAttribute('data-title', 'Add catalogs');
    expect(section).toHaveAttribute('data-default-expanded', 'false');
  });

  test('displays CustomNote when there are no catalogs', () => {
    renderSection({ catalogs: [], dataCatalogs: [], toolCatalogs: [] });

    const note = screen.getByTestId('custom-note');
    expect(note).toBeInTheDocument();
    expect(note).toHaveTextContent(
      'Currently, there are no catalogs available. To create a new one, please go to the Catalogs.'
    );

    // Verify the pickers are not rendered
    expect(screen.queryAllByTestId('relationship-picker')).toHaveLength(0);
  });

  test('renders three compact pickers when catalogs are available', () => {
    renderSection();

    // Check that the note is not displayed
    expect(screen.queryByTestId('custom-note')).not.toBeInTheDocument();

    // One compact RelationshipPicker per catalog type, headed by its label
    const pickers = screen.getAllByTestId('relationship-picker');
    expect(pickers).toHaveLength(3);
    expect(pickers.map((p) => p.getAttribute('aria-label'))).toEqual([
      'LLM providers catalogs',
      'Data sources catalogs',
      'Tools catalogs',
    ]);
    pickers.forEach((p) => expect(p).toHaveAttribute('data-variant', 'compact'));
    expect(pickers.map((p) => p.getAttribute('data-item-label'))).toEqual([
      'LLM catalog',
      'data catalog',
      'tool catalog',
    ]);
  });

  test('correctly passes options to the pickers', () => {
    renderSection();

    const optionsCounts = screen.getAllByTestId('relationship-picker-options-count');

    // Check if the correct number of options are passed to each picker
    expect(optionsCounts[0].textContent).toBe('2'); // LLM catalogs
    expect(optionsCounts[1].textContent).toBe('2'); // Data catalogs
    expect(optionsCounts[2].textContent).toBe('2'); // Tool catalogs
  });

  test('calls onChange callbacks with the full new selection when a picker changes', () => {
    renderSection();

    const addButtons = screen.getAllByTestId('relationship-picker-add');
    fireEvent.click(addButtons[0]);
    expect(mockCallbacks.onCatalogsChange).toHaveBeenCalledWith([mockCatalogs[0]]);

    fireEvent.click(addButtons[1]);
    expect(mockCallbacks.onDataCatalogsChange).toHaveBeenCalledWith([mockDataCatalogs[0]]);

    fireEvent.click(addButtons[2]);
    expect(mockCallbacks.onToolCatalogsChange).toHaveBeenCalledWith([mockToolCatalogs[0]]);
  });

  test('correctly passes selected values to the pickers, keyed by value', () => {
    renderSection({
      selectedCatalogs: [mockCatalogs[0]],
      selectedDataCatalogs: [mockDataCatalogs[0]],
      selectedToolCatalogs: [mockToolCatalogs[0]],
    });

    const items = screen.getAllByTestId('relationship-picker-item');
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveAttribute('data-item-id', '1');
    expect(items[0]).toHaveTextContent('LLM Catalog 1');
    expect(items[1]).toHaveAttribute('data-item-id', '3');
    expect(items[1]).toHaveTextContent('Data Catalog 1');
    expect(items[2]).toHaveAttribute('data-item-id', '5');
    expect(items[2]).toHaveTextContent('Tool Catalog 1');
  });

  test('disables the pickers when loading is true', () => {
    renderSection({ loading: true });

    screen.getAllByTestId('relationship-picker').forEach(component => {
      expect(component).toHaveAttribute('data-disabled', 'true');
    });
    screen.getAllByTestId('relationship-picker-add').forEach((button) => {
      expect(button).toBeDisabled();
    });
  });

  test('shows all three pickers when only some catalog types are empty', () => {
    renderSection({ dataCatalogs: [], toolCatalogs: [] });

    // Even with only LLM catalogs present, we should see all three sections
    const note = screen.queryByTestId('custom-note');
    expect(note).not.toBeInTheDocument();

    const pickers = screen.getAllByTestId('relationship-picker');
    expect(pickers).toHaveLength(3);

    // Check that the first section has options but others don't
    const optionsCounts = screen.getAllByTestId('relationship-picker-options-count');
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
        features={{ feature_portal: true, feature_chat: true }}
      />
    );

    // Should display the note when catalogs are undefined
    const note = screen.getByTestId('custom-note');
    expect(note).toBeInTheDocument();

    // Shouldn't have any pickers
    expect(screen.queryAllByTestId('relationship-picker')).toHaveLength(0);
  });
});
