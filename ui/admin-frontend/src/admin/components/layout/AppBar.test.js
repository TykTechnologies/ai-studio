import React from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import pubClient from '../../utils/pubClient';
import MyAppBar from './AppBar';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
  logout: jest.fn(),
}));
jest.mock('../../styles/sharedStyles', () => ({
  StyledIconButton: ({ children, onClick }) => <button onClick={onClick}>{children}</button>,
}));
jest.mock('../../context/EditionContext', () => ({
  useEdition: () => ({ version: 'test', isEnterprise: false }),
}));

describe('Admin AppBar docs link', () => {
  it('links to the docs quickstart when the server reports a docs URL', async () => {
    pubClient.get.mockResolvedValue({ data: { features: { docs_url: 'http://localhost:8989' } } });
    render(<MyAppBar />);
    const link = await screen.findByRole('link', { name: /Docs/ });
    expect(link).toHaveAttribute('href', 'http://localhost:8989/docs/quickstart');
  });

  it('shows no docs link when there is no docs URL (an embedding host)', async () => {
    pubClient.get.mockResolvedValue({ data: { features: { docs_url: '' } } });
    render(<MyAppBar />);
    await waitFor(() => expect(pubClient.get).toHaveBeenCalledWith('/common/system'));
    // Let the settings response be applied before checking.
    await act(() => pubClient.get.mock.results[0].value);
    expect(screen.queryByRole('link', { name: /Docs/ })).not.toBeInTheDocument();
  });
});
