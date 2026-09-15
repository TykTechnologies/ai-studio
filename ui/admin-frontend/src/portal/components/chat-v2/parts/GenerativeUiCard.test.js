import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import { generateTheme } from '../../../../admin/theme';
import { GenerativeUiCard, makePresentRenderer } from './GenerativeUiCard';
import { getToolUiRegistry } from '../toolUiRegistry';

const mockAppend = jest.fn();
jest.mock('@assistant-ui/react', () => ({
  ...jest.requireActual('@assistant-ui/react'),
  useAui: () => ({ thread: () => ({ append: mockAppend }) }),
}));
const appendMock = mockAppend;

const wrap = (ui) => <ThemeProvider theme={generateTheme()}>{ui}</ThemeProvider>;

const dashboard = {
  component: 'Card',
  title: 'Q3 revenue',
  children: [
    { component: 'Row', gap: 4, children: [
      { component: 'Fact', label: 'Bookings', value: '$1.2M' },
      { component: 'Fact', label: 'Churn', value: '2.1%' },
    ] },
    { component: 'Table', columns: [{ label: 'Region' }, { label: 'ARR' }], rows: [['EMEA', '$400k'], ['US', '$800k']] },
    { component: 'Markdown', value: 'Growth is **strong**.' },
    { component: 'Button', label: 'Drill down', action: { type: 'drill', region: 'EMEA' } },
  ],
};

describe('GenerativeUiCard', () => {
  beforeEach(() => appendMock.mockClear());

  it('renders the component tree and resolves a parked call once the args are complete', () => {
    const addResult = jest.fn();
    render(wrap(<GenerativeUiCard args={dashboard} status={{ type: 'requires-action', reason: 'tool-calls' }} addResult={addResult} />));

    const root = screen.getByTestId('generative-ui');
    expect(root).toHaveTextContent('Q3 revenue');
    expect(root).toHaveTextContent('Bookings');
    expect(root).toHaveTextContent('$1.2M');
    expect(root.querySelector('[data-aui="table"]')).toHaveTextContent('EMEA');
    // Markdown is rendered, not shown raw.
    expect(root.querySelector('strong')).toHaveTextContent('strong');
    expect(addResult).toHaveBeenCalledTimes(1);
    expect(addResult).toHaveBeenCalledWith({});
  });

  it('renders a call replayed from history without answering it again', () => {
    const addResult = jest.fn();
    render(wrap(<GenerativeUiCard args={dashboard} result={{}} status={{ type: 'complete', reason: 'stop' }} addResult={addResult} />));
    expect(screen.getByTestId('generative-ui')).toHaveTextContent('Q3 revenue');
    expect(addResult).not.toHaveBeenCalled();
  });

  it('accepts text as a button label', () => {
    render(wrap(<GenerativeUiCard args={{ component: 'Button', text: 'Go there', action: { type: 'go' } }} result={{}} status={{ type: 'complete' }} />));
    expect(screen.getByRole('button', { name: 'Go there' })).toBeInTheDocument();
  });

  it('does not resolve while the arguments are still streaming', () => {
    const addResult = jest.fn();
    render(wrap(<GenerativeUiCard args={{ component: 'Text', value: 'partial' }} status={{ type: 'running' }} addResult={addResult} />));
    expect(addResult).not.toHaveBeenCalled();
  });

  it('turns an interactive $action into the next user message', () => {
    render(wrap(<GenerativeUiCard args={dashboard} status={{ type: 'complete', reason: 'stop' }} result={{}} addResult={jest.fn()} />));
    fireEvent.click(screen.getByText('Drill down'));
    expect(appendMock).toHaveBeenCalledTimes(1);
    const msg = appendMock.mock.calls[0][0];
    expect(msg.role).toBe('user');
    expect(msg.content[0].text).toContain('drill');
    expect(msg.content[0].text).toContain('EMEA');
  });

  it('blocks unsafe image sources and backgrounds from the model', () => {
    const tree = {
      component: 'Card',
      background: 'url(https://evil.example/track.png)',
      children: [
        { component: 'Image', src: 'javascript:alert(1)', alt: 'blocked picture' },
        { component: 'Image', src: 'https://example.com/ok.png', alt: 'fine' },
        { component: 'Markdown', value: 'Hello <script>alert(1)</script> <img src=x onerror=alert(1)>' },
      ],
    };
    render(wrap(<GenerativeUiCard args={tree} result={{}} status={{ type: 'complete' }} />));
    const root = screen.getByTestId('generative-ui');
    expect(root.querySelectorAll('img')).toHaveLength(1);
    expect(root.querySelector('img').getAttribute('src')).toBe('https://example.com/ok.png');
    expect(root.querySelector('[data-aui="image-blocked"]')).toHaveTextContent('blocked picture');
    expect(root.querySelector('[data-aui="card"]').getAttribute('style') || '').not.toContain('url(');
    expect(root.querySelector('script')).toBeNull();
    expect(root.querySelector('[data-aui="markdown"]')).toHaveTextContent('<script>');
  });

  it('is picked for client tools of kind present', () => {
    const registry = getToolUiRegistry([
      { name: 'present', ui: { kind: 'present' } },
      { name: 'ask-approval', ui: { kind: 'approval' } },
    ]);
    expect(registry.present.displayName).toBe('GenerativeUI(present)');
    expect(registry['ask-approval'].displayName).toBe('HumanTool(ask-approval)');
    expect(makePresentRenderer({ name: 'x' }).displayName).toBe('GenerativeUI(x)');
  });
});
