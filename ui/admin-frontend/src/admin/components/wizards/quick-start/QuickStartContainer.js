import React, { useRef } from 'react';
import QuickStartModal from './QuickStartModal';
import { useQuickStart as useQuickStartContext } from './QuickStartContext';
import WelcomeStep from './WelcomeStep';
import ConfigureAIStep from './ConfigureAIStep';
import AssignOwnerStep from './AssignOwnerStep';
import AppDetailsStep from './AppDetailsStep';
import SummaryStep from './SummaryStep';
import useQuickStart from '../../../hooks/useQuickStart';

const QuickStartContainer = () => {
  const {
    showQuickStart,
    setShowQuickStart,
    userName,
    handleQuickStartComplete,
    handleQuickStartSkip
  } = useQuickStart();
  
  // Create a ref to store the goToNextStep function from the QuickStartContext
  const quickStartContextRef = useRef(null);

  // This component will capture the QuickStartContext value
  const ContextCapture = () => {
    const contextValue = useQuickStartContext();
    
    // Store the context value in the ref
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
          content: <WelcomeStep userName={userName} />,
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
          content: <AssignOwnerStep />,
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