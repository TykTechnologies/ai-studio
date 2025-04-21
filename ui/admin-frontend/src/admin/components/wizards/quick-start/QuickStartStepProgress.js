import React from 'react';
import { Typography, Box } from '@mui/material';
import { useQuickStart } from './QuickStartContext';
import Icon from '../../../../components/common/Icon';
import {
  StepProgressContainer,
  StepProgressConnector,
  StepsContainer,
  StepContainer,
  StepNumber
} from './styles';

const QuickStartStepProgress = () => {
  const { steps, activeStep, isLastStep } = useQuickStart();

  if (isLastStep) return null;

  const progressSteps = steps.filter(step => !step.isWelcomeStep && !step.isLastStep);
  const currentStepIndex = activeStep === 0 ? -1 : activeStep - 1;
  
  // Determine if a step is completed (any step before the current step)
  const isStepCompleted = (index) => {
    return index < currentStepIndex;
  };
  
  // Calculate the width for the active step indicator (current step only)
  const activeStepWidth = currentStepIndex >= 0 ? 
    `calc(${100 / progressSteps.length}%)` : '0';
    
  // Calculate the width for completed steps indicator
  const completedStepsWidth = currentStepIndex > 0 ? 
    `calc(${currentStepIndex * (100 / progressSteps.length)}%)` : '0';

  return (
    <StepProgressContainer>
      <StepProgressConnector />
      
      {/* Completed steps indicator with rgba(43, 168, 74, 0.5) color */}
      {currentStepIndex > 0 && (
        <Box
          sx={{
            position: 'absolute',
            bottom: 0,
            left: 0,
            width: completedStepsWidth,
            height: '1.6px',
            backgroundColor: 'rgba(43, 168, 74, 0.5)',
            zIndex: 1,
          }}
        />
      )}
      
      {/* Active step indicator with iconSuccessDefault color */}
      {currentStepIndex >= 0 && (
        <Box
          sx={{
            position: 'absolute',
            bottom: 0,
            left: completedStepsWidth,
            width: activeStepWidth,
            height: '1.6px',
            backgroundColor: theme => theme.palette.background.iconSuccessDefault,
            zIndex: 1,
          }}
        />
      )}
      
      <StepsContainer>
        {progressSteps.map((step, index) => {
          const isActive = index === currentStepIndex;
          const completed = isStepCompleted(index);
          const stepWidth = `${100 / progressSteps.length}%`;
          
          return (
            <StepContainer key={step.id} width={stepWidth}>
              {completed ? (
                <Icon 
                  name="circle-check" 
                  sx={{ 
                    width: 24, 
                    height: 24, 
                    color: theme => theme.palette.background.iconSuccessDefault,
                    marginRight: theme => theme.spacing(1)
                  }} 
                />
              ) : (
                <StepNumber active={isActive} completed={completed}>
                  <Typography 
                    variant="bodyMediumMedium" 
                    color={isActive ? "text.primary" : "text.defaultSubdued"}
                  >
                    {index + 1}
                  </Typography>
                </StepNumber>
              )}
              <Typography 
                variant={isActive ? "bodyLargeBold" : "bodyLargeDefault"}
                color={isActive ? "text.primary" : "text.neutralDisabled"}
              >
                {step.label}
              </Typography>
            </StepContainer>
          );
        })}
      </StepsContainer>
    </StepProgressContainer>
  );
};

export default QuickStartStepProgress;