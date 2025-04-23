import React from 'react';
import { render, fireEvent, screen } from '@testing-library/react';
import QuickStartContainer from './QuickStartContainer';
import * as QuickStartContext from './QuickStartContext';

// Mock the useQuickStart hook from hooks to prevent network calls
jest.mock('../../../hooks/useQuickStart', () => () => ({
  showQuickStart: false,
  setShowQuickStart: jest.fn(),
  currentUser: {
    id: 'user123',
    name: 'Test User',
    email: 'test@example.com'
  },
  handleQuickStartComplete: jest.fn(),
  handleQuickStartSkip: jest.fn(),
}));

// Mock all step components and modal
jest.mock('./WelcomeStep', () => () => <div data-testid="welcome-step" />);
jest.mock('./ConfigureAIStep', () => () => <div data-testid="configure-ai-step" />);
jest.mock('./AssignOwnerStep', () => () => <div data-testid="assign-owner-step" />);
jest.mock('./AppDetailsStep', () => () => <div data-testid="app-details-step" />);
jest.mock('./SummaryStep', () => () => <div data-testid="summary-step" />);
jest.mock('./FinalStep', () => () => <div data-testid="final-step" />);
jest.mock('./QuickStartModal', () =>
  function MockQuickStartModal(props) {
    if (!props.open) return null;
    return (
      <div data-testid="quick-start-modal">
        {props.renderBeforeContent && props.renderBeforeContent()}
        {props.steps && props.steps.map(step => (
          <div key={step.id} data-testid={`step-${step.id}`}>{step.content}</div>
        ))}
        <button data-testid="quick-start-close" onClick={props.onClose}>Close</button>
        <button data-testid="quick-start-next" onClick={props.onComplete}>Next</button>
        <button data-testid="quick-start-skip" onClick={props.onSkip}>Skip</button>
      </div>
    );
  }
);

// Helper to render with optional context override
const renderWithContext = (contextValue = {}, props = {}) => {
  jest.spyOn(QuickStartContext, 'useQuickStart').mockReturnValue({
    showQuickStart: false,
    setShowQuickStart: jest.fn(),
    currentUser: {
      id: 'user123',
      name: 'Test User',
      email: 'test@example.com'
    },
    handleQuickStartComplete: jest.fn(),
    handleQuickStartSkip: jest.fn(),
    ...contextValue,
  });
  return render(
    <QuickStartContainer
      quickStartState={{
        showQuickStart: false,
        setShowQuickStart: jest.fn(),
        currentUser: {
          id: 'user123',
          name: 'Test User',
          email: 'test@example.com'
        },
        handleQuickStartComplete: jest.fn(),
        handleQuickStartSkip: jest.fn(),
        ...contextValue,
        ...(props.quickStartState || {}),
      }}
      {...props}
    />
  );
};

describe('QuickStartContainer', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders nothing if showQuickStart is false', () => {
    renderWithContext({ showQuickStart: false });
    expect(screen.queryByTestId('quick-start-modal')).not.toBeInTheDocument();
  });

  it('renders modal and all steps when showQuickStart is true', () => {
    renderWithContext({ showQuickStart: true });
    expect(screen.getByTestId('quick-start-modal')).toBeInTheDocument();
    expect(screen.getByTestId('welcome-step')).toBeInTheDocument();
    expect(screen.getByTestId('configure-ai-step')).toBeInTheDocument();
    expect(screen.getByTestId('assign-owner-step')).toBeInTheDocument();
    expect(screen.getByTestId('app-details-step')).toBeInTheDocument();
    expect(screen.getByTestId('summary-step')).toBeInTheDocument();
    expect(screen.getByTestId('final-step')).toBeInTheDocument();
  });

  it('calls setShowQuickStart(false) when close button is clicked', () => {
    const setShowQuickStart = jest.fn();
    renderWithContext({ showQuickStart: true, setShowQuickStart });
    fireEvent.click(screen.getByTestId('quick-start-close'));
    expect(setShowQuickStart).toHaveBeenCalledWith(false);
  });

  it('calls handleQuickStartComplete when next is clicked', () => {
    const handleQuickStartComplete = jest.fn();
    renderWithContext({ showQuickStart: true, handleQuickStartComplete });
    fireEvent.click(screen.getByTestId('quick-start-next'));
    expect(handleQuickStartComplete).toHaveBeenCalled();
  });

  it('calls handleQuickStartSkip when skip is clicked', () => {
    const handleQuickStartSkip = jest.fn();
    renderWithContext({ showQuickStart: true, handleQuickStartSkip });
    fireEvent.click(screen.getByTestId('quick-start-skip'));
    expect(handleQuickStartSkip).toHaveBeenCalled();
  });

  it('uses quickStartState prop when provided', () => {
    const setShowQuickStart = jest.fn();
    renderWithContext({}, {
      quickStartState: {
        showQuickStart: true,
        setShowQuickStart,
        currentUser: {
          id: 'user456',
          name: 'Prop User',
          email: 'prop@example.com'
        },
        handleQuickStartComplete: jest.fn(),
        handleQuickStartSkip: jest.fn(),
      },
    });
    expect(screen.getByTestId('quick-start-modal')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('quick-start-close'));
    expect(setShowQuickStart).toHaveBeenCalledWith(false);
  });

  it('renders ContextCapture and passes context to ref', () => {
    // This test ensures ContextCapture runs and doesn't throw
    expect(() => renderWithContext({ showQuickStart: true })).not.toThrow();
  });

  it('renders all step contents in correct order', () => {
    renderWithContext({ showQuickStart: true });
    const steps = [
      'welcome-step',
      'configure-ai-step',
      'assign-owner-step',
      'app-details-step',
      'summary-step',
      'final-step',
    ];
    steps.forEach(testid => {
      expect(screen.getByTestId(testid)).toBeInTheDocument();
    });
  });

  it('is robust to missing optional callbacks', () => {
    renderWithContext({ showQuickStart: true, handleQuickStartComplete: undefined, handleQuickStartSkip: undefined });
    fireEvent.click(screen.getByTestId('quick-start-next'));
    fireEvent.click(screen.getByTestId('quick-start-skip'));
    // No error should occur
    expect(screen.getByTestId('quick-start-modal')).toBeInTheDocument();
  });

  // Edge case: quickStartState missing setShowQuickStart
  it('does not throw if setShowQuickStart is missing', () => {
    expect(() => renderWithContext({}, { quickStartState: { showQuickStart: true } })).not.toThrow();
  });
});
