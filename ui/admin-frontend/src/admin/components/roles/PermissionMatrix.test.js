import React from 'react';
import { render as rtlRender, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import PermissionMatrix from './PermissionMatrix';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

jest.mock('../../hooks/usePermissionCatalogue');
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const catalogue = {
  loading: false,
  actions: ['read', 'write', 'delete', 'execute'],
  grouped: [
    {
      group: 'LLM management',
      resources: [
        { key: 'llms', label: 'LLM providers', group: 'LLM management', actions: ['read', 'write', 'delete'] },
      ],
    },
    {
      group: 'Governance',
      resources: [
        { key: 'audit', label: 'Audit trail', group: 'Governance', actions: ['read'], sensitive: true },
      ],
    },
  ],
};

const render = (ui) => rtlRender(<ThemeProvider theme={adminTheme}>{ui}</ThemeProvider>);

describe('PermissionMatrix', () => {
  beforeEach(() => {
    usePermissionCatalogue.mockReturnValue(catalogue);
  });

  it('renders a table per group, omitting actions a resource does not offer', () => {
    render(<PermissionMatrix value={new Set()} onChange={() => {}} />);
    expect(screen.getByText('LLM management')).toBeInTheDocument();
    expect(screen.getByText('Governance')).toBeInTheDocument();
    expect(screen.getByLabelText('llms:write')).toBeInTheDocument();
    expect(screen.queryByLabelText('llms:execute')).toBeNull();
    expect(screen.queryByLabelText('audit:write')).toBeNull();
    expect(screen.getByTestId('icon-shield')).toBeInTheDocument();
  });

  it('ticking write also ticks read, and untick read clears the row', () => {
    const onChange = jest.fn();
    const { rerender } = render(<PermissionMatrix value={new Set()} onChange={onChange} />);
    fireEvent.click(screen.getByLabelText('llms:write'));
    expect([...onChange.mock.calls[0][0]].sort()).toEqual(['llms:read', 'llms:write']);

    rerender(
      <ThemeProvider theme={adminTheme}>
        <PermissionMatrix value={new Set(['llms:read', 'llms:write'])} onChange={onChange} />
      </ThemeProvider>
    );
    fireEvent.click(screen.getByLabelText('llms:read'));
    expect([...onChange.mock.calls[1][0]]).toEqual([]);
  });

  it('row "All" toggles every action and shows indeterminate for partial rows', () => {
    const onChange = jest.fn();
    render(<PermissionMatrix value={new Set(['llms:read'])} onChange={onChange} />);
    const all = screen.getByLabelText('All LLM providers permissions');
    expect(all).toHaveAttribute('data-indeterminate', 'true');
    fireEvent.click(all);
    expect([...onChange.mock.calls[0][0]].sort()).toEqual(['llms:delete', 'llms:read', 'llms:write']);
  });

  it('is inert when readOnly and filters rows by search', () => {
    const onChange = jest.fn();
    render(<PermissionMatrix value={new Set(['audit:read'])} onChange={onChange} readOnly />);
    expect(screen.getByLabelText('audit:read')).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Filter resources'), { target: { value: 'audit' } });
    expect(screen.queryByText('LLM providers')).toBeNull();
    expect(screen.getByText('Audit trail')).toBeInTheDocument();
  });
});
