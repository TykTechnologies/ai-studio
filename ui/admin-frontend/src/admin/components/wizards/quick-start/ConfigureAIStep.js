import React, { useState, useEffect, memo, useCallback } from 'react';
import {
  Box,
  Typography,
  Divider,
  CircularProgress,
  Snackbar,
  Alert,
} from '@mui/material';
import { StyledTextField } from '../../../styles/sharedStyles';
import { useQuickStart } from './QuickStartContext';
import { createLLM, updateLLM } from '../../../services';
import { ActionsContainer } from './styles';
import { PrimaryButton, SecondaryLinkButton } from '../../../styles/sharedStyles';
import CustomNote from '../../common/CustomNote';
import CustomSelect from '../../common/CustomSelect';
import CustomSelectBadge from '../../common/CustomSelectBadge';
import Icon from '../../../../components/common/Icon';
import RadioSelectionGroup from '../../common/RadioSelectionGroup';
import { getVendorCodes, getVendorName, getVendorLogo, vendorRequiresAccessDetails } from '../../../utils/vendorLogos';
import { PRIVACY_LEVEL_SCORES, PRIVACY_LEVEL_OPTIONS, PRIVACY_BADGE_CONFIGS } from './utils';

const ConfigureAIStep = () => {
  const {
    setStepValid,
    goToNextStep,
    skipQuickStart,
    llmData,
    setLlmData,
    createdLlmId,
    setCreatedLlmId,
    availableLLMs
  } = useQuickStart();
  
  const [llmType, setLlmType] = useState('existing');
  const [selectedLlmId, setSelectedLlmId] = useState('');
  
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

  const checkRequiredFields = useCallback(() => {
    if (llmType === 'existing') {
      const isValid = selectedLlmId !== '';
      setIsFormValid(isValid);
      setStepValid('configure-ai', isValid);
      return isValid;
    }

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
  }, [formData, llmType, selectedLlmId, setStepValid]);

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
  }, [checkRequiredFields]);
  
  useEffect(() => {
    if (llmData && Object.keys(llmData).length > 0) {
      setFormData(llmData);
    }
  }, [llmData]);
  
  const handleLlmTypeChange = (e) => {
    setLlmType(e.target.value);
    
    if (e.target.value === 'new') {
      setSelectedLlmId('');
      setCreatedLlmId('');
      if (selectedLlmId) {
        setFormData({
          name: '',
          llmProvider: '',
          apiEndpoint: '',
          apiKey: '',
          privacyLevel: 'public'
        });
      }
    }
  };
  
  const handleExistingLlmChange = (e) => {
    const llmId = e.target.value;
    setSelectedLlmId(llmId);
    
    const selectedLlm = availableLLMs.find(llm => llm.id === llmId);
    if (selectedLlm) {
      setFormData({
        name: selectedLlm.attributes.name,
        llmProvider: selectedLlm.attributes.vendor,
        apiEndpoint: selectedLlm.attributes.api_endpoint,
        apiKey: selectedLlm.attributes.api_key,
        privacyLevel: Object.keys(PRIVACY_LEVEL_SCORES).find(
          key => PRIVACY_LEVEL_SCORES[key] === selectedLlm.attributes.privacy_score
        ) || 'public'
      });
    }
  };
  
  const createOrUpdateLLM = async () => {
    setLoading(true);
    
    try {
      if (llmType === 'existing' && selectedLlmId) {
        const selectedLlm = availableLLMs.find(llm => llm.id === selectedLlmId);
        if (selectedLlm) {
          setLlmData({
            name: selectedLlm.attributes.name,
            llmProvider: selectedLlm.attributes.vendor,
            apiEndpoint: selectedLlm.attributes.api_endpoint,
            apiKey: selectedLlm.attributes.api_key,
            privacyLevel: Object.keys(PRIVACY_LEVEL_SCORES).find(
              key => PRIVACY_LEVEL_SCORES[key] === selectedLlm.attributes.privacy_score
            ) || 'public'
          });
          setCreatedLlmId(selectedLlmId);
          goToNextStep();
          return;
        }
      }
      
      const llmDataForApi = {
        name: formData.name,
        apiKey: formData.apiKey,
        apiEndpoint: formData.apiEndpoint,
        privacyScore: PRIVACY_LEVEL_SCORES[formData.privacyLevel],
        llmProvider: formData.llmProvider,
        active: true
      };
      
      let response;
      
      if (createdLlmId) {
        if (JSON.stringify(formData) !== JSON.stringify(llmData)) {
          await updateLLM(createdLlmId, llmDataForApi);
        }
      } else {
        response = await createLLM(llmDataForApi);
        const newLlmId = response.id;
        setCreatedLlmId(newLlmId);
      }
      
      setLlmData(formData);
      goToNextStep();
    } catch (error) {
      setSnackbar({
        open: true,
        message: `Failed to ${createdLlmId ? 'update' : 'create'} LLM: ${error.message}`,
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
        {availableLLMs.length > 0 && (
          <RadioSelectionGroup
            options={[
              {
                value: 'existing',
                label: 'Use existing LLM provider',
              },
              {
                value: 'new',
                label: 'Add new LLM provider',
              }
            ]}
            value={llmType}
            onChange={handleLlmTypeChange}
            renderContent={(option) => {
              if (option.value === 'existing') {
                return (
                  <Box sx={{ ml: 4, mt: 2, mb: 2 }}>
                    <CustomSelect
                      name="existingLlm"
                      value={selectedLlmId}
                      onChange={handleExistingLlmChange}
                      options={availableLLMs.map(llm => ({
                        value: llm.id,
                        label: llm.attributes.name,
                        vendor: llm.attributes.vendor
                      }))}
                      renderOption={(option) => (
                        <Box sx={{ display: "flex", alignItems: "center" }}>
                          <img
                            src={getVendorLogo(option.vendor || '')}
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
                  </Box>
                );
              }
              return null;
            }}
          />
        )}

        {(llmType === 'new' || availableLLMs.length === 0) && (
          <>
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
                  name="apiEndpoint"
                  value={formData.apiEndpoint}
                  onChange={handleChange}
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
                  name="apiKey"
                  type="password"
                  value={formData.apiKey}
                  onChange={handleChange}
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
                options={PRIVACY_LEVEL_OPTIONS}
                renderOption={(option) => (
                  <Box sx={{ display: 'flex', alignItems: 'center', width: '100%' }}>
                    <Box sx={{ mr: 2 }}>
                      <CustomSelectBadge config={PRIVACY_BADGE_CONFIGS[option.value]} />
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
          </>
        )}
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

export default memo(ConfigureAIStep);