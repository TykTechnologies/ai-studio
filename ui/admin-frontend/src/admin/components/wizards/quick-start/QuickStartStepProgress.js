import React from 'react';
import { Typography, Box, useMediaQuery } from '@mui/material';
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
  const isMobile = useMediaQuery('(max-width:600px)');
  const isTablet = useMediaQuery('(max-width:900px)');

  if (isLastStep) return null;

  const progressSteps = steps.filter(step => !step.isWelcomeStep && !step.isLastStep);
  const currentStepIndex = activeStep === 0 ? -1 : activeStep - 1;
  
  const isStepCompleted = (index) => {
    return index < currentStepIndex;
  };
  
  const activeStepWidth = currentStepIndex >= 0 ?
    `calc(${100 / progressSteps.length}%)` : '0';
    
  const completedStepsWidth = currentStepIndex > 0 ?
    `calc(${currentStepIndex * (100 / progressSteps.length)}%)` : '0';

  return (
    <StepProgressContainer>
      <StepProgressConnector />
      
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
                    width: isMobile ? 20 : isTablet ? 22 : 24,
                    height: isMobile ? 20 : isTablet ? 22 : 24,
                    color: theme => theme.palette.background.iconSuccessDefault,
                    marginRight: theme => theme.spacing(isMobile ? 0.5 : isTablet ? 0.75 : 1)
                  }}
                />
              ) : (
                <StepNumber active={isActive} completed={completed}>
                  <Typography
                    variant="bodyMediumMedium"
                    color={isActive ? "text.primary" : "text.defaultSubdued"}
                    sx={{
                      fontSize: isMobile ? '0.75rem' : isTablet ? '0.85rem' : 'inherit',
                      lineHeight: 1
                    }}
                  >
                    {index + 1}
                  </Typography>
                </StepNumber>
              )}
              <Typography
                variant={isActive ? "bodyLargeBold" : "bodyLargeDefault"}
                color={isActive ? "text.primary" : "text.neutralDisabled"}
                sx={{
                  fontSize: isMobile ? '0.75rem' : isTablet ? '0.85rem' : 'inherit',
                  lineHeight: 1.2
                }}
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