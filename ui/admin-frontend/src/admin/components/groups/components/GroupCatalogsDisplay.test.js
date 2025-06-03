import React from 'react';
import { screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import GroupCatalogsDisplay from './GroupCatalogsDisplay';
import { renderWithTheme } from '../../../../test-utils/render-with-theme';
import { GROUP_CATALOGS_DEFAULTS } from '../utils/groupDetailConfig';

jest.mock('../../common/CollapsibleSection', () => ({
  __esModule: true,
  default: ({ title, defaultExpanded, children }) => (
    <div data-testid="collapsible-section" data-title={title} data-default-expanded={defaultExpanded}>
      {children}
    </div>
  ),
}));

jest.mock('../../common/CustomNote', () => ({
  __esModule: true,
  default: ({ message }) => (
    <div data-testid="custom-note" data-message={message}>
      {message}
    </div>
  ),
}));

jest.mock('../../../styles/sharedStyles', () => {
  const { chipStylesMock } = jest.requireActual('../../../../test-utils/styled-component-mocks');
  return {
    StyledChip: chipStylesMock.StyledChip
  };
});

jest.mock('./styles', () => ({
  CatalogContainer: ({ children }) => <div data-testid="catalog-container">{children}</div>,
  getColorsForVariant: (theme, variant) => ({
    bgColor: variant === 'llm' ? '#e3f2fd' : variant === 'data' ? '#e8f5e9' : '#fff3e0',
    textColor: variant === 'llm' ? '#0d47a1' : variant === 'data' ? '#1b5e20' : '#e65100',
  }),
  CatalogTypeContainer: ({ children }) => <div data-testid="catalog-type-container">{children}</div>,
  CatalogTypeLabel: ({ children, variant, color }) => (
    <div data-testid="catalog-type-label" data-variant={variant} data-color={color}>
      {children}
    </div>
  ),
  CatalogsWrapper: ({ children }) => <div data-testid="catalogs-wrapper">{children}</div>,
  CatalogBorderLine: () => <div data-testid="catalog-border-line" />,
}));

jest.mock('../utils/groupDetailConfig', () => {
  const originalModule = jest.requireActual('../utils/groupDetailConfig');
  return {
    ...originalModule,
    getCatalogTypes: (features, catalogues, dataCatalogues, toolCatalogues) => {
      const types = [];
      if (!features || features.llm !== false) {
        types.push({
          label: 'LLM Catalogues',
          variant: 'llm',
          items: catalogues || [],
          show: true
        });
      }
      if (!features || features.data !== false) {
        types.push({
          label: 'Data Catalogues',
          variant: 'data',
          items: dataCatalogues || [],
          show: true
        });
      }
      if (!features || features.tool !== false) {
        types.push({
          label: 'Tool Catalogues',
          variant: 'tool',
          items: toolCatalogues || [],
          show: true
        });
      }
      return types;
    },
    GROUP_CATALOGS_DEFAULTS: {
      title: 'Catalogues',
      defaultExpanded: true,
      emptyMessage: 'No catalogues assigned to this team.',
    }
  };
});

describe('GroupCatalogsDisplay Component', () => {
  const defaultProps = {
    catalogues: [],
    dataCatalogues: [],
    toolCatalogues: [],
    features: {},
  };

  test('renders CollapsibleSection with default title and expanded state', () => {
    renderWithTheme(<GroupCatalogsDisplay {...defaultProps} />);
    
    const section = screen.getByTestId('collapsible-section');
    expect(section).toBeInTheDocument();
    expect(section).toHaveAttribute('data-title', GROUP_CATALOGS_DEFAULTS.title);
    expect(section).toHaveAttribute('data-default-expanded', 'true');
  });

  test('renders CustomNote when no catalogs are available', () => {
    renderWithTheme(<GroupCatalogsDisplay {...defaultProps} />);
    
    const note = screen.getByTestId('custom-note');
    expect(note).toBeInTheDocument();
    expect(note).toHaveAttribute('data-message', GROUP_CATALOGS_DEFAULTS.emptyMessage);
  });

  test('renders catalogs when available', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '1', name: 'LLM Catalog 1' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    expect(screen.queryByTestId('custom-note')).not.toBeInTheDocument();
    expect(screen.getByTestId('catalogs-wrapper')).toBeInTheDocument();
    const catalogTypeContainers = screen.getAllByTestId('catalog-type-container');
    expect(catalogTypeContainers.length).toBeGreaterThan(0);
    expect(screen.getAllByTestId('catalog-type-label')[0]).toBeInTheDocument();
    expect(screen.getAllByTestId('catalog-container')[0]).toBeInTheDocument();
    expect(screen.getByTestId('styled-chip')).toBeInTheDocument();
    expect(screen.getByText('LLM Catalog 1')).toBeInTheDocument();
  });

  test('renders multiple catalog types correctly', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '1', name: 'LLM Catalog 1' }],
      dataCatalogues: [{ id: '2', name: 'Data Catalog 1' }],
      toolCatalogues: [{ id: '3', name: 'Tool Catalog 1' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const catalogTypeContainers = screen.getAllByTestId('catalog-type-container');
    expect(catalogTypeContainers).toHaveLength(3);
    
    const catalogTypeLabels = screen.getAllByTestId('catalog-type-label');
    expect(catalogTypeLabels).toHaveLength(3);
    expect(catalogTypeLabels[0]).toHaveTextContent('LLM Catalogues');
    expect(catalogTypeLabels[1]).toHaveTextContent('Data Catalogues');
    expect(catalogTypeLabels[2]).toHaveTextContent('Tool Catalogues');
    
    const chips = screen.getAllByTestId('styled-chip');
    expect(chips).toHaveLength(3);
    expect(chips[0]).toHaveTextContent('LLM Catalog 1');
    expect(chips[1]).toHaveTextContent('Data Catalog 1');
    expect(chips[2]).toHaveTextContent('Tool Catalog 1');
    
    const borderLines = screen.getAllByTestId('catalog-border-line');
    expect(borderLines).toHaveLength(2);
  });

  test('renders catalogs with attributes name when name is missing', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '1', attributes: { name: 'LLM Catalog with Attributes' } }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const chip = screen.getByTestId('styled-chip');
    expect(chip).toHaveAttribute('data-label', 'LLM Catalog with Attributes');
    expect(chip).toHaveTextContent('LLM Catalog with Attributes');
  });

  test('renders fallback text when catalog has no name', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '42' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const chip = screen.getByTestId('styled-chip');
    expect(chip).toHaveAttribute('data-label', 'Item 42');
    expect(chip).toHaveTextContent('Item 42');
  });

  test('displays "None" when a catalog type is shown but has no items', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '1', name: 'LLM Catalog' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const noneTexts = screen.getAllByText('None');
    expect(noneTexts).toHaveLength(2); // Only Data and Tool catalogs show "None"
  });

  test('applies correct colors based on catalog variant', () => {
    const props = {
      ...defaultProps,
      catalogues: [{ id: '1', name: 'LLM Catalog' }],
      dataCatalogues: [{ id: '2', name: 'Data Catalog' }],
      toolCatalogues: [{ id: '3', name: 'Tool Catalog' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const chips = screen.getAllByTestId('styled-chip');
    
    expect(chips[0]).toHaveAttribute('data-bg-color', '#e3f2fd');
    expect(chips[0]).toHaveAttribute('data-text-color', '#0d47a1');
    
    expect(chips[1]).toHaveAttribute('data-bg-color', '#e8f5e9');
    expect(chips[1]).toHaveAttribute('data-text-color', '#1b5e20');
    
    expect(chips[2]).toHaveAttribute('data-bg-color', '#fff3e0');
    expect(chips[2]).toHaveAttribute('data-text-color', '#e65100');
  });

  test('does not render disabled catalog types based on features prop', () => {
    const props = {
      ...defaultProps,
      features: { llm: false },
      catalogues: [{ id: '1', name: 'LLM Catalog' }],
      dataCatalogues: [{ id: '2', name: 'Data Catalog' }],
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const catalogTypeLabels = screen.getAllByTestId('catalog-type-label');
    expect(catalogTypeLabels).toHaveLength(2);
    expect(catalogTypeLabels[0]).toHaveTextContent('Data Catalogues');
    expect(catalogTypeLabels[1]).toHaveTextContent('Tool Catalogues');
    
    expect(screen.queryByText('LLM Catalogues')).not.toBeInTheDocument();
  });

  test('uses custom title when provided', () => {
    const props = {
      ...defaultProps,
      title: 'Custom Catalogs Title',
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const section = screen.getByTestId('collapsible-section');
    expect(section).toHaveAttribute('data-title', 'Custom Catalogs Title');
  });

  test('uses custom defaultExpanded when provided', () => {
    const props = {
      ...defaultProps,
      defaultExpanded: false,
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const section = screen.getByTestId('collapsible-section');
    expect(section).toHaveAttribute('data-default-expanded', 'false');
  });

  test('uses custom emptyMessage when provided', () => {
    const props = {
      ...defaultProps,
      emptyMessage: 'Custom empty message',
    };
    
    renderWithTheme(<GroupCatalogsDisplay {...props} />);
    
    const note = screen.getByTestId('custom-note');
    expect(note).toHaveAttribute('data-message', 'Custom empty message');
  });
});