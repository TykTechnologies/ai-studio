import React from 'react';
import { render, screen, within } from '@testing-library/react';
import CatalogueBadges from './CatalogueBadges';
// import catalogueBadgeConfigs from '../utils/catalogueBadgeConfig'; // No longer needed directly due to Tooltip mock

// Mock the CustomSelectBadge component
jest.mock('../../../components/common/CustomSelectBadge', () => ({ config }) => (
  <div data-testid="custom-select-badge">{`${config.text} (${config.backgroundColor || 'mock-bg'})`}</div>
));

// Mock the catalogueBadgeConfigs to provide consistent colors for CustomSelectBadge mock
jest.mock('../utils/catalogueBadgeConfig', () => ({
  llm: { backgroundColor: 'blue', color: 'white', text: 'LLM' }, // text prop here is for default, will be overridden
  data: { backgroundColor: 'green', color: 'white', text: 'Data' },
  tool: { backgroundColor: 'orange', color: 'white', text: 'Tool' },
}));

// Global jest.fn for assertions/call tracking. It is NOT responsible for rendering the mock.
const mockTooltipAssertionSpy = jest.fn();

// Mock MUI Tooltip
jest.mock('@mui/material/Tooltip', () => {
  // This is the actual functional component mock that React will use for <Tooltip />.
  const DirectRenderTooltipMock = ({ title, children }) => {
    // console.log('DirectRenderTooltipMock actually rendering with title:', title); // For debugging if needed
    // We explicitly call our assertion spy here if we want to track render attempts.
    mockTooltipAssertionSpy({ title, children });
    return (
      <div data-testid="mock-tooltip" data-title={title}>
        {children} 
      </div>
    );
  };
  return DirectRenderTooltipMock;
});

// Helper to get the jest.fn() instance used for assertions like .mockClear() and .toHaveBeenCalledTimes()
const getMockTooltipImplementation = () => mockTooltipAssertionSpy;

