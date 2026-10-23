import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import { PermissionsProvider } from '../../context/PermissionsContext';
import { clearIdentity } from '../../utils/identityStore';
import AllowInternalHostSwitch from './AllowInternalHostSwitch';

jest.mock('../../utils/pubClient', () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const as = (isAdmin, ui) =>
  render(
    <PermissionsProvider identity={{ id: '1', attributes: { is_admin: isAdmin, permissions: isAdmin ? ['*'] : [] } }}>
      {ui}
    </PermissionsProvider>,
  );

// w4e: the "Allow internal host" switch is admin-only. An administrator can
// change it; anyone else only sees that it is on.
describe('AllowInternalHostSwitch', () => {
  beforeEach(() => clearIdentity());

  it('lets an administrator turn it on, with a warning', () => {
    const onChange = jest.fn();
    const { rerender } = as(true, <AllowInternalHostSwitch checked={false} onChange={onChange} />);
    fireEvent.click(screen.getByTestId('allow-internal-host-switch'));
    expect(onChange).toHaveBeenCalledWith(true);
    expect(screen.queryByText(/Only allow hosts you control/)).toBeNull();
    rerender(
      <PermissionsProvider identity={{ id: '1', attributes: { is_admin: true, permissions: ['*'] } }}>
        <AllowInternalHostSwitch checked onChange={onChange} />
      </PermissionsProvider>,
    );
    expect(screen.getByText(/Only allow hosts you control/)).toBeInTheDocument();
  });

  it('shows a non-admin no control, and the state only when on', () => {
    const { unmount } = as(false, <AllowInternalHostSwitch checked={false} onChange={jest.fn()} />);
    expect(screen.queryByTestId('allow-internal-host-switch')).toBeNull();
    expect(screen.queryByTestId('allow-internal-host-readonly')).toBeNull();
    unmount();
    as(false, <AllowInternalHostSwitch checked onChange={jest.fn()} />);
    expect(screen.queryByTestId('allow-internal-host-switch')).toBeNull();
    expect(screen.getByTestId('allow-internal-host-readonly')).toBeInTheDocument();
  });
});
