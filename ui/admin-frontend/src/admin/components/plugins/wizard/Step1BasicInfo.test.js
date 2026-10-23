import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import testTheme from '../../../utils/testTheme';
import Step1BasicInfo from './Step1BasicInfo';
import useConfig from '../../../hooks/useConfig';

jest.mock('../../../hooks/useConfig', () => jest.fn());
jest.mock('../../../utils/apiClient');

const renderStep = (onComplete = jest.fn()) => {
  render(
    <ThemeProvider theme={testTheme}>
      <Step1BasicInfo data={{}} onComplete={onComplete} onBack={jest.fn()} />
    </ThemeProvider>
  );
  return onComplete;
};

const fill = (label, value) => {
  fireEvent.change(screen.getByLabelText(new RegExp(label)), { target: { value } });
};

describe('Step1BasicInfo plugin sources', () => {
  test('offers every source by default', () => {
    useConfig.mockReturnValue({ config: { plugin_allowed_sources: ['oci', 'file', 'grpc'], oci_require_signature: false } });
    renderStep();
    expect(
      screen.getByText('Plugin command - use oci:// for OCI artifacts, grpc:// for external services, or local path for binaries')
    ).toBeInTheDocument();
  });

  test('mentions only oci:// and signing when only oci is allowed', () => {
    useConfig.mockReturnValue({ config: { plugin_allowed_sources: ['oci'], oci_require_signature: true } });
    renderStep();
    expect(screen.getByText('Plugin command - use oci:// for OCI artifacts (they must be signed)')).toBeInTheDocument();
    expect(screen.queryByText(/grpc:\/\//)).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText('e.g., oci://registry.com/my-plugin:latest')).toBeInTheDocument();
  });

  test("rejects a file path with the backend's wording", () => {
    useConfig.mockReturnValue({ config: { plugin_allowed_sources: ['oci'] } });
    const onComplete = renderStep();
    fill('Plugin Name', 'demo');
    fill('Command', '/usr/local/bin/demo');
    fireEvent.click(screen.getByText('Load Plugin'));
    expect(
      screen.getByText(
        '🔒 SECURITY: plugin source "file" is not allowed by this installation (allowed: oci). Set AI_STUDIO_PLUGIN_ALLOWED_SOURCES to change this.'
      )
    ).toBeInTheDocument();
    expect(onComplete).not.toHaveBeenCalled();
  });

  test('accepts an allowed source', () => {
    useConfig.mockReturnValue({ config: { plugin_allowed_sources: ['oci'] } });
    const onComplete = renderStep();
    fill('Plugin Name', 'demo');
    fill('Command', 'oci://registry.example.com/plugins/demo:1.0.0');
    fireEvent.click(screen.getByText('Load Plugin'));
    expect(onComplete).toHaveBeenCalledWith(expect.objectContaining({ command: 'oci://registry.example.com/plugins/demo:1.0.0' }));
  });
});
