import React from 'react';
import { screen, fireEvent, waitFor } from '@testing-library/react';
import { renderWithRouterAndTheme, renderWithRoutesAndTheme } from '../../../../test-utils/render-with-theme';
import { teamsService } from '../../../services/teamsService';
import ManageTeamsSection from './ManageTeamsSection';

jest.mock('../../../services/teamsService', () => ({
  teamsService: {
    getTeams: jest.fn()
  }
}));

jest.mock('../../common/CollapsibleSection', () => ({ children, title, defaultExpanded }) =>
  <div data-testid="collapsible-section" data-title={title} data-default-expanded={defaultExpanded?.toString()}>
    {children}
  </div>
);

jest.mock('../../../../portal/styles/authStyles', () => ({
  StyledCheckbox: ({ checked, onChange, label }) => (
    <label>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        data-testid={`checkbox-${label}`}
      />
      {label}
    </label>
  )
}));

jest.mock('../../common/CustomNote', () => ({ message }) =>
  <div data-testid="custom-note">{message}</div>
);

const mockTeams = [
  { id: '1', attributes: { name: 'Default Team' } },
  { id: '2', attributes: { name: 'Development Team' } },
  { id: '3', attributes: { name: 'QA Team' } }
];

describe('ManageTeamsSection', () => {
  let mockSetSelectedTeams;

  beforeEach(() => {
    mockSetSelectedTeams = jest.fn();
    jest.clearAllMocks();
  });

  const renderComponent = (props = {}, route = '/', hasIdParam = false) => {
    if (hasIdParam) {
      return renderWithRoutesAndTheme(
        <ManageTeamsSection
          selectedTeams={[]}
          setSelectedTeams={mockSetSelectedTeams}
          {...props}
        />,
        { 
          routes: [{ path: '/users/:id', element: <ManageTeamsSection selectedTeams={[]} setSelectedTeams={mockSetSelectedTeams} {...props} /> }],
          initialEntry: '/users/123'
        }
      );
    }
    return renderWithRouterAndTheme(
      <ManageTeamsSection
        selectedTeams={[]}
        setSelectedTeams={mockSetSelectedTeams}
        {...props}
      />,
      { route }
    );
  };

  it('renders with correct title and default collapsed state', () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent();

    const section = screen.getByTestId('collapsible-section');
    expect(section).toBeInTheDocument();
    expect(section).toHaveAttribute('data-title', 'Manage Teams');
    expect(section).toHaveAttribute('data-default-expanded', 'false');
  });

  it('renders expanded when id param is present', () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent({}, '/', true);

    const section = screen.getByTestId('collapsible-section');
    expect(section).toHaveAttribute('data-default-expanded', 'true');
  });

  it('displays loading spinner while fetching teams', () => {
    teamsService.getTeams.mockImplementation(() => new Promise(() => {}));
    
    renderComponent();

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('displays teams with checkboxes when teams are available', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/Teams help organize users and manage access/)).toBeInTheDocument();
    expect(screen.getByText('Select the teams this user should be part of.')).toBeInTheDocument();
    
    mockTeams.forEach(team => {
      expect(screen.getByTestId(`checkbox-${team.attributes.name}`)).toBeInTheDocument();
    });
  });

  it('displays custom note when only default team exists', async () => {
    const defaultTeamOnly = [{ id: '1', attributes: { name: 'Default Team' } }];
    teamsService.getTeams.mockResolvedValue({ data: { data: defaultTeamOnly } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('custom-note')).toBeInTheDocument();
    expect(screen.getByText(/All users are automatically assigned to the default team/)).toBeInTheDocument();
  });

  it('displays custom note when no teams exist', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: [] } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('custom-note')).toBeInTheDocument();
  });

  it('displays teams when multiple teams exist including default', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('custom-note')).not.toBeInTheDocument();
    expect(screen.getByTestId('checkbox-Default Team')).toBeInTheDocument();
    expect(screen.getByTestId('checkbox-Development Team')).toBeInTheDocument();
    expect(screen.getByTestId('checkbox-QA Team')).toBeInTheDocument();
  });

  it('displays teams when only non-default teams exist', async () => {
    const nonDefaultTeams = [
      { id: '2', attributes: { name: 'Development Team' } },
      { id: '3', attributes: { name: 'QA Team' } }
    ];
    teamsService.getTeams.mockResolvedValue({ data: { data: nonDefaultTeams } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('custom-note')).not.toBeInTheDocument();
    expect(screen.getByTestId('checkbox-Development Team')).toBeInTheDocument();
    expect(screen.getByTestId('checkbox-QA Team')).toBeInTheDocument();
  });

  it('handles checking a team checkbox', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    const checkbox = screen.getByTestId('checkbox-Development Team');
    fireEvent.click(checkbox);

    expect(mockSetSelectedTeams).toHaveBeenCalledWith(expect.any(Function));
    
    const updateFunction = mockSetSelectedTeams.mock.calls[0][0];
    expect(updateFunction([])).toEqual([2]);
  });

  it('handles unchecking a team checkbox', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent({ selectedTeams: [2, 3] });

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    const checkbox = screen.getByTestId('checkbox-Development Team');
    expect(checkbox).toBeChecked();
    
    fireEvent.click(checkbox);

    expect(mockSetSelectedTeams).toHaveBeenCalledWith(expect.any(Function));
    
    const updateFunction = mockSetSelectedTeams.mock.calls[0][0];
    expect(updateFunction([2, 3])).toEqual([3]);
  });

  it('displays checked state for selected teams', async () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent({ selectedTeams: [1, 3] });

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('checkbox-Default Team')).toBeChecked();
    expect(screen.getByTestId('checkbox-Development Team')).not.toBeChecked();
    expect(screen.getByTestId('checkbox-QA Team')).toBeChecked();
  });

  it('handles API error gracefully', async () => {
    const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    teamsService.getTeams.mockRejectedValue(new Error('API Error'));
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    expect(consoleSpy).toHaveBeenCalledWith('Error fetching teams:', expect.any(Error));
    expect(screen.getByTestId('custom-note')).toBeInTheDocument();
    
    consoleSpy.mockRestore();
  });

  it('calls teamsService.getTeams with correct parameters', () => {
    teamsService.getTeams.mockResolvedValue({ data: { data: mockTeams } });
    
    renderComponent();

    expect(teamsService.getTeams).toHaveBeenCalledWith({ all: true });
  });

  it('handles teams with string IDs correctly', async () => {
    const teamsWithStringIds = [
      { id: '10', attributes: { name: 'String ID Team' } },
      { id: '20', attributes: { name: 'Another String ID Team' } }
    ];
    teamsService.getTeams.mockResolvedValue({ data: { data: teamsWithStringIds } });
    
    renderComponent();

    await waitFor(() => {
      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    const checkbox = screen.getByTestId('checkbox-String ID Team');
    fireEvent.click(checkbox);

    expect(mockSetSelectedTeams).toHaveBeenCalledWith(expect.any(Function));
    
    const updateFunction = mockSetSelectedTeams.mock.calls[0][0];
    expect(updateFunction([])).toEqual([10]);
  });
}); 