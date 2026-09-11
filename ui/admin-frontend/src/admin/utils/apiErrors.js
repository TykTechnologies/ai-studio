/**
 * Typed errors raised by the API client for authorization failures, so pages
 * can tell "your role lacks this" apart from "this edition lacks this"
 * without inspecting HTTP status codes.
 *
 * Both keep the original axios `response` and `config` so any untouched
 * `error.response?.status` check keeps working.
 */

export const ERROR_CODES = Object.freeze({
  PERMISSION_DENIED: 'permission_denied',
  ENTERPRISE_REQUIRED: 'enterprise_required',
  UNAUTHENTICATED: 'unauthenticated',
  SYSTEM_ROLE: 'system_role',
  LAST_OWNER: 'last_owner',
  OWNER_USER_ONLY: 'owner_user_only',
  OWNER_REQUIRED: 'owner_required',
});

export const firstError = (error) => {
  const errors = error?.response?.data?.errors;
  return Array.isArray(errors) && errors.length > 0 ? errors[0] : null;
};

export const errorCode = (error) => error?.code || firstError(error)?.code || '';

export class PermissionDeniedError extends Error {
  constructor(detail, permission, cause) {
    super(detail || 'You do not have permission to perform this action');
    this.name = 'PermissionDeniedError';
    this.code = ERROR_CODES.PERMISSION_DENIED;
    this.isPermissionDenied = true;
    this.requiredPermission = permission || '';
    this.response = cause?.response;
    this.config = cause?.config;
    this.cause = cause;
  }
}

export class EnterpriseFeatureError extends Error {
  constructor(detail, cause) {
    super(detail || 'This feature requires the Enterprise Edition');
    this.name = 'EnterpriseFeatureError';
    this.code = ERROR_CODES.ENTERPRISE_REQUIRED;
    this.isEnterpriseFeature = true;
    this.response = cause?.response;
    this.config = cause?.config;
    this.cause = cause;
  }
}

export const isPermissionDenied = (error) =>
  error?.isPermissionDenied === true || errorCode(error) === ERROR_CODES.PERMISSION_DENIED;

/**
 * True for an edition/licence gate. A code-less 403 keeps its historical
 * meaning ("Community Edition") so a frontend deployed ahead of the backend
 * does not misreport upgrades as permission errors.
 */
export const isEnterpriseFeature = (error) => {
  if (error?.isEnterpriseFeature === true) return true;
  const code = errorCode(error);
  if (code === ERROR_CODES.ENTERPRISE_REQUIRED) return true;
  const status = error?.response?.status;
  if (status === 402) return true;
  return status === 403 && !code;
};

/** Wraps an axios error into a typed error when the body carries a code. */
export const classifyAuthError = (error) => {
  const status = error?.response?.status;
  const first = firstError(error);
  if (status === 403 && first?.code === ERROR_CODES.PERMISSION_DENIED) {
    return new PermissionDeniedError(first.detail, first.permission, error);
  }
  if (status === 402 || first?.code === ERROR_CODES.ENTERPRISE_REQUIRED) {
    return new EnterpriseFeatureError(first?.detail, error);
  }
  return error;
};
