import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

// Mock dependencies
jest.mock('@mui/material', () => ({
  Typography: ({ children, variant, color, component, ...props }) => (
    <div data-testid="typography" data-variant={variant} data-color={color} {...props}>
      {children}
    </div>
  ),
  Box: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
}));

// Mock the styled components from sharedStyles
jest.mock('../../../styles/sharedStyles', () => ({
  SectionContainer: ({ children, sx, ...props }) => (
    <div data-testid="section-container" style={sx} {...props}>
      {children}
    </div>
  ),
  SectionHeader: ({ children, isCollapsible = false, sx, ...props }) => (
    <div data-testid="section-header" data-collapsible={isCollapsible.toString()} style={sx} {...props}>
      {children}
    </div>
  ),
  SectionContent: ({ children, ...props }) => (
    <div data-testid="section-content" {...props}>
      {children}
    </div>
  ),
}));

// Import the component under test after mocks are set up
const Section = require('../Section').default;

describe('Section Component', () => {
  test('renders with title', () => {
    render(<Section title="Test Title" />);
    
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByTestId('section-header')).toBeInTheDocument();
    expect(screen.getByTestId('section-content')).toBeInTheDocument();
  });

  test('renders without title', () => {
    render(<Section />);
    
    expect(screen.queryByTestId('section-header')).not.toBeInTheDocument();
    expect(screen.getByTestId('section-content')).toBeInTheDocument();
  });

  test('renders children properly', () => {
    render(
      <Section title="Test Title">
        <div data-testid="test-child">Child Content</div>
      </Section>
    );
    
    expect(screen.getByTestId('test-child')).toBeInTheDocument();
    expect(screen.getByText('Child Content')).toBeInTheDocument();
  });

  test('applies custom sx styles to container', () => {
    const customSx = { marginBottom: '20px', padding: '15px' };
    render(<Section title="Test Title" sx={customSx} />);
    
    const container = screen.getByTestId('section-container');
    expect(container).toHaveStyle('margin-bottom: 20px');
    expect(container).toHaveStyle('padding: 15px');
  });

  test('renders title with correct typography variant', () => {
    render(<Section title="Test Title" />);
    
    const titleElement = screen.getAllByTestId('typography')[0];
    expect(titleElement).toHaveAttribute('data-variant', 'headingMedium');
    expect(titleElement).toHaveAttribute('data-color', 'text.primary');
    expect(titleElement).toHaveTextContent('Test Title');
  });

  test('applies correct sx styling to SectionHeader', () => {
    render(<Section title="Test Title" />);
    
    const header = screen.getByTestId('section-header');
    expect(header).toHaveAttribute('data-collapsible', 'false');
  });

  test('passes sx prop correctly to SectionContainer', () => {
    const customSx = { width: '500px' };
    render(<Section title="Test Title" sx={customSx} />);
    
    const container = screen.getByTestId('section-container');
    expect(container).toHaveStyle('width: 500px');
  });

  test('renders children inside SectionContent', () => {
    render(
      <Section>
        <p>Test Paragraph</p>
      </Section>
    );
    
    const content = screen.getByTestId('section-content');
    expect(content).toHaveTextContent('Test Paragraph');
  });

  // The title/description/actions header shared by the detail pages (M5).
  test('renders the description under the title and actions on the right', () => {
    render(
      <Section title="Used by" description="Everything that references this." actions={<button type="button">Export</button>}>
        <p>body</p>
      </Section>
    );

    const header = screen.getByTestId('section-header');
    expect(header).toHaveTextContent('Used by');
    expect(header).toHaveTextContent('Everything that references this.');
    expect(screen.getByRole('button', { name: 'Export' })).toBeInTheDocument();
    expect(screen.getByTestId('section-content')).toHaveTextContent('body');
  });

  test('renders a header for actions alone, and none when there is neither title nor actions', () => {
    const { unmount } = render(<Section actions={<span>act</span>} />);
    expect(screen.getByTestId('section-header')).toHaveTextContent('act');
    unmount();
    render(<Section>x</Section>);
    expect(screen.queryByTestId('section-header')).not.toBeInTheDocument();
  });

  test('passes data attributes through to the container', () => {
    render(<Section title="T" data-testid="my-section">x</Section>);
    expect(screen.getByTestId('my-section')).toBeInTheDocument();
  });
});
