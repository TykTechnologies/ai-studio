import React from 'react';
import { 
  StyledDialog, 
  StyledDialogContent 
} from './styles';
import { QuickStartProvider, useQuickStart } from './QuickStartContext';
import QuickStartStepProgress from './QuickStartStepProgress';

const StepContent = () => {
  const { steps, activeStep } = useQuickStart();
  
  return (
    <>
      {steps.map((step, index) => (
        <div key={step.id} style={{ display: index === activeStep ? 'block' : 'none' }}>
          {step.content}
        </div>
      ))}
    </>
  );
};

const QuickStartModal = ({
  open,
  onClose,
  steps,
  onComplete,
  onSkip,
  initialStep = 0,
  renderBeforeContent
}) => {
  const handleComplete = () => {
    onComplete && onComplete();
    onClose && onClose();
  };

  const handleSkip = () => {
    onSkip && onSkip();
    onClose && onClose();
  };
  
  return (
    <StyledDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <StyledDialogContent>
        <QuickStartProvider
          steps={steps}
          initialStep={initialStep}
          onComplete={handleComplete}
          onSkip={handleSkip}
        >
          {renderBeforeContent && renderBeforeContent()}
          <QuickStartStepProgress />
          <StepContent />
        </QuickStartProvider>
      </StyledDialogContent>
    </StyledDialog>
  );
};

export default QuickStartModal;