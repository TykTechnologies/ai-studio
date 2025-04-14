import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Divider,
} from '@mui/material';
import { StyledTextField } from '../../styles/sharedStyles';
import { useQuickStart } from './quick-start/QuickStartContext';
import { ActionsContainer } from './quick-start/styles';
import { PrimaryButton, SecondaryLinkButton } from '../../styles/sharedStyles';
import CustomNote from '../common/CustomNote';
import CustomSelect from '../common/CustomSelect';
import PrivacyLevelBadge from '../common/PrivacyLevelBadge';
import Icon from '../../../components/common/Icon';

const ConfigureAIStep = () => {
  const { setStepValid, goToNextStep, goToPreviousStep } = useQuickStart();
  const [formData, setFormData] = useState({
    name: '',
    llmProvider: '',
    apiEndpoint: '',
    apiKey: '',
    privacyLevel: 'public'
  });
  const [errors, setErrors] = useState({});
  const [vendors, setVendors] = useState([]);
  const [isFormValid, setIsFormValid] = useState(false);

  const checkRequiredFields = React.useCallback(() => {
    const isValid = formData.name.trim() !== '' && formData.llmProvider.trim() !== '';
    setIsFormValid(isValid);
    setStepValid('configure-ai', isValid);
    return isValid;
  }, [formData, setStepValid]);

  useEffect(() => {
    const vendorList = [
      { value: 'openai', label: 'OpenAI' },
      { value: 'anthropic', label: 'Anthropic' },
      { value: 'google', label: 'Google AI' },
      { value: 'azure', label: 'Azure OpenAI' },
      { value: 'cohere', label: 'Cohere' },
    ];
    setVendors(vendorList);
  }, []);
  
  useEffect(() => {
    checkRequiredFields();
  }, [formData, checkRequiredFields]);

  const validateForm = () => {
    const newErrors = {};
    if (!formData.name.trim()) newErrors.name = "Name is required";
    if (!formData.llmProvider.trim()) newErrors.llmProvider = "LLM Provider is required";
    
    setErrors(newErrors);
    const isValid = Object.keys(newErrors).length === 0;
    
    return isValid;
  };
  
  const handleNextClick = () => {
    if (validateForm()) {
      goToNextStep();
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
    // Form validation is now handled by the useEffect hook
  };

  const privacyLevelOptions = [
    { value: 'public', label: 'Public', description: 'Safe to share data (e.g. blogs, press releases)' },
    { value: 'internal', label: 'Internal', description: 'Limited to users within the org. (e.g. reports, policies)' },
    { value: 'confidential', label: 'Confidential', description: 'Sensitive data (e.g. financials, strategies)' },
    { value: 'restricted', label: 'Restricted', description: 'PII or personal data (e.g. names, emails, costumer info)' }
  ];

  return (
    <Box sx={{ width: '100%', pt: 2 }}>
      <Box sx={{ textAlign: 'center', mb: 4, px: 10 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Let's start building your AI infrastructure by setting up the Large Language Model provider you want your developers to access. Set a name for the LLM, select the LLM provider and specify the privacy level.
        </Typography>
      </Box>
      <Box sx={{ mt: 3, px: 25 }}>
        <Box sx={{ mb: 3 }}>
          <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
            Name*
          </Typography>
          <StyledTextField
            fullWidth
            name="name"
            value={formData.name}
            onChange={handleChange}
            error={!!errors.name}
            helperText={errors.name}
            required
            autoComplete="off"
          />
        </Box>

        <Box sx={{ mb: 3 }}>
          <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
            LLM Provider*
          </Typography>
          <CustomSelect
            name="llmProvider"
            value={formData.llmProvider}
            onChange={handleChange}
            options={vendors}
            error={!!errors.llmProvider}
            helperText={errors.llmProvider}
            required
          />
          <Box sx={{ display: 'flex', alignItems: 'center', mt: 1 }}>
            <Icon 
              name="circle-exclamation" 
              sx={{ 
                width: 14, 
                height: 14, 
                mr: 0.5,
                color: 'border.neutralPressed'
              }} 
            />
            <Typography variant="bodySmallDefault" color="text.defaultSubdued">
              For some providers you will need to enter the API endpoint and key.
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: 'flex', gap: 2, mb: 4 }}>
          <Box sx={{ flex: 1 }}>
            <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
              API Endpoint
            </Typography>
            <StyledTextField
              fullWidth
              name="apiEndpoint"
              value={formData.apiEndpoint}
              onChange={handleChange}
              autoComplete="off"
            />
          </Box>

          <Box sx={{ flex: 1 }}>
            <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
              API Key
            </Typography>
            <StyledTextField
              fullWidth
              name="apiKey"
              type="password"
              value={formData.apiKey}
              onChange={handleChange}
              autoComplete="off"
            />
          </Box>
        </Box>

        <Divider sx={{ borderColor: 'border.neutralDefault', mb: 3 }} />

        <Box sx={{ mb: 3, display: 'flex', flexDirection: 'column' }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Privacy Level
          </Typography>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Privacy levels control LLM access based on data sensitivity. Lower-level models can't access higher-security data or tools. Set the privacy level to limit the highest data sensitivity this model can access.
          </Typography>
          <CustomSelect
            name="privacyLevel"
            value={formData.privacyLevel}
            onChange={handleChange}
            options={privacyLevelOptions}
            renderOption={(option) => (
              <Box sx={{ display: 'flex', alignItems: 'center', width: '100%' }}>
                <Box sx={{ mr: 2 }}>
                  <PrivacyLevelBadge level={option.value} />
                </Box>
                <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                  {option.description}
                </Typography>
              </Box>
            )}
          />
        </Box>

        <CustomNote
          message="Later, you can add Data sources to enhance AI capabilities with more information and functionality."
        />
      </Box>
      
      <ActionsContainer sx={{ justifyContent: 'space-between'}}>
        <SecondaryLinkButton onClick={goToPreviousStep}>
          Skip quick start
        </SecondaryLinkButton>
        <PrimaryButton
          onClick={handleNextClick}
          disabled={!isFormValid}
        >
          Continue
        </PrimaryButton>
      </ActionsContainer>
    </Box>
  );
};

export default ConfigureAIStep;