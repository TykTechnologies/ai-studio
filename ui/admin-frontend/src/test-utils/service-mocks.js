export const mockTeamsService = {
  getTeam: jest.fn(),
  createTeam: jest.fn(),
  updateTeam: jest.fn(),
  deleteTeam: jest.fn(),
};

export const mockErrorHandler = {
  handleApiError: jest.fn(e => new Error(e.message || 'API Error')),
};

export const mockNavigate = jest.fn();

export const mockRoleBadgeConfigs = {
  'Chat user': {
    text: 'Chat user',
    textVariant: 'bodyMediumSemiBold',
    textColor: 'text.primary',
    bgColor: 'background.buttonPrimaryOutlineHover'
  },
  'Developer': {
    text: 'Developer',
    textVariant: 'bodyMediumSemiBold',
    textColor: 'text.primary',
    bgColor: 'background.surfaceBrandDefaultPortal'
  },
  'Admin': {
    text: 'Admin',
    textVariant: 'bodyMediumSemiBold',
    textColor: 'text.primary',
    bgColor: 'background.surfaceBrandDefaultDashboard'
  }
};

export const mockUserRoles = [
  {
    value: 'Chat user',
    label: 'Chat user',
    connector: 'can access',
    main: 'Chats'
  },
  {
    value: 'Developer',
    label: 'Developer',
    connector: 'can access',
    main: 'AI portal and Chats'
  },
  {
    value: 'Admin',
    label: 'Admin',
    connector: 'can access',
    main: 'Admin, AI portal and Chats'
  }
];

export const setupTeamsServiceMocks = () => {
  jest.mock('../admin/services/teamsService', () => ({
    teamsService: mockTeamsService
  }));
};

export const setupErrorHandlerMock = () => {
  jest.mock('../admin/services/utils/errorHandler', () => mockErrorHandler);
};

export const setupReactRouterMock = () => {
  jest.mock('react-router-dom', () => ({
    ...jest.requireActual('react-router-dom'),
    useNavigate: () => mockNavigate,
  }));
};

export const setupRoleConfigMocks = () => {
  jest.mock('../../groups/utils/roleBadgeConfig', () => ({
    roleBadgeConfigs: mockRoleBadgeConfigs
  }));
  
  jest.mock('../utils/userRolesConfig', () => ({
    USER_ROLES: mockUserRoles
  }));
}; 