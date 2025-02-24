import { render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import App from './App';

// Mock react-markdown and related modules
jest.mock('react-markdown', () => () => null);
jest.mock('remark-gfm', () => () => null);

// Mock chart.js and related modules
jest.mock('chart.js', () => ({
  Chart: {
    register: jest.fn(),
    defaults: { font: { family: 'Arial' } }
  },
  registerables: [],
  CategoryScale: jest.fn(),
  LinearScale: jest.fn(),
  PointElement: jest.fn(),
  LineElement: jest.fn(),
  Title: jest.fn(),
  Tooltip: jest.fn(),
  Legend: jest.fn(),
  Filler: jest.fn(),
  _adapters: { _date: {} }
}));

jest.mock('react-chartjs-2', () => ({
  Line: () => null,
  Bar: () => null
}));

jest.mock('chartjs-adapter-date-fns', () => ({}));

// Mock the config module
jest.mock('./config', () => {
  const mockConfig = {
    API_BASE_URL: 'http://localhost:3000',
    features: {
      docs_url: 'http://docs.example.com',
      feature_chat: true,
      feature_portal: true
    }
  };
  return {
    loadConfig: jest.fn().mockResolvedValue(mockConfig),
    getConfig: jest.fn().mockReturnValue(mockConfig)
  };
});

// Create mock functions
const mockGet = jest.fn();
const mockPost = jest.fn();
const mockNavigate = jest.fn();
const mockApiGet = jest.fn();

// Mock the modules
jest.mock('./admin/utils/pubClient', () => ({
  __esModule: true,
  default: {
    get: (...args) => mockGet(...args),
    post: (...args) => mockPost(...args)
  },
  reinitializePubClient: jest.fn()
}));

jest.mock('./admin/utils/apiClient', () => {
  const mockClient = {
    get: (...args) => mockApiGet(...args)
  };
  return {
    __esModule: true,
    default: mockClient,
    reinitializeApiClient: jest.fn()
  };
});

// Mock system features hook
jest.mock('./admin/hooks/useSystemFeatures', () => {
  const mockFeatures = {
    docs_url: 'http://docs.example.com',
    feature_chat: true,
    feature_portal: true
  };
  return {
    __esModule: true,
    default: () => ({
      features: mockFeatures,
      loading: false,
      error: null
    })
  };
});

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockNavigate,
  BrowserRouter: ({ children }) => children
}));

// Suppress expected console errors in tests
beforeAll(() => {
  jest.spyOn(console, 'error').mockImplementation((...args) => {
    // Allow unexpected errors to show up in the console
    if (!args[0].includes('Error fetching system features') && 
        !args[0].includes('Unexpected status code') &&
        !args[0].includes('Warning: Received `true` for a non-boolean attribute')) {
      console.log(...args);
    }
  });
});

afterAll(() => {
  console.error.mockRestore();
});

const renderWithRouter = (ui, { route = '/' } = {}) => {
  return render(
    <MemoryRouter initialEntries={[route]}>
      {ui}
    </MemoryRouter>
  );
};

