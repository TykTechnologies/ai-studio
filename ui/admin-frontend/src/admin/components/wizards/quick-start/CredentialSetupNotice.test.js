import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import CredentialSetupNotice, { providersNeedingCredentials } from './CredentialSetupNotice';

jest.mock('../../../utils/listAll', () => ({
  __esModule: true,
  listAll: jest.fn(),
}));

const { listAll } = require('../../../utils/listAll');

let mockGranted = () => true;
jest.mock('../../../context/PermissionsContext', () => ({
  usePermissions: () => ({ can: (perm) => mockGranted(perm) }),
}));

const llm = (id, name, status, ref, active = true) => ({
  id,
  attributes: { name, active, credential_status: status, credential_ref: ref },
});

// The seeded state of a fresh instance (#505): both providers active, both
// pointing at empty secrets.
const seeded = [
  llm('1', 'OpenAI', 'unresolved', 'OPENAI_KEY'),
  llm('2', 'Anthropic', 'unresolved', 'ANTHROPIC_KEY'),
];

describe('providersNeedingCredentials', () => {
  test('keeps active providers whose key is unresolved or missing', () => {
    expect(
      providersNeedingCredentials([
        ...seeded,
        llm('3', 'No key', 'unset'),
        llm('4', 'Vault', 'secret', 'OK_KEY'),
        llm('5', 'Inline', 'inline'),
        llm('6', 'Off', 'unresolved', 'OFF_KEY', false),
      ])
    ).toEqual([
      { id: '1', name: 'OpenAI', secret: 'OPENAI_KEY' },
      { id: '2', name: 'Anthropic', secret: 'ANTHROPIC_KEY' },
      { id: '3', name: 'No key', secret: null },
    ]);
  });

  test('copes with no providers', () => {
    expect(providersNeedingCredentials(undefined)).toEqual([]);
  });
});

describe('CredentialSetupNotice', () => {
  beforeEach(() => {
    mockGranted = () => true;
    listAll.mockResolvedValue({
      data: { data: [{ id: '11', attributes: { var_name: 'OPENAI_KEY' } }, { id: '12', attributes: { var_name: 'OTHER' } }] },
    });
  });

  test('names each empty secret and links to it, falling back to the secrets list', async () => {
    render(<CredentialSetupNotice llms={seeded} />);
    expect(screen.getByTestId('quickstart-credential-notice')).toHaveTextContent('Add your LLM provider keys first');

    const openai = screen.getByRole('link', { name: 'OPENAI_KEY' });
    await waitFor(() => expect(openai).toHaveAttribute('href', '/admin/secrets/edit/11'));
    expect(openai).toHaveAttribute('target', '_blank');
    // ANTHROPIC_KEY is not in the listing (or the user cannot read secrets).
    expect(screen.getByRole('link', { name: 'ANTHROPIC_KEY' })).toHaveAttribute('href', '/admin/secrets');
    expect(listAll).toHaveBeenCalledTimes(1);
  });

  test('a provider with no key at all links to its edit form', () => {
    render(<CredentialSetupNotice llms={[llm('3', 'Bare', 'unset')]} />);
    expect(screen.getByRole('link', { name: 'Add one' })).toHaveAttribute('href', '/admin/llms/edit/3');
    expect(listAll).not.toHaveBeenCalled();
  });

  test('renders nothing, and fetches nothing, when every provider can make calls', () => {
    const { container } = render(<CredentialSetupNotice llms={[llm('4', 'Vault', 'secret', 'OK_KEY')]} />);
    expect(container).toBeEmptyDOMElement();
    expect(listAll).not.toHaveBeenCalled();
  });

  test('without secrets:write it links the list, without secrets:read it names the secret unlinked', () => {
    mockGranted = (perm) => perm === 'secrets:read';
    const { unmount } = render(<CredentialSetupNotice llms={seeded} />);
    expect(screen.getByRole('link', { name: 'OPENAI_KEY' })).toHaveAttribute('href', '/admin/secrets');
    expect(listAll).not.toHaveBeenCalled();
    unmount();

    mockGranted = () => false;
    render(<CredentialSetupNotice llms={[...seeded, llm('3', 'Bare', 'unset')]} />);
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByText('OPENAI_KEY')).toBeInTheDocument();
    expect(screen.queryByText('Add one')).not.toBeInTheDocument();
  });
});
