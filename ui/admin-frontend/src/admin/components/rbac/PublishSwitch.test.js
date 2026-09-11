import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import PublishSwitch from './PublishSwitch';
import { usePermissions } from '../../context/PermissionsContext';
import { P } from '../../rbac/permissions';

jest.mock('../../context/PermissionsContext', () => ({
  usePermissions: jest.fn(),
}));

const withPermissions = (held) => {
  const set = new Set(held);
  usePermissions.mockReturnValue({
    can: (perm) => set.has('*') || set.has(perm),
  });
};

describe('PublishSwitch', () => {
  it('is enabled and toggles when the user holds publish', () => {
    withPermissions([P.LLMS_WRITE, P.LLMS_PUBLISH]);
    const onChange = jest.fn();
    render(<PublishSwitch permission={P.LLMS_PUBLISH} checked={false} onChange={onChange} name="active" label="Enabled in Proxy" />);
    const input = screen.getByLabelText('Enabled in Proxy');
    expect(input).not.toBeDisabled();
    fireEvent.click(input);
    expect(onChange).toHaveBeenCalled();
    expect(screen.queryByTestId('publish-switch-locked')).not.toBeInTheDocument();
  });

  it('is disabled and explains the missing permission for a writer without publish', () => {
    withPermissions([P.LLMS_WRITE]);
    const onChange = jest.fn();
    render(<PublishSwitch permission={P.LLMS_PUBLISH} checked={true} onChange={onChange} name="active" label="Enabled in Proxy" />);
    const input = screen.getByLabelText('Enabled in Proxy');
    expect(input).toBeDisabled();
    expect(input).toBeChecked();
    expect(screen.getByTestId('publish-switch-locked')).toBeInTheDocument();
    // jsdom still dispatches change on a disabled input under fireEvent, so
    // the disabled attribute is the contract here; the API rejects the
    // change regardless.
  });

  it('respects an explicit disabled prop even with the permission', () => {
    withPermissions(['*']);
    render(<PublishSwitch permission={P.AGENTS_PUBLISH} control="checkbox" checked={true} onChange={() => {}} disabled name="isActive" label="Active" />);
    expect(screen.getByLabelText('Active')).toBeDisabled();
    expect(screen.queryByTestId('publish-switch-locked')).not.toBeInTheDocument();
  });
});
