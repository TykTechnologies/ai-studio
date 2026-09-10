import * as svc from './governedMetadataService';
import apiClient from '../utils/apiClient';
import { handleApiError } from './utils/errorHandler';

jest.mock('../utils/apiClient');
jest.mock('./utils/errorHandler');

describe('governedMetadataService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    handleApiError.mockImplementation((error) => error);
  });

  test('isGovernedMetadataAvailable returns the flag and false on 403', async () => {
    apiClient.get.mockResolvedValueOnce({ data: { available: true } });
    expect(await svc.isGovernedMetadataAvailable()).toBe(true);
    expect(apiClient.get).toHaveBeenCalledWith('/metadata/available');

    apiClient.get.mockRejectedValueOnce({ response: { status: 403 } });
    expect(await svc.isGovernedMetadataAvailable()).toBe(false);
  });

  test('getMetadataSchemas flattens JSON:API items', async () => {
    apiClient.get.mockResolvedValueOnce({
      data: { data: [{ id: 3, type: 'MetadataSchema', attributes: { name: 'Core', slug: 'core', fields: [] } }] },
    });
    const result = await svc.getMetadataSchemas();
    expect(apiClient.get).toHaveBeenCalledWith('/metadata/schemas');
    expect(result).toEqual([{ id: 3, name: 'Core', slug: 'core', fields: [] }]);
  });

  test('createMetadataSchema posts a JSON:API body', async () => {
    apiClient.post.mockResolvedValueOnce({ data: { data: { id: 1, attributes: { name: 'Core' } } } });
    const result = await svc.createMetadataSchema({ name: 'Core', fields: [] });
    expect(apiClient.post).toHaveBeenCalledWith('/metadata/schemas', {
      data: { type: 'MetadataSchema', attributes: { name: 'Core', fields: [] } },
    });
    expect(result).toEqual({ id: 1, name: 'Core' });
  });

  test('updateMetadataSchema and deleteMetadataSchema hit the id routes', async () => {
    apiClient.patch.mockResolvedValueOnce({ data: { data: { id: 1, attributes: { enforcement: 'enforce' } } } });
    await svc.updateMetadataSchema(1, { enforcement: 'enforce' });
    expect(apiClient.patch).toHaveBeenCalledWith('/metadata/schemas/1', {
      data: { type: 'MetadataSchema', attributes: { enforcement: 'enforce' } },
    });
    apiClient.delete.mockResolvedValueOnce({});
    await svc.deleteMetadataSchema(1);
    expect(apiClient.delete).toHaveBeenCalledWith('/metadata/schemas/1');
  });

  test('resolveMetadataSchema passes object_type and returns null on 403', async () => {
    apiClient.get.mockResolvedValueOnce({ data: { fields: [{ key: 'a' }], enforcement: 'enforce' } });
    const resolved = await svc.resolveMetadataSchema('llm');
    expect(apiClient.get).toHaveBeenCalledWith('/metadata/schemas/resolve', { params: { object_type: 'llm' } });
    expect(resolved.enforcement).toBe('enforce');

    apiClient.get.mockRejectedValueOnce({ response: { status: 403 } });
    expect(await svc.resolveMetadataSchema('llm')).toBeNull();
  });

  test('vocabulary CRUD uses the vocabulary routes', async () => {
    apiClient.post.mockResolvedValueOnce({ data: { data: { id: 2, attributes: { slug: 'risk_tier' } } } });
    const created = await svc.createMetadataVocabulary({ name: 'Risk', terms: [{ value: 'low' }] });
    expect(apiClient.post).toHaveBeenCalledWith('/metadata/vocabularies', {
      data: { type: 'MetadataVocabulary', attributes: { name: 'Risk', terms: [{ value: 'low' }] } },
    });
    expect(created.slug).toBe('risk_tier');

    apiClient.delete.mockResolvedValueOnce({});
    await svc.deleteMetadataVocabulary(2);
    expect(apiClient.delete).toHaveBeenCalledWith('/metadata/vocabularies/2');
  });

  test('validateObjectMetadata and setObjectMetadata send the expected bodies', async () => {
    apiClient.post.mockResolvedValueOnce({ data: { valid: false, errors: [{ field: 'risk_tier', code: 'required', message: 'x' }] } });
    const result = await svc.validateObjectMetadata('llm', { a: 1 });
    expect(apiClient.post).toHaveBeenCalledWith('/metadata/validate', { object_type: 'llm', values: { a: 1 } });
    expect(result.valid).toBe(false);

    apiClient.put.mockResolvedValueOnce({ data: { data: { values: { a: 1 } } } });
    await svc.setObjectMetadata('llm', '7', { a: 1 }, true);
    expect(apiClient.put).toHaveBeenCalledWith('/metadata/objects/llm/7', { values: { a: 1 }, merge: true });
  });

  test('getObjectMetadata returns null on 404', async () => {
    apiClient.get.mockRejectedValueOnce({ response: { status: 404 } });
    expect(await svc.getObjectMetadata('llm', '1')).toBeNull();
  });

  test('getMetadataUsers asks for every user', async () => {
    apiClient.get.mockResolvedValueOnce({ data: { data: [{ id: 1 }] } });
    expect(await svc.getMetadataUsers()).toEqual([{ id: 1 }]);
    expect(apiClient.get).toHaveBeenCalledWith('/users', { params: { all: true } });
  });

  test('getMetadataComplianceReport forwards filters and throws handled errors', async () => {
    apiClient.get.mockResolvedValueOnce({ data: { entries: [], counts: {} } });
    await svc.getMetadataComplianceReport({ status: 'missing' });
    expect(apiClient.get).toHaveBeenCalledWith('/metadata/compliance', { params: { status: 'missing' } });

    const boom = new Error('Network error');
    apiClient.get.mockRejectedValueOnce(boom);
    handleApiError.mockReturnValueOnce(new Error('Handled error'));
    await expect(svc.getMetadataComplianceReport()).rejects.toThrow('Handled error');
    expect(handleApiError).toHaveBeenCalledWith(boom);
  });

  describe('extractGovernedMetadataErrors', () => {
    test('maps pointers to field keys and collects unpointed errors under "_"', () => {
      const error = {
        response: {
          status: 422,
          data: {
            errors: [
              { title: 'Metadata Validation Failed', detail: 'Risk tier is required', source: { pointer: '/data/attributes/governed_metadata/risk_tier' } },
              { title: 'Metadata Validation Failed', detail: 'bad email', source: { pointer: '/data/attributes/governed_metadata/support_contact' } },
              { title: 'Metadata Change Rejected', detail: 'policy says no', code: 'hook_rejected' },
            ],
          },
        },
      };
      expect(svc.extractGovernedMetadataErrors(error)).toEqual({
        risk_tier: 'Risk tier is required',
        support_contact: 'bad email',
        _: 'policy says no',
      });
    });

    test('returns {} for non-422 errors', () => {
      expect(svc.extractGovernedMetadataErrors({ response: { status: 500, data: { errors: [{ detail: 'x' }] } } })).toEqual({});
      expect(svc.extractGovernedMetadataErrors(new Error('nope'))).toEqual({});
    });
  });

  test('validationResultToFieldMessages splits errors and warnings by field', () => {
    expect(
      svc.validationResultToFieldMessages({
        errors: [{ field: 'a', message: 'bad' }, { field: 'a', message: 'worse' }],
        warnings: [{ field: 'b', message: 'meh' }],
      })
    ).toEqual({ errors: { a: 'bad; worse' }, warnings: { b: 'meh' } });
    expect(svc.validationResultToFieldMessages(null)).toEqual({ errors: {}, warnings: {} });
  });

  test('normalizeGovernedMetadataValues drops empty values', () => {
    expect(svc.normalizeGovernedMetadataValues({ a: '', b: ' x ', c: null, d: [], e: ['y'], f: 0, g: false, h: undefined })).toEqual({
      b: ' x ',
      e: ['y'],
      f: 0,
      g: false,
    });
  });
});