describe('CatalogueBadges', () => {
  const baseProps = {
    catalogues: [],
    dataCatalogues: [],
    toolCatalogues: [],
  };

  beforeEach(() => {
    // Clear mock calls before each test
    getMockTooltipImplementation().mockClear();
  });

  test('renders correctly with no catalogues', () => {
    render(<CatalogueBadges {...baseProps} />);
    expect(screen.queryByTestId('custom-select-badge')).not.toBeInTheDocument();
    expect(screen.queryByText(/\+/)).not.toBeInTheDocument();
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with one LLM catalogue', () => {
    render(<CatalogueBadges {...baseProps} catalogues={['LLM1']} />);
    const badge = screen.getByTestId('custom-select-badge');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('LLM1 (blue)');
    expect(screen.queryByText(/\+/)).not.toBeInTheDocument();
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with one Data catalogue', () => {
    render(<CatalogueBadges {...baseProps} dataCatalogues={['Data1']} />);
    const badge = screen.getByTestId('custom-select-badge');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('Data1 (green)');
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with one Tool catalogue', () => {
    render(<CatalogueBadges {...baseProps} toolCatalogues={['Tool1']} />);
    const badge = screen.getByTestId('custom-select-badge');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('Tool1 (orange)');
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with fewer catalogues than MAX_BADGES (2)', () => {
    render(
      <CatalogueBadges
        {...baseProps}
        catalogues={['LLM1']}
        dataCatalogues={['Data1']}
      />
    );
    const badges = screen.getAllByTestId('custom-select-badge');
    expect(badges).toHaveLength(2);
    expect(badges[0]).toHaveTextContent('LLM1 (blue)');
    expect(badges[1]).toHaveTextContent('Data1 (green)');
    expect(screen.queryByText(/\+/)).not.toBeInTheDocument();
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with MAX_BADGES (2) catalogues', () => {
    render(
      <CatalogueBadges
        {...baseProps}
        catalogues={['LLM1', 'LLM2']}
      />
    );
    const badges = screen.getAllByTestId('custom-select-badge');
    expect(badges).toHaveLength(2);
    expect(badges[0]).toHaveTextContent('LLM1 (blue)');
    expect(badges[1]).toHaveTextContent('LLM2 (blue)');
    expect(screen.queryByText(/\+/)).not.toBeInTheDocument();
    expect(getMockTooltipImplementation()).not.toHaveBeenCalled();
  });

  test('renders correctly with more catalogues than MAX_BADGES, showing the "+N" indicator and Tooltip', () => {
    render(
      <CatalogueBadges
        {...baseProps}
        catalogues={['LLM1', 'LLM2']}
        dataCatalogues={['Data1']}
        toolCatalogues={['Tool1']} // Total 4, MAX_BADGES = 2, so +2
      />
    );
    const badges = screen.getAllByTestId('custom-select-badge');
    expect(badges).toHaveLength(2); // MAX_BADGES
    expect(badges[0]).toHaveTextContent('LLM1 (blue)');
    expect(badges[1]).toHaveTextContent('LLM2 (blue)');

    const tooltipElement = screen.getByTestId('mock-tooltip');
    expect(within(tooltipElement).getByText('+2')).toBeInTheDocument();
    const mockTooltip = getMockTooltipImplementation();
    expect(mockTooltip).toHaveBeenCalledTimes(1);
    // Check props passed to the last call of mockTooltip
    const lastTooltipCall = mockTooltip.mock.calls[mockTooltip.mock.calls.length - 1][0];
    expect(lastTooltipCall.title).toBe('2 more catalogues');

    // Additionally, we can check if our mock rendered the data-title (optional, depends on mock structure)
    expect(tooltipElement).toHaveAttribute('data-title', '2 more catalogues');
  });

  test('renders a mix of catalogue types correctly with Tooltip for overflow', () => {
    render(
      <CatalogueBadges
        {...baseProps}
        catalogues={['Gemini']}
        dataCatalogues={['Snowflake']}
        toolCatalogues={['Calculator']} // Total 3, MAX_BADGES = 2, so +1
      />
    );
    const badges = screen.getAllByTestId('custom-select-badge');
    expect(badges).toHaveLength(2); // Only two should be visible
    expect(badges[0]).toHaveTextContent('Gemini (blue)');
    expect(badges[1]).toHaveTextContent('Snowflake (green)');

    const tooltipElement = screen.getByTestId('mock-tooltip');
    expect(within(tooltipElement).getByText('+1')).toBeInTheDocument();
    const mockTooltip = getMockTooltipImplementation();
    expect(mockTooltip).toHaveBeenCalledTimes(1);
    const lastTooltipCall = mockTooltip.mock.calls[mockTooltip.mock.calls.length - 1][0];
    expect(lastTooltipCall.title).toBe('1 more catalogues');

    expect(tooltipElement).toHaveAttribute('data-title', '1 more catalogues');
  });

  test('renders only data and tool catalogues when llm catalogues are empty, with Tooltip for overflow', () => {
    render(
      <CatalogueBadges
        {...baseProps}
        dataCatalogues={['Data1', 'Data2']}
        toolCatalogues={['Tool1']} // Total 3, MAX_BADGES = 2, so +1
      />
    );
    const badges = screen.getAllByTestId('custom-select-badge');
    expect(badges).toHaveLength(2);
    expect(badges[0]).toHaveTextContent('Data1 (green)');
    expect(badges[1]).toHaveTextContent('Data2 (green)');

    const tooltipElement = screen.getByTestId('mock-tooltip');
    expect(within(tooltipElement).getByText('+1')).toBeInTheDocument();
    const mockTooltip = getMockTooltipImplementation();
    expect(mockTooltip).toHaveBeenCalledTimes(1);
    const lastTooltipCall = mockTooltip.mock.calls[mockTooltip.mock.calls.length - 1][0];
    expect(lastTooltipCall.title).toBe('1 more catalogues');
    
    expect(tooltipElement).toHaveAttribute('data-title', '1 more catalogues');
  });
}); 