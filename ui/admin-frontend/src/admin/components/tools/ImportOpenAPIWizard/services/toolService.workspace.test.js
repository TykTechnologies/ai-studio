import apiClient from '../../../../utils/apiClient';
import { createTool } from './toolService';

// Workspaces .54: a tool imported from an OpenAPI document lands in the
// Workspace chosen in the wizard; with none chosen the landing rule applies.

jest.mock('../../../../utils/apiClient', () => ({
  __esModule: true,
  default: { post: jest.fn() },
}));
jest.mock('../utils/specUtils', () => ({
  validateSpec: jest.fn(),
  encodeSpec: jest.fn((s) => s),
}));

const base = { name: 'Weather', description: 'd', tool_type: 'REST', oas_spec: '{}', privacy_score: 25 };
const sent = () => apiClient.post.mock.calls[0][1].data.attributes;

describe('createTool Workspace', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.post.mockResolvedValue({ data: { data: { id: '1' } } });
  });

  it('sends the chosen Workspace', async () => {
    await createTool({ ...base, workspace_id: 3 });
    expect(sent().workspace_id).toBe(3);
  });

  it('sends none when none is chosen', async () => {
    await createTool({ ...base, workspace_id: null });
    expect(sent()).not.toHaveProperty('workspace_id');
  });
});
