import React, { useRef } from 'react';
import QuickStartModal from './QuickStartModal';
import { useQuickStart as useQuickStartContext } from './QuickStartContext';
import WelcomeStep from './WelcomeStep';
import ConfigureAIStep from './ConfigureAIStep';
import AssignOwnerStep from './AssignOwnerStep';
import AppDetailsStep from './AppDetailsStep';
import SummaryStep from './SummaryStep';
import FinalStep from './FinalStep';
import useQuickStart from '../../../hooks/useQuickStart';

const QuickStartContainer = ({ quickStartState }) => {
  const hookState = useQuickStart();

  const {
    showQuickStart,
    setShowQuickStart,
    currentUser,
    handleQuickStartComplete,
    handleQuickStartSkip
  } = quickStartState || hookState;
  
  const quickStartContextRef = useRef(null);

  const ContextCapture = () => {
    const contextValue = useQuickStartContext();
    quickStartContextRef.current = contextValue;
    
    return null;
  };

  return (
    <QuickStartModal
      open={showQuickStart}
      onClose={() => setShowQuickStart(false)}
      renderBeforeContent={() => <ContextCapture />}
      steps={[
        {
          id: "welcome",
          label: "Welcome",
          content: <WelcomeStep userName={currentUser?.name} />,
          isWelcomeStep: true
        },
        {
          id: "configure-ai",
          label: "Configure AI",
          content: <ConfigureAIStep />,
          validate: () => true
        },
        {
          id: "assign-owner",
          label: "Assign owner",
          content: <AssignOwnerStep currentUser={currentUser} />,
          validate: () => true
        },
        {
          id: "app-details",
          label: "App details",
          content: <AppDetailsStep />,
          validate: () => true
        },
        {
          id: "summary",
          label: "Summary & credentials",
          content: <SummaryStep />
        },
        {
          id: "finish",
          label: "Finish",
          content: <FinalStep />,
          isLastStep: true,
        }
      ]}
      onComplete={handleQuickStartComplete}
      onSkip={handleQuickStartSkip}
      primaryActionLabels={{ next: "Quick start", complete: "Finish" }}
      skipLabel="Explore by myself"
    />
  );
};

export default QuickStartContainer;