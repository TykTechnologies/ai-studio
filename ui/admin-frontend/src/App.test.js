import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';

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

// Mock the config module to resolve immediately
jest.mock('./config', () => ({
  loadConfig: jest.fn().mockResolvedValue({
    API_BASE_URL: 'http://localhost:3000',
    features: {
      docs_url: 'http://docs.example.com',
      feature_chat: true,
      feature_portal: true
    }
  }),
  getConfig: jest.fn().mockReturnValue({
    API_BASE_URL: 'http://localhost:3000',
    features: {
      docs_url: 'http://docs.example.com',
      feature_chat: true,
      feature_portal: true
    }
  })
}));

// Mock pubClient - keep pending to show loading state
jest.mock('./admin/utils/pubClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn().mockImplementation(() => new Promise(() => {})),
    post: jest.fn()
  },
  reinitializePubClient: jest.fn()
}));

// Mock apiClient
jest.mock('./admin/utils/apiClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn()
  },
  reinitializeApiClient: jest.fn()
}));

// Mock system features hook
jest.mock('./admin/hooks/useSystemFeatures', () => ({
  __esModule: true,
  default: () => ({
    features: {
      docs_url: 'http://docs.example.com',
      feature_chat: true,
      feature_portal: true
    },
    loading: false,
    error: null,
    fetchFeatures: jest.fn()
  })
}));

// Mock react-router-dom
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => jest.fn(),
  BrowserRouter: ({ children }) => children
}));

// Import App after mocks are set up
import App from './App';

// Suppress console output
beforeAll(() => {
  jest.spyOn(console, 'error').mockImplementation(() => {});
  jest.spyOn(console, 'log').mockImplementation(() => {});
});

afterAll(() => {
  console.error.mockRestore?.();
  console.log.mockRestore?.();
});

const renderWithRouter = (ui, { route = '/' } = {}) => {
  return render(
    <MemoryRouter initialEntries={[route]}>
      {ui}
    </MemoryRouter>
  );
};

describe('App Component', () => {
  test('shows loading spinner initially while checking authentication', () => {
    renderWithRouter(<App />);
    // App shows loading spinner while waiting for auth check
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });
});
