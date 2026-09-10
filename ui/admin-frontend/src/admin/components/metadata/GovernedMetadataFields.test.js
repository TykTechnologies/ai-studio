import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import testTheme from '../../utils/testTheme';
import GovernedMetadataFields from './GovernedMetadataFields';
import { useEdition } from '../../context/EditionContext';
import {
  resolveMetadataSchema,
  getMetadataVocabularies,
  getMetadataUsers,
  validateObjectMetadata,
} from '../../services/governedMetadataService';

jest.mock('../../context/EditionContext');
jest.mock('../../services/governedMetadataService', () => ({
  ...jest.requireActual('../../services/governedMetadataService'),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
  validateObjectMetadata: jest.fn(),
}));

const FIELDS = [
  { key: 'risk_tier', label: 'Risk tier', type: 'vocabulary', vocabulary_slug: 'risk_tier', required: true, order: 1 },
  { key: 'support_contact', label: 'Support contact', type: 'email', order: 2 },
  { key: 'owner', label: 'Owner', type: 'user', order: 3 },
  { key: 'approved_consumers', label: 'Approved consumers', type: 'string_list', order: 4 },
  { key: 'expires', label: 'Expires', type: 'date', warn_if_past: true, order: 5 },
  { key: 'score', label: 'Score', type: 'number', order: 6 },
  { key: 'flag', label: 'Flag', type: 'boolean', order: 7 },
];

// Stateful harness: the inputs are controlled, so the parent must feed values back.
const Harness = ({ onChange = () => {}, initial = {}, ...props }) => {
  const [value, setValue] = React.useState(initial);
  return (
    <GovernedMetadataFields
      objectType="llm"
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
      {...props}
    />
  );
};

const renderFields = (props) =>
  render(
    <ThemeProvider theme={testTheme}>
      <Harness {...props} />
    </ThemeProvider>
  );

describe('GovernedMetadataFields', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: true });
    resolveMetadataSchema.mockResolvedValue({ fields: FIELDS, enforcement: 'advisory' });
    getMetadataVocabularies.mockResolvedValue([
      { slug: 'risk_tier', name: 'Risk', terms: [{ value: 'low', label: 'Low' }, { value: 'high', label: 'High' }] },
    ]);
    getMetadataUsers.mockResolvedValue([{ id: 42, attributes: { name: 'Alice', email: 'alice@x.io' } }]);
    validateObjectMetadata.mockResolvedValue({ valid: true, errors: [], warnings: [] });
  });

  it('renders nothing in Community Edition', async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    const { container } = renderFields();
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    expect(resolveMetadataSchema).not.toHaveBeenCalled();
  });

  it('renders nothing when no fields resolve', async () => {
    resolveMetadataSchema.mockResolvedValue({ fields: [], enforcement: 'advisory' });
    const { container } = renderFields();
    await waitFor(() => expect(resolveMetadataSchema).toHaveBeenCalledWith('llm'));
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it('renders one input per field and emits normalised values', async () => {
    const onChange = jest.fn();
    renderFields({ onChange, defaultExpanded: true });
    await screen.findByText('Governance Metadata');

    expect(screen.getByLabelText(/Support contact/)).toBeInTheDocument();
    expect(screen.getByLabelText('Approved consumers')).toBeInTheDocument();
    expect(screen.getByLabelText(/Expires/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Score/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Flag/)).toBeInTheDocument();
    expect(screen.getByText('Advisory')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/Support contact/), { target: { value: 'help@x.io' } });
    expect(onChange).toHaveBeenLastCalledWith({ support_contact: 'help@x.io' });

    fireEvent.change(screen.getByLabelText(/Score/), { target: { value: '3' } });
    expect(onChange).toHaveBeenLastCalledWith({ support_contact: 'help@x.io', score: 3 });

    fireEvent.change(screen.getByLabelText(/Support contact/), { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith({ score: 3 });

    await waitFor(() => expect(validateObjectMetadata).toHaveBeenCalledWith('llm', { score: 3 }));
  });

  it('shows server errors under the matching field and non-field errors at the top', async () => {
    renderFields({ errors: { support_contact: 'bad email', _: 'policy says no' } });
    await screen.findByText('Governance Metadata');
    expect(await screen.findByText('bad email')).toBeInTheDocument();
    expect(screen.getByText('policy says no')).toBeInTheDocument();
  });

  it('forces the section open and marks it enforced when the schema enforces', async () => {
    resolveMetadataSchema.mockResolvedValue({ fields: FIELDS, enforcement: 'enforce' });
    renderFields();
    expect(await screen.findByText('Enforced')).toBeInTheDocument();
    expect(await screen.findByLabelText(/Support contact/)).toBeVisible();
  });

  it('surfaces live validation warnings', async () => {
    validateObjectMetadata.mockResolvedValue({
      valid: true,
      errors: [],
      warnings: [{ field: 'expires', code: 'expired', message: 'Expires is in the past' }],
    });
    renderFields({ defaultExpanded: true });
    await screen.findByText('Governance Metadata');
    fireEvent.change(screen.getByLabelText(/Expires/), { target: { value: '2000-01-01' } });
    expect(await screen.findByText('Expires is in the past')).toBeInTheDocument();
  });
});
