import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import PluginCard from './PluginCard';

jest.mock('../../context/EditionContext', () => ({
  useEdition: () => ({ isEnterprise: true, loading: false }),
}));

const plugin = {
  plugin_id: 'com.example.demo',
  name: 'Demo',
  version: '1.0.0',
  description: 'A demo plugin',
  publisher: 'tyk-official',
  enterprise_only: false,
};

describe('PluginCard plugin sources', () => {
  test('install is offered when oci is allowed', () => {
    render(<PluginCard plugin={plugin} onViewDetails={jest.fn()} onInstall={jest.fn()} />);
    expect(screen.getByRole('button', { name: 'Install' })).toBeEnabled();
  });

  test('install is disabled, with the setting named, when oci is not allowed', async () => {
    const onInstall = jest.fn();
    render(<PluginCard plugin={plugin} ociAllowed={false} onViewDetails={jest.fn()} onInstall={onInstall} />);
    const install = screen.getByRole('button', { name: 'Install' });
    expect(install).toBeDisabled();
    fireEvent.mouseOver(install.parentElement);
    expect(await screen.findByText(/AI_STUDIO_PLUGIN_ALLOWED_SOURCES/)).toBeInTheDocument();
  });
});
