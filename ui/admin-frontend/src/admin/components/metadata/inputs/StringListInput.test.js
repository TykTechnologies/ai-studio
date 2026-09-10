import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import StringListInput from './StringListInput';

describe('StringListInput', () => {
  it('adds on Enter and via the add button, ignoring blanks and duplicates', () => {
    const onChange = jest.fn();
    render(<StringListInput id="x" label="Consumers" value={['billing']} onChange={onChange} />);

    const input = screen.getByLabelText('Consumers');
    fireEvent.change(input, { target: { value: 'analytics' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenLastCalledWith(['billing', 'analytics']);

    fireEvent.change(input, { target: { value: 'billing' } });
    fireEvent.click(screen.getByLabelText('Add Consumers'));
    expect(onChange).toHaveBeenCalledTimes(1);

    fireEvent.change(input, { target: { value: '   ' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it('removes a chip', () => {
    const onChange = jest.fn();
    render(<StringListInput id="x" label="Consumers" value={['a', 'b']} onChange={onChange} />);
    // MUI renders one CancelIcon per deletable chip, in chip order.
    fireEvent.click(screen.getAllByTestId('CancelIcon')[0]);
    expect(onChange).toHaveBeenCalledWith(['b']);
  });
});
