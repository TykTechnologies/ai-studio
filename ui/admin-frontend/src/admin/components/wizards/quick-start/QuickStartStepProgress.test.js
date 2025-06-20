import React from 'react';
import { render, screen } from '@testing-library/react';
import { ThemeProvider } from '@mui/material/styles';
import QuickStartStepProgress from './QuickStartStepProgress';
import theme from '../../../theme';

// Mock useQuickStart and Icon
jest.mock('./QuickStartContext', () => ({
  useQuickStart: jest.fn(),
}));
jest.mock('../../../../components/common/Icon', () => () => <div data-testid="icon" />);

const { useQuickStart } = require('./QuickStartContext');

const renderWithTheme = (ui) =>
  render(<ThemeProvider theme={theme}>{ui}</ThemeProvider>);

describe('QuickStartStepProgress', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders no progress if isLastStep is true', () => {
    useQuickStart.mockReturnValue({ steps: [], activeStep: 0, isLastStep: true });
    renderWithTheme(<QuickStartStepProgress />);
    expect(screen.queryByTestId('step-progress-container')).not.toBeInTheDocument();
  });

  it('renders correct number of steps (excluding welcome/last)', () => {
    useQuickStart.mockReturnValue({
      steps: [
        { id: 'welcome', isWelcomeStep: true },
        { id: 'one' },
        { id: 'two' },
        { id: 'summary', isLastStep: true },
      ],
      activeStep: 1,
      isLastStep: false,
    });
    renderWithTheme(<QuickStartStepProgress />);
    // Should render step numbers 1 and 2
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('shows completed and active step indicators', () => {
    useQuickStart.mockReturnValue({
      steps: [
        { id: 'welcome', isWelcomeStep: true },
        { id: 'one' },
        { id: 'two' },
        { id: 'summary', isLastStep: true },
      ],
      activeStep: 2,
      isLastStep: false,
    });
    renderWithTheme(<QuickStartStepProgress />);
    // Completed step: first step (index 0)
    // Active step: second step (index 1)
    // Should render icon for active step
    expect(screen.getAllByTestId('icon')).toHaveLength(1);
  });

  it('renders step numbers for incomplete steps', () => {
    useQuickStart.mockReturnValue({
      steps: [
        { id: 'welcome', isWelcomeStep: true },
        { id: 'one' },
        { id: 'two' },
        { id: 'summary', isLastStep: true },
      ],
      activeStep: 1,
      isLastStep: false,
    });
    renderWithTheme(<QuickStartStepProgress />);
    // Should show step number 1 for the first step
    expect(screen.getByText('1')).toBeInTheDocument();
  });
});