describe('App Component', () => {
  beforeEach(() => {
    // Clear all mocks before each test
    jest.clearAllMocks();
    mockNavigate.mockReset();
    mockGet.mockReset();
    mockPost.mockReset();
    mockApiGet.mockReset();
  });

  test('shows loading spinner initially', async () => {
    mockGet.mockImplementation(() => new Promise(() => {})); // Never resolves to keep loading state
    renderWithRouter(<App />);
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  test('shows error message when initialization fails', async () => {
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.reject(new Error('Failed to load config'));
      }
      return Promise.resolve({ status: 200 });
    });
    
    renderWithRouter(<App />);
    
    await waitFor(() => {
      expect(screen.getByText('Failed to check authentication status.')).toBeInTheDocument();
    });
  });

  test('redirects to login when user is not authenticated', async () => {
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ 
          status: 200,
          data: {
            API_BASE_URL: 'http://localhost:3000'
          }
        });
      }
      if (url === '/common/me') {
        return Promise.reject({ response: { status: 401 } });
      }
      return Promise.resolve({ status: 200 });
    });
    
    renderWithRouter(<App />, { route: '/login' });
    
    await waitFor(() => {
      // Look for the heading specifically to avoid multiple matches
      expect(screen.getByRole('heading', { level: 1, name: 'Login' })).toBeInTheDocument();
    });
  });

  test('redirects admin users to admin dashboard when authenticated', async () => {
    // Mock successful initialization and auth check
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ 
          status: 200,
          data: {
            API_BASE_URL: 'http://localhost:3000'
          }
        });
      }
      if (url === '/common/me') {
        return Promise.resolve({
          status: 200,
          data: {
            attributes: {
              is_admin: true,
              entitlements: {
                chats: [],
                catalogues: [],
                data_catalogues: []
              },
              ui_options: {
                show_chat: true,
                show_portal: true
              }
            }
          }
        });
      }
      return Promise.resolve({ status: 200 });
    });

    // Mock API client responses for admin dashboard
    mockApiGet.mockImplementation((url) => {
      if (url === '/llms') {
        return Promise.resolve({ data: { data: [] } });
      }
      if (url === '/chats') {
        return Promise.resolve({ data: { data: [] } });
      }
      if (url.includes('/analytics/')) {
        return Promise.resolve({ 
          data: {
            labels: [],
            data: []
          }
        });
      }
      return Promise.resolve({
        data: {
          features: {
            docs_url: 'http://docs.example.com',
            feature_chat: true,
            feature_portal: true
          }
        }
      });
    });

    renderWithRouter(<App />, { route: '/' });

    // Wait for either the navigation or the dashboard content to appear
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith(
        expect.stringMatching(/^\/admin\/(dashboard|dash)$/),
        expect.objectContaining({ replace: true })
      );
    }, { timeout: 3000 });
  });

  test('redirects regular users to portal dashboard when authenticated', async () => {
    // Mock successful initialization and auth check
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ 
          status: 200,
          data: {
            API_BASE_URL: 'http://localhost:3000'
          }
        });
      }
      if (url === '/common/me') {
        return Promise.resolve({
          status: 200,
          data: {
            attributes: {
              is_admin: false,
              entitlements: {
                chats: [],
                catalogues: [],
                data_catalogues: []
              },
              ui_options: {
                show_chat: true,
                show_portal: true
              }
            }
          }
        });
      }
      if (url === '/common/apps') {
        return Promise.resolve({
          data: { data: [] }
        });
      }
      if (url.includes('/common/history')) {
        return Promise.resolve({
          data: { data: [] },
          headers: { 'x-total-pages': '1' }
        });
      }
      return Promise.resolve({ status: 200 });
    });

    // Mock API client responses for portal dashboard
    mockApiGet.mockImplementation((url) => {
      return Promise.resolve({
        data: {
          features: {
            docs_url: 'http://docs.example.com',
            feature_chat: true,
            feature_portal: true
          }
        }
      });
    });

    renderWithRouter(<App />, { route: '/' });

    // Wait for portal tab to be selected
    await waitFor(() => {
      expect(screen.getByRole('tab', { name: 'Developer Portal' })).toHaveAttribute('aria-selected', 'true');
    });

    // Then check for either navigation or overview text
    await waitFor(() => {
      const overviewText = screen.queryByText('Overview');
      const navigationCalled = mockNavigate.mock.calls.some(
        call => call[0] === '/portal/dashboard' && call[1]?.replace === true
      );
      
      if (!overviewText && !navigationCalled) {
        throw new Error('Neither Overview text nor navigation to /portal/dashboard detected');
      }
    }, { timeout: 3000 });
  });

  test('handles unexpected status codes gracefully', async () => {
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ 
          status: 200,
          data: {
            API_BASE_URL: 'http://localhost:3000'
          }
        });
      }
      if (url === '/common/me') {
        return Promise.reject({ response: { status: 500 } });
      }
      return Promise.resolve({ status: 200 });
    });
    
    renderWithRouter(<App />, { route: '/login' });
    
    await waitFor(() => {
      // First we should see the error message
      expect(screen.getByText('Failed to check authentication status.')).toBeInTheDocument();
    });
  });

  test('public routes are accessible without authentication', async () => {
    mockGet.mockImplementation((url) => {
      if (url === '/auth/config') {
        return Promise.resolve({ 
          status: 200,
          data: {
            API_BASE_URL: 'http://localhost:3000'
          }
        });
      }
      if (url === '/common/me') {
        return Promise.reject({ response: { status: 401 } });
      }
      return Promise.resolve({ status: 200 });
    });
    
    renderWithRouter(<App />, { route: '/login' });
    
    await waitFor(() => {
      expect(screen.getByText("Don't have an account?")).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(screen.getByText('Forgot password?')).toBeInTheDocument();
    });
  });
});
