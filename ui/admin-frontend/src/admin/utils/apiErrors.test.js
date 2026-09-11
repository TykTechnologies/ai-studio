import {
  classifyAuthError,
  isPermissionDenied,
  isEnterpriseFeature,
  PermissionDeniedError,
  EnterpriseFeatureError,
  ERROR_CODES,
} from './apiErrors';

const axiosError = (status, body, method = 'get') => ({
  message: `Request failed with status code ${status}`,
  response: { status, data: body },
  config: { method },
});

describe('apiErrors', () => {
  it('classifies a permission_denied 403 into a typed error that keeps the response', () => {
    const raw = axiosError(403, {
      errors: [{ title: 'Forbidden', detail: 'missing permission llms:write', code: 'permission_denied', permission: 'llms:write' }],
    });
    const err = classifyAuthError(raw);
    expect(err).toBeInstanceOf(PermissionDeniedError);
    expect(err.requiredPermission).toBe('llms:write');
    expect(err.message).toBe('missing permission llms:write');
    expect(err.response.status).toBe(403);
    expect(isPermissionDenied(err)).toBe(true);
    expect(isEnterpriseFeature(err)).toBe(false);
  });

  it('classifies enterprise_required (402) into an enterprise error', () => {
    const raw = axiosError(402, { errors: [{ title: 'Enterprise Feature', detail: 'upgrade', code: 'enterprise_required' }] });
    const err = classifyAuthError(raw);
    expect(err).toBeInstanceOf(EnterpriseFeatureError);
    expect(isEnterpriseFeature(err)).toBe(true);
    expect(isPermissionDenied(err)).toBe(false);
    expect(err.code).toBe(ERROR_CODES.ENTERPRISE_REQUIRED);
  });

  it('keeps the legacy meaning of a code-less 403 as an edition gate', () => {
    const raw = axiosError(403, { errors: [{ title: 'Enterprise Feature', detail: 'nope' }] });
    expect(classifyAuthError(raw)).toBe(raw);
    expect(isEnterpriseFeature(raw)).toBe(true);
    expect(isPermissionDenied(raw)).toBe(false);
  });

  it('reads codes off raw axios errors too', () => {
    const raw = axiosError(403, { errors: [{ code: 'permission_denied' }] });
    expect(isPermissionDenied(raw)).toBe(true);
    expect(isEnterpriseFeature(raw)).toBe(false);
  });

  it('leaves other errors alone', () => {
    const raw = axiosError(500, { errors: [{ detail: 'boom' }] });
    expect(classifyAuthError(raw)).toBe(raw);
    expect(isPermissionDenied(raw)).toBe(false);
    expect(isEnterpriseFeature(raw)).toBe(false);
  });
});
