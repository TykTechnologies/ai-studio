import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import '@testing-library/jest-dom';
import FieldDefinitionDialog, { fromForm, emptyField } from './FieldDefinitionDialog';

const vocabularies = [{ slug: 'risk', name: 'Risk' }];

const renderDialog = (props = {}) => {
  const onSave = jest.fn();
  const onClose = jest.fn();
  render(<FieldDefinitionDialog open field={null} existingKeys={['taken']} vocabularies={vocabularies} onSave={onSave} onClose={onClose} {...props} />);
  return { onSave, onClose, dialog: screen.getByRole('dialog') };
};

const pickType = async (dialog, label) => {
  fireEvent.mouseDown(within(dialog).getByLabelText(/^Type/));
  fireEvent.click(await screen.findByRole('option', { name: label }));
};

describe('FieldDefinitionDialog', () => {
  it('rejects invalid and duplicate keys', () => {
    const { onSave, dialog } = renderDialog();
    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: 'Bad Key' } });
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(within(dialog).getByText(/lowercase letters/)).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: 'taken' } });
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(within(dialog).getByText(/already exists/)).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('requires a vocabulary for vocabulary types and shows type-specific controls', async () => {
    const { onSave, dialog } = renderDialog();
    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: 'tier' } });
    await pickType(dialog, 'Vocabulary (single value)');
    expect(within(dialog).queryByLabelText(/Pattern/)).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(within(dialog).getByText('Choose a vocabulary')).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();

    fireEvent.mouseDown(within(dialog).getByLabelText(/^Vocabulary/));
    fireEvent.click(await screen.findByRole('option', { name: 'Risk (risk)' }));
    fireEvent.click(within(dialog).getByLabelText('Required'));
    fireEvent.click(within(dialog).getByLabelText('Sent to gateways'));
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ key: 'tier', label: 'tier', type: 'vocabulary', vocabulary_slug: 'risk', required: true, gateway_visible: true, portal_visible: false }));
  });

  it('validates number min/max and only shows the date switch for dates', async () => {
    const { onSave, dialog } = renderDialog();
    fireEvent.change(within(dialog).getByLabelText(/^Key/), { target: { value: 'score' } });
    await pickType(dialog, 'Number');
    expect(within(dialog).queryByLabelText('Warn when in the past')).not.toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText(/^Min/), { target: { value: '9' } });
    fireEvent.change(within(dialog).getByLabelText(/^Max/), { target: { value: '1' } });
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(within(dialog).getByText('Min must not exceed max')).toBeInTheDocument();
    fireEvent.change(within(dialog).getByLabelText(/^Max/), { target: { value: '10' } });
    fireEvent.click(within(dialog).getByText('Add field'));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ key: 'score', type: 'number', min: 9, max: 10 }));

    await pickType(dialog, 'Date');
    expect(within(dialog).getByLabelText('Warn when in the past')).toBeInTheDocument();
  });

  it('edits an existing field with the key locked and cancel closes', () => {
    const { onSave, onClose, dialog } = renderDialog({ field: { key: 'owner', label: 'Owner', type: 'string', pattern: '^x', max_length: 5 } });
    expect(within(dialog).getByText('Edit Field')).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/^Key/)).toBeDisabled();
    expect(within(dialog).getByLabelText(/Pattern/)).toHaveValue('^x');
    fireEvent.change(within(dialog).getByLabelText(/^Label/), { target: { value: 'Business owner' } });
    fireEvent.click(within(dialog).getByText('Update field'));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ key: 'owner', label: 'Business owner', pattern: '^x', max_length: 5 }));
    fireEvent.click(within(dialog).getByText('Cancel'));
    expect(onClose).toHaveBeenCalled();
  });

  it('fromForm drops constraints that do not apply to the type', () => {
    const base = { ...emptyField(), key: 'k', label: '', type: 'boolean', pattern: '^x', max_length: '5', min: '1', max: '2', vocabulary_slug: 'risk', warn_if_past: true };
    const out = fromForm(base);
    expect(out).toEqual({ key: 'k', label: 'k', description: '', type: 'boolean', required: false, required_on_publish: false, severity: 'error', portal_visible: false, gateway_visible: false });
    expect(fromForm({ ...base, type: 'text' })).toMatchObject({ pattern: '^x', max_length: 5 });
    expect(fromForm({ ...base, type: 'date' })).toMatchObject({ warn_if_past: true });
    expect(fromForm({ ...base, type: 'multi_vocabulary' })).toMatchObject({ vocabulary_slug: 'risk' });
  });
});
