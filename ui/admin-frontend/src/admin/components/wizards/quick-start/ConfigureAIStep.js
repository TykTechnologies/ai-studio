import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Divider,
  CircularProgress,
  Snackbar,
  Alert
} from '@mui/material';
import { StyledTextField } from '../../../styles/sharedStyles';
import { useQuickStart } from './QuickStartContext';
import apiClient from '../../../utils/apiClient';
import { ActionsContainer } from './styles';
import { PrimaryButton, SecondaryLinkButton } from '../../../styles/sharedStyles';
import CustomNote from '../../common/CustomNote';
import CustomSelect from '../../common/CustomSelect';
import CustomSelectBadge from '../../common/CustomSelectBadge';
import Icon from '../../../../components/common/Icon';

const PRIVACY_LEVEL_SCORES = {
  public: 25,
  internal: 50,
  confidential: 75,
  restricted: 100
};

const ConfigureAIStep = () => {
  const {
    setStepValid,
    goToNextStep,
    goToPreviousStep,
    llmData,
    setLlmData,
    createdLlmId,
    setCreatedLlmId
  } = useQuickStart();
  
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
  const [loading, setLoading] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success'
  });

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
  
  useEffect(() => {
    if (llmData && Object.keys(llmData).length > 0) {
      setFormData(llmData);
    }
  }, [llmData]);

  const validateForm = () => {
    const newErrors = {};
    if (!formData.name.trim()) newErrors.name = "Name is required";
    if (!formData.llmProvider.trim()) newErrors.llmProvider = "LLM Provider is required";
    
    setErrors(newErrors);
    const isValid = Object.keys(newErrors).length === 0;
    
    return isValid;
  };
  
  const createOrUpdateLLM = async () => {
    setLoading(true);
    
    try {
      const llmPayload = {
        data: {
          type: "LLM",
          attributes: {
            name: formData.name,
            api_key: formData.apiKey,
            api_endpoint: formData.apiEndpoint,
            privacy_score: PRIVACY_LEVEL_SCORES[formData.privacyLevel],
            short_description: "",
            long_description: "",
            logo_url: "",
            vendor: formData.llmProvider,
            active: true,
            filters: [],
            default_model: "",
            allowed_models: []
          }
        }
      };
      
      let response;
      
      if (createdLlmId) {
        if (JSON.stringify(formData) !== JSON.stringify(llmData)) {
          response = await apiClient.patch(`/llms/${createdLlmId}`, llmPayload);
          setSnackbar({
            open: true,
            message: 'LLM updated successfully',
            severity: 'success'
          });
        }
      } else {
        response = await apiClient.post('/llms', llmPayload);
        const newLlmId = response.data.data.id;
        setCreatedLlmId(newLlmId);
        setSnackbar({
          open: true,
          message: 'LLM created successfully',
          severity: 'success'
        });
      }
      
      setLlmData(formData);
      goToNextStep();
    } catch (error) {
      console.error('Error creating/updating LLM:', error);
      setSnackbar({
        open: true,
        message: `Failed to ${createdLlmId ? 'update' : 'create'} LLM: ${error.message || 'Unknown error'}`,
        severity: 'error'
      });
    } finally {
      setLoading(false);
    }
  };
  
  const handleNextClick = () => {
    if (validateForm()) {
      createOrUpdateLLM();
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

  const privacyBadgeConfigs = {
    public: {
      icon: 'unlock',
      text: 'Public',
      textColor: 'text.successDefault',
      bgColor: 'border.successDefaultSubdued'
    },
    internal: {
      icon: 'lock',
      text: 'Internal',
      textColor: 'text.warningDefault',
      bgColor: 'border.warningDefaultSubdued'
    },
    confidential: {
      icon: 'lock-keyhole',
      text: 'Confidential',
      textColor: 'border.criticalHover',
      bgColor: 'border.criticalDefaultSubdue'
    },
    restricted: {
      icon: 'shield-keyhole',
      text: 'Restricted',
      textColor: 'background.surfaceCriticalDefault',
      bgColor: 'background.buttonPrimaryDefault'
    }
  };

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
                  <CustomSelectBadge config={privacyBadgeConfigs[option.value]} />
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
          disabled={!isFormValid || loading}
        >
          {loading ? <CircularProgress size={24} color="inherit" /> : 'Continue'}
        </PrimaryButton>
      </ActionsContainer>
      
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={() => setSnackbar(prev => ({ ...prev, open: false }))}
          severity={snackbar.severity}
          sx={{ width: '100%' }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default ConfigureAIStep;