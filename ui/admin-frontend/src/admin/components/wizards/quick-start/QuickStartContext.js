import React, { createContext, useContext, useState, useCallback, useEffect } from 'react';
import { getAllLLMs } from '../../../services/llmService';

const QuickStartContext = createContext();

export const QuickStartProvider = ({
  children,
  steps,
  initialStep = 0,
  onComplete,
  onSkip,
  currentUser = null,
  showLicenseBanner = false,
  licenseDaysLeft = null
}) => {
  const [activeStep, setActiveStep] = useState(initialStep);
  const [stepValidation, setStepValidation] = useState({});
  const [llmData, setLlmData] = useState({});
  const [createdLlmId, setCreatedLlmId] = useState(null);
  const [ownerData, setOwnerData] = useState({});
  const [createdOwnerId, setCreatedOwnerId] = useState(null);
  const [appData, setAppData] = useState({});
  const [credentialData, setCredentialData] = useState({});
  const [createdAppId, setCreatedAppId] = useState(null);
  const [availableLLMs, setAvailableLLMs] = useState([]);
  
  useEffect(() => {
    getAllLLMs()
    .then((llms) => setAvailableLLMs(llms))
    .catch(error => console.error('Error fetching LLMs', error));
  }, []);

  const validateStep = useCallback((stepId) => {
    const step = steps.find(s => s.id === stepId);
    if (!step || !step.validate) return true;
    return step.validate();
  }, [steps]);

  const goToNextStep = useCallback(() => {
    const currentStepId = steps[activeStep].id;
    if (!validateStep(currentStepId)) return;

    if (activeStep < steps.length - 1) {
      setActiveStep(prev => prev + 1);
    } else {
      onComplete?.();
    }
  }, [activeStep, steps, validateStep, onComplete]);

  const goToPreviousStep = useCallback(() => {
    if (activeStep > 0) {
      setActiveStep(prev => prev - 1);
    }
  }, [activeStep]);

  const skipQuickStart = useCallback(() => {
    onSkip?.();
  }, [onSkip]);

  const setStepValid = useCallback((stepId, isValid) => {
    setStepValidation(prev => ({
      ...prev,
      [stepId]: isValid
    }));
  }, []);

  const value = {
    activeStep,
    steps,
    stepValidation,
    goToNextStep,
    goToPreviousStep,
    skipQuickStart,
    validateStep,
    setStepValid,
    isFirstStep: activeStep === 0,
    isLastStep: activeStep === steps.length - 1,
    currentStep: steps[activeStep],
    llmData,
    setLlmData,
    createdLlmId,
    setCreatedLlmId,
    ownerData,
    setOwnerData,
    createdOwnerId,
    setCreatedOwnerId,
    appData,
    credentialData,
    setCredentialData,
    setAppData,
    createdAppId,
    setCreatedAppId,
    currentUser,
    availableLLMs,
    showLicenseBanner,
    licenseDaysLeft,
  };

  return (
    <QuickStartContext.Provider value={value}>
      {children}
    </QuickStartContext.Provider>
  );
};

export const useQuickStart = () => {
  const context = useContext(QuickStartContext);
  if (!context) {
    throw new Error('useQuickStart must be used within a QuickStartProvider');
  }
  return context;
};