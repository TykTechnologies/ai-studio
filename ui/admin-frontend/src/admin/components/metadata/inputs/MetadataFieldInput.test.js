import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import MetadataFieldInput from './MetadataFieldInput';

const vocabulary = {
  slug: 'risk',
  terms: [
    { value: 'low', label: 'Low' },
    { value: 'high', label: 'High' },
    { value: 'legacy', label: 'Legacy', deprecated: true },
  ],
};
const users = [
  { id: 1, attributes: { name: 'Alice', email: 'alice@x.io' } },
  { id: 2, attributes: { email: 'bob@x.io' } },
];

const renderInput = (field, props = {}) => {
  const onChange = jest.fn();
  render(<MetadataFieldInput field={field} onChange={onChange} vocabulary={vocabulary} users={users} {...props} />);
  return onChange;
};

// Stateful harness for sequences of edits: the inputs are controlled, so the
// parent must feed values back for a second change to register.
const Controlled = ({ field, initial, onChange }) => {
  const [value, setValue] = React.useState(initial);
  return (
    <MetadataFieldInput
      field={field}
      value={value}
      vocabulary={vocabulary}
      users={users}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
};
const renderControlled = (field, initial) => {
  const onChange = jest.fn();
  render(<Controlled field={field} initial={initial} onChange={onChange} />);
  return onChange;
};

describe('MetadataFieldInput', () => {
  it('string / email / url emit text and undefined when cleared', () => {
    const onChange = renderInput({ key: 'contact', label: 'Contact', type: 'email', max_length: 20 }, { value: 'a@b.c' });
    const input = screen.getByLabelText(/Contact/);
    expect(input).toHaveAttribute('type', 'email');
    expect(input).toHaveAttribute('maxlength', '20');
    fireEvent.change(input, { target: { value: 'new@b.c' } });
    expect(onChange).toHaveBeenLastCalledWith('new@b.c');
    fireEvent.change(input, { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it('text renders a multi-line input with the description as helper text', () => {
    renderInput({ key: 'notes', label: 'Notes', type: 'text', description: 'Free text' });
    expect(screen.getByText('Free text')).toBeInTheDocument();
    expect(screen.getByLabelText(/Notes/).tagName).toBe('TEXTAREA');
  });

  it('number emits numbers and respects min/max', () => {
    const onChange = renderControlled({ key: 'score', label: 'Score', type: 'number', min: 1, max: 10 });
    const input = screen.getByLabelText(/Score/);
    expect(input).toHaveAttribute('min', '1');
    expect(input).toHaveAttribute('max', '10');
    fireEvent.change(input, { target: { value: '7' } });
    expect(onChange).toHaveBeenLastCalledWith(7);
    fireEvent.change(input, { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it('boolean toggles true/false', () => {
    const onChange = renderInput({ key: 'flag', label: 'Flag', type: 'boolean' }, { value: false });
    fireEvent.click(screen.getByLabelText('Flag'));
    expect(onChange).toHaveBeenLastCalledWith(true);
  });

  it('date warns locally when warn_if_past and the value is in the past', () => {
    renderInput({ key: 'exp', label: 'Expires', type: 'date', warn_if_past: true }, { value: '2000-01-01' });
    expect(screen.getByText('Expires is in the past')).toBeInTheDocument();
    expect(screen.getByLabelText(/Expires/)).toHaveAttribute('type', 'date');
  });

  it('date shows a server error instead of the local warning', () => {
    renderInput({ key: 'exp', label: 'Expires', type: 'date', warn_if_past: true }, { value: '2000-01-01', error: 'bad date' });
    expect(screen.getByText('bad date')).toBeInTheDocument();
    expect(screen.queryByText('Expires is in the past')).not.toBeInTheDocument();
  });

  it('user select lists users and emits numeric ids', async () => {
    const onChange = renderInput({ key: 'owner', label: 'Owner', type: 'user' });
    fireEvent.mouseDown(screen.getByLabelText(/Owner/));
    expect(await screen.findByRole('option', { name: 'Alice (alice@x.io)' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'bob@x.io' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('option', { name: 'Alice (alice@x.io)' }));
    expect(onChange).toHaveBeenLastCalledWith(1);
  });

  it('optional selects offer None which clears the value', async () => {
    const onChange = renderInput({ key: 'owner', label: 'Owner', type: 'user' }, { value: 1 });
    fireEvent.mouseDown(screen.getByLabelText(/Owner/));
    fireEvent.click(await screen.findByRole('option', { name: 'None' }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it('required selects do not offer None', async () => {
    renderInput({ key: 'tier', label: 'Tier', type: 'vocabulary', vocabulary_slug: 'risk', required: true });
    fireEvent.mouseDown(screen.getByLabelText(/Tier/));
    await screen.findByRole('option', { name: 'Low' });
    expect(screen.queryByRole('option', { name: 'None' })).not.toBeInTheDocument();
  });

  it('vocabulary select disables deprecated terms unless already selected', async () => {
    const onChange = renderInput({ key: 'tier', label: 'Tier', type: 'vocabulary', vocabulary_slug: 'risk' });
    fireEvent.mouseDown(screen.getByLabelText(/Tier/));
    expect(await screen.findByRole('option', { name: 'Legacy (deprecated)' })).toHaveAttribute('aria-disabled', 'true');
    fireEvent.click(screen.getByRole('option', { name: 'High' }));
    expect(onChange).toHaveBeenLastCalledWith('high');
  });

  it('a deprecated term that is already selected stays selectable', async () => {
    renderInput({ key: 'tier', label: 'Tier', type: 'vocabulary', vocabulary_slug: 'risk' }, { value: 'legacy' });
    fireEvent.mouseDown(screen.getByLabelText(/Tier/));
    expect(await screen.findByRole('option', { name: 'Legacy (deprecated)' })).not.toHaveAttribute('aria-disabled', 'true');
  });

  it('multi vocabulary emits arrays and undefined when emptied', async () => {
    const onChange = renderControlled({ key: 'regs', label: 'Regs', type: 'multi_vocabulary', vocabulary_slug: 'risk' }, ['low']);
    fireEvent.mouseDown(screen.getByLabelText(/Regs/));
    fireEvent.click(await screen.findByRole('option', { name: 'High' }));
    expect(onChange).toHaveBeenLastCalledWith(['low', 'high']);
    fireEvent.click(screen.getByRole('option', { name: 'Low' }));
    expect(onChange).toHaveBeenLastCalledWith(['high']);
    fireEvent.click(screen.getByRole('option', { name: 'High' }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it('string_list emits arrays and undefined when emptied', () => {
    const onChange = renderInput({ key: 'consumers', label: 'Consumers', type: 'string_list' }, { value: ['a'] });
    fireEvent.click(screen.getAllByTestId('CancelIcon')[0]);
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it('only enforced error-severity fields are natively required', () => {
    renderInput({ key: 'a', label: 'Advisory', type: 'string', required: true });
    expect(screen.getByLabelText(/Advisory/)).not.toBeRequired();
    renderInput({ key: 'b', label: 'Warned', type: 'string', required: true, severity: 'warning' }, { enforced: true });
    expect(screen.getByLabelText(/Warned/)).not.toBeRequired();
    renderInput({ key: 'c', label: 'Blocking', type: 'string', required: true }, { enforced: true });
    expect(screen.getByLabelText(/Blocking/)).toBeRequired();
  });

  it('unknown types fall back to a text input and disabled propagates', () => {
    renderInput({ key: 'x', label: 'Mystery', type: 'hologram' }, { disabled: true });
    expect(screen.getByLabelText(/Mystery/)).toBeDisabled();
  });

  it('shows warnings when there is no error and errors take precedence', () => {
    renderInput({ key: 'a', label: 'A', type: 'string' }, { warning: 'hmm' });
    expect(screen.getByText('hmm')).toBeInTheDocument();
    renderInput({ key: 'b', label: 'B', type: 'string' }, { warning: 'hmm2', error: 'boom' });
    expect(screen.getByText('boom')).toBeInTheDocument();
    expect(screen.queryByText('hmm2')).not.toBeInTheDocument();
  });
});
