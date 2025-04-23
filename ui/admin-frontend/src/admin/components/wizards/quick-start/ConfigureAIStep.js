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
import { getVendorCodes, getVendorName, getVendorLogo, vendorRequiresAccessDetails } from '../../../utils/vendorLogos';

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
    skipQuickStart,
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
  const [vendors, setVendors] = useState([]);
  const [isFormValid, setIsFormValid] = useState(false);
  const [loading, setLoading] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success'
  });

  const checkRequiredFields = React.useCallback(() => {
    const nameValid = formData.name.trim() !== '';
    const providerValid = formData.llmProvider.trim() !== '';
    
    const requiresAccessDetails = formData.llmProvider &&
      vendorRequiresAccessDetails(formData.llmProvider);
    
    const apiEndpointValid = !requiresAccessDetails || formData.apiEndpoint.trim() !== '';
    const apiKeyValid = !requiresAccessDetails || formData.apiKey.trim() !== '';
    
    const isValid = nameValid && providerValid && apiEndpointValid && apiKeyValid;
    
    setIsFormValid(isValid);
    setStepValid('configure-ai', isValid);
    return isValid;
  }, [formData, setStepValid]);

  useEffect(() => {
    const vendorCodes = getVendorCodes();
    const vendorList = vendorCodes.map(code => ({
      value: code,
      label: getVendorName(code)
    }));
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
          await apiClient.patch(`/llms/${createdLlmId}`, llmPayload);
        }
      } else {
        response = await apiClient.post('/llms', llmPayload);
        const newLlmId = response.data.data.id;
        setCreatedLlmId(newLlmId);
      }
      
      setLlmData(formData);
      goToNextStep();
    } catch (error) {
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
    createOrUpdateLLM();
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
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
      <Box sx={{
        textAlign: 'center',
        mb: 4,
        px: {
          xs: 2,
          sm: 4,
          md: 10
        }
      }}>
        <Typography
          variant="bodyLargeDefault"
          color="text.defaultSubdued"
          sx={{
            fontSize: {
              xs: '0.875rem',
              sm: 'inherit'
            }
          }}
        >
          Let's start building your AI infrastructure by setting up the Large Language Model provider you want your developers to access. Set a name for the LLM, select the LLM provider and specify the privacy level.
        </Typography>
      </Box>
      <Box sx={{
        mt: 3,
        px: {
          xs: 2,
          sm: 4,
          md: 10,
          lg: 25
        }
      }}>
        <Box sx={{ mb: 3 }}>
          <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
            Name*
          </Typography>
          <StyledTextField
            fullWidth
            name="name"
            value={formData.name}
            onChange={handleChange}
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
            required
            renderOption={(option) => (
              <Box sx={{ display: "flex", alignItems: "center" }}>
                <img
                  src={getVendorLogo(option.value)}
                  alt={option.label}
                  style={{
                    width: 24,
                    height: 24,
                    marginRight: 8,
                    objectFit: "contain",
                  }}
                  onError={(e) => {
                    e.target.onerror = null;
                    e.target.src = "/images/placeholder-logo.png";
                  }}
                />
                {option.label}
              </Box>
            )}
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

        <Box
          sx={{
            display: 'flex',
            flexDirection: { xs: 'column', sm: 'row' },
            gap: 2,
            mb: 4
          }}
          autoComplete="off"
          data-form-type="other"
        >
          <Box sx={{ flex: 1 }}>
            <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
              API Endpoint{formData.llmProvider && vendorRequiresAccessDetails(formData.llmProvider) ? '*' : ''}
            </Typography>
            <StyledTextField
              fullWidth
              name="api_endpoint_field"
              value={formData.apiEndpoint}
              onChange={(e) => {
                const { value } = e.target;
                setFormData(prev => ({
                  ...prev,
                  apiEndpoint: value
                }));
              }}
              autoComplete="new-password"
              inputProps={{
                autoComplete: "new-password",
                "data-form-type": "other",
                "data-lpignore": "true"
              }}
            />
          </Box>

          <Box sx={{ flex: 1 }}>
            <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
              API Key{formData.llmProvider && vendorRequiresAccessDetails(formData.llmProvider) ? '*' : ''}
            </Typography>
            <StyledTextField
              fullWidth
              name="api_key_field"
              type="password"
              value={formData.apiKey}
              onChange={(e) => {
                const { value } = e.target;
                setFormData(prev => ({
                  ...prev,
                  apiKey: value
                }));
              }}
              autoComplete="new-password"
              inputProps={{
                autoComplete: "new-password",
                "data-form-type": "other",
                "data-lpignore": "true"
              }}
            />
          </Box>
        </Box>

        <Divider sx={{ borderColor: 'border.neutralDefault', mb: 3 }} />

        <Box sx={{ mb: 3, display: 'flex', flexDirection: 'column' }}>
          <Typography variant="bodyLargeBold" color="text.primary">
            Privacy Level
          </Typography>
          <Typography
            variant="bodyMediumDefault"
            color="text.defaultSubdued"
            sx={{
              fontSize: {
                xs: '0.75rem',
                sm: 'inherit'
              }
            }}
          >
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
                <Typography
                  variant="bodyLargeDefault"
                  color="text.defaultSubdued"
                  sx={{
                    fontSize: {
                      xs: '0.75rem',
                      sm: 'inherit'
                    }
                  }}
                >
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
      
      <ActionsContainer sx={{
        justifyContent: 'space-between',
        flexDirection: { xs: 'column', sm: 'row' },
        gap: { xs: 2, sm: 0 },
        width: '100%',
        padding: { xs: 2, sm: 0 },
        alignItems: 'center'
      }}>
        <SecondaryLinkButton
          onClick={skipQuickStart}
        >
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