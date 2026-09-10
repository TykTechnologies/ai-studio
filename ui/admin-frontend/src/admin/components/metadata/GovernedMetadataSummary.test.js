import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import testTheme from '../../utils/testTheme';
import GovernedMetadataSummary, { MetadataStatusChip } from './GovernedMetadataSummary';
import { useEdition } from '../../context/EditionContext';
import { resolveMetadataSchema, getMetadataVocabularies, getMetadataUsers } from '../../services/governedMetadataService';

jest.mock('../../context/EditionContext');
jest.mock('../../services/governedMetadataService', () => ({
  ...jest.requireActual('../../services/governedMetadataService'),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
}));

const FIELDS = [
  { key: 'owner', label: 'Owner', type: 'user', order: 1 },
  { key: 'tier', label: 'Risk tier', type: 'vocabulary', vocabulary_slug: 'risk', order: 2 },
  { key: 'regs', label: 'Regulations', type: 'multi_vocabulary', vocabulary_slug: 'regs', order: 3 },
  { key: 'consumers', label: 'Consumers', type: 'string_list', order: 4 },
  { key: 'approved', label: 'Approved', type: 'boolean', order: 5 },
  { key: 'notes', label: 'Notes', type: 'text', order: 6 },
];

const renderSummary = (props) =>
  render(
    <ThemeProvider theme={testTheme}>
      <GovernedMetadataSummary objectType="llm" {...props} />
    </ThemeProvider>
  );

describe('GovernedMetadataSummary', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: true });
    resolveMetadataSchema.mockResolvedValue({ fields: FIELDS, enforcement: 'advisory' });
    getMetadataVocabularies.mockResolvedValue([
      { slug: 'risk', terms: [{ value: 'high', label: 'High' }] },
      { slug: 'regs', terms: [{ value: 'gdpr', label: 'GDPR' }, { value: 'hipaa', label: 'HIPAA' }] },
    ]);
    getMetadataUsers.mockResolvedValue([{ id: 42, attributes: { name: 'Alice', email: 'a@x.io' } }]);
  });

  it('renders nothing in CE or when no fields apply', async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    const { container } = renderSummary({ values: { owner: 42 } });
    await waitFor(() => expect(container).toBeEmptyDOMElement());
    useEdition.mockReturnValue({ isEnterprise: true });
    resolveMetadataSchema.mockResolvedValue({ fields: [] });
    const { container: c2 } = renderSummary({ values: { owner: 42 } });
    await waitFor(() => expect(resolveMetadataSchema).toHaveBeenCalled());
    await waitFor(() => expect(c2).toBeEmptyDOMElement());
  });

  it('formats every field type and shows the status chip', async () => {
    renderSummary({
      status: 'warnings',
      values: { owner: 42, tier: 'high', regs: ['gdpr', 'hipaa'], consumers: ['billing'], approved: false, notes: 'hello' },
    });
    expect(await screen.findByText('Governance Metadata')).toBeInTheDocument();
    expect(screen.getByText('warnings')).toBeInTheDocument();
    expect(screen.getByText('Alice')).toBeInTheDocument();
    expect(screen.getByText('High')).toBeInTheDocument();
    expect(screen.getByText('GDPR')).toBeInTheDocument();
    expect(screen.getByText('HIPAA')).toBeInTheDocument();
    expect(screen.getByText('billing')).toBeInTheDocument();
    expect(screen.getByText('No')).toBeInTheDocument();
    expect(screen.getByText('hello')).toBeInTheDocument();
  });

  it('shows a dash for missing values and falls back for unknown users and terms', async () => {
    renderSummary({ values: { owner: 7, tier: 'unknown' } });
    expect(await screen.findByText('User 7')).toBeInTheDocument();
    expect(screen.getByText('unknown')).toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(3);
  });

  it('status chip maps statuses to colours and hides when empty', () => {
    const { container } = render(<MetadataStatusChip status="" />);
    expect(container).toBeEmptyDOMElement();
    render(<MetadataStatusChip status="invalid" />);
    expect(screen.getByText('invalid').closest('.MuiChip-root')).toHaveClass('MuiChip-colorError');
  });
});
