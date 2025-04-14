import React, { useRef } from 'react';
import { QuickStartModal, useQuickStart as useQuickStartContext } from './index';
import WelcomeStep from '../WelcomeStep';
import ConfigureAIStep from '../ConfigureAIStep';
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
          validate: () => true // Validation is handled within the component
        },
        {
          id: "assign-owner",
          label: "Assign owner",
          content: null // Will be implemented later
        },
        {
          id: "app-details",
          label: "App details",
          content: null // Will be implemented later
        },
        {
          id: "summary",
          label: "Summary & credentials",
          content: null // Will be implemented later
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