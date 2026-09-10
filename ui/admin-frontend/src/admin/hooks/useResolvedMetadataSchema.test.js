import { renderHook, waitFor } from '@testing-library/react';
import useResolvedMetadataSchema from './useResolvedMetadataSchema';
import { useEdition } from '../context/EditionContext';
import { resolveMetadataSchema, getMetadataVocabularies, getMetadataUsers } from '../services/governedMetadataService';

jest.mock('../context/EditionContext');
jest.mock('../services/governedMetadataService', () => ({
  ...jest.requireActual('../services/governedMetadataService'),
  resolveMetadataSchema: jest.fn(),
  getMetadataVocabularies: jest.fn(),
  getMetadataUsers: jest.fn(),
}));

describe('useResolvedMetadataSchema', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useEdition.mockReturnValue({ isEnterprise: true });
    getMetadataVocabularies.mockResolvedValue([{ slug: 'risk', terms: [] }]);
    getMetadataUsers.mockResolvedValue([{ id: 3, attributes: { name: 'Cat', email: 'c@x.io' } }, { id: 4, attributes: { email: 'd@x.io' } }]);
  });

  it('returns an empty schema in Community Edition without calling the API', async () => {
    useEdition.mockReturnValue({ isEnterprise: false });
    const { result } = renderHook(() => useResolvedMetadataSchema('llm'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.fields).toEqual([]);
    expect(resolveMetadataSchema).not.toHaveBeenCalled();
  });

  it('loads fields sorted by order and only the lookups the fields need', async () => {
    resolveMetadataSchema.mockResolvedValue({
      fields: [{ key: 'b', type: 'string', order: 2 }, { key: 'a', type: 'vocabulary', vocabulary_slug: 'risk', order: 1 }],
      enforcement: 'enforce',
    });
    const { result } = renderHook(() => useResolvedMetadataSchema('tool'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.fields.map((f) => f.key)).toEqual(['a', 'b']);
    expect(result.current.enforcement).toBe('enforce');
    expect(result.current.vocabulariesBySlug.risk).toBeDefined();
    expect(getMetadataUsers).not.toHaveBeenCalled();
  });

  it('loads users for user fields and indexes them by id with an email fallback', async () => {
    resolveMetadataSchema.mockResolvedValue({ fields: [{ key: 'owner', type: 'user' }], enforcement: 'advisory' });
    const { result } = renderHook(() => useResolvedMetadataSchema('llm'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(getMetadataVocabularies).not.toHaveBeenCalled();
    expect(result.current.usersById).toEqual({ 3: 'Cat', 4: 'd@x.io' });
  });

  it('treats a null resolution (403) or an error as no schema', async () => {
    resolveMetadataSchema.mockResolvedValue(null);
    const { result } = renderHook(() => useResolvedMetadataSchema('llm'));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.fields).toEqual([]);

    resolveMetadataSchema.mockRejectedValue(new Error('boom'));
    const { result: r2 } = renderHook(() => useResolvedMetadataSchema('llm'));
    await waitFor(() => expect(r2.current.loading).toBe(false));
    expect(r2.current.fields).toEqual([]);
  });

  it('reloads when the object type changes', async () => {
    resolveMetadataSchema.mockResolvedValue({ fields: [{ key: 'x', type: 'string' }], enforcement: 'advisory' });
    const { result, rerender } = renderHook(({ type }) => useResolvedMetadataSchema(type), { initialProps: { type: 'llm' } });
    await waitFor(() => expect(result.current.fields).toHaveLength(1));
    rerender({ type: 'tool' });
    await waitFor(() => expect(resolveMetadataSchema).toHaveBeenLastCalledWith('tool'));
  });
});
