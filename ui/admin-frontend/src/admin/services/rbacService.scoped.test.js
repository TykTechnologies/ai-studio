import apiClient from '../utils/apiClient';
import { syncScopedBindings, syncSubjectRoles } from './rbacService';

jest.mock('../utils/apiClient', () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), delete: jest.fn() },
}));

const binding = (id, roleId, scopeType = '', scopeId = '') => ({
  id: String(id),
  attributes: { role_id: roleId, scope_type: scopeType, scope_id: scopeId },
});

describe('scoped role bindings', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockResolvedValue({
      data: {
        data: [
          binding(1, 10), // global
          binding(2, 20, 'workspace', '5'),
          binding(3, 30, 'workspace', '5'),
          binding(4, 20, 'workspace', '6'), // another Workspace
        ],
      },
    });
    apiClient.post.mockResolvedValue({ data: { data: {} } });
    apiClient.delete.mockResolvedValue({});
  });

  it('syncScopedBindings changes only the bindings of that Workspace', async () => {
    const res = await syncScopedBindings(5, 'user', 9, [20, 40]);
    expect(res.added).toEqual([40]);
    expect(res.removed).toEqual([30]);
    expect(res.failed).toEqual([]);
    expect(apiClient.delete).toHaveBeenCalledTimes(1);
    expect(apiClient.delete).toHaveBeenCalledWith('/rbac/bindings/3');
    expect(apiClient.post).toHaveBeenCalledWith('/rbac/bindings', {
      subject_type: 'user',
      subject_id: 9,
      role_id: 40,
      scope_type: 'workspace',
      scope_id: '5',
    });
  });

  it('syncSubjectRoles still never touches scoped bindings', async () => {
    const res = await syncSubjectRoles('user', 9, []);
    expect(res.removed).toEqual([10]);
    expect(apiClient.delete).toHaveBeenCalledTimes(1);
    expect(apiClient.delete).toHaveBeenCalledWith('/rbac/bindings/1');
  });
});
