import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Divider,
  CircularProgress,
  Snackbar,
  Alert,
  Switch,
  FormControlLabel,
  InputAdornment
} from '@mui/material';
import { StyledTextField } from '../../../styles/sharedStyles';
import { useQuickStart } from './QuickStartContext';
import apiClient from '../../../utils/apiClient';
import { ActionsContainer } from './styles';
import { PrimaryButton, SecondaryLinkButton } from '../../../styles/sharedStyles';
import CustomNote from '../../common/CustomNote';
import Icon from '../../../../components/common/Icon';

const AppDetailsStep = () => {
  const {
    setStepValid,
    goToNextStep,
    goToPreviousStep,
    appData,
    setAppData,
    createdAppId,
    setCreatedAppId,
    ownerData,
    createdLlmId,
    setCredentialData
  } = useQuickStart();
  
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    setBudget: false,
    monthlyBudget: '',
    budgetStartDate: new Date().toISOString().split('T')[0]
  });
  const [errors, setErrors] = useState({});
  const [isFormValid, setIsFormValid] = useState(false);
  const [loading, setLoading] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success'
  });

  const checkRequiredFields = React.useCallback(() => {
    const isValid = formData.name.trim() !== '';
    setIsFormValid(isValid);
    setStepValid('app-details', isValid);
    return isValid;
  }, [formData, setStepValid]);

  useEffect(() => {
    checkRequiredFields();
  }, [formData, checkRequiredFields]);
  
  useEffect(() => {
    if (appData && Object.keys(appData).length > 0) {
      setFormData(appData);
    }
  }, [appData]);

  const validateForm = () => {
    const newErrors = {};
    if (!formData.name.trim()) newErrors.name = "Name is required";
    
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };
  
  const createOrUpdateApp = async () => {
    setLoading(true);
    
    try {
      const appPayload = {
        data: {
          type: "apps",
          attributes: {
            name: formData.name,
            description: formData.description,
            user_id: ownerData.userId,
            llm_ids: createdLlmId ? [parseInt(createdLlmId, 10)] : [],
            datasource_ids: [],
            monthly_budget: formData.setBudget ? parseFloat(formData.monthlyBudget) : null,
            budget_start_date: formData.setBudget ? new Date(formData.budgetStartDate).toISOString() : null
          }
        }
      };
      
      let response;
      
      if (createdAppId) {
        if (JSON.stringify(formData) !== JSON.stringify(appData)) {
          response = await apiClient.patch(`/apps/${createdAppId}`, appPayload);
          setSnackbar({
            open: true,
            message: 'App updated successfully',
            severity: 'success'
          });
        }
      } else {
        response = await apiClient.post('/apps', appPayload);
        console.log('App created:', response.data);
        const newAppId = response.data.data.id;
        setCreatedAppId(newAppId);
        
        // Fetch the credential details using the credential_id from the response
        if (response.data.data.attributes.credential_id) {
          const credentialId = response.data.data.attributes.credential_id;
          const credentialResponse = await apiClient.get(`/credentials/${credentialId}`);
          if (credentialResponse.data && credentialResponse.data.data) {
            setCredentialData({
              keyID: credentialResponse.data.data.attributes.key_id,
              secret: credentialResponse.data.data.attributes.secret,
              active: credentialResponse.data.data.attributes.active
            });
          }
        }
        
        setSnackbar({
          open: true,
          message: 'App created successfully',
          severity: 'success'
        });
      }
      
      setAppData(formData);
      goToNextStep();
    } catch (error) {
      console.error('Error creating/updating app:', error);
      setSnackbar({
        open: true,
        message: `Failed to ${createdAppId ? 'update' : 'create'} app: ${error.message || 'Unknown error'}`,
        severity: 'error'
      });
    } finally {
      setLoading(false);
    }
  };
  
  const handleNextClick = () => {
    if (validateForm()) {
      createOrUpdateApp();
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleBudgetToggle = (e) => {
    setFormData(prev => ({
      ...prev,
      setBudget: e.target.checked
    }));
  };

  return (
    <Box sx={{ width: '100%', pt: 2 }}>
      <Box sx={{ textAlign: 'center', mb: 4, px: 10 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Finally, let's add a name and description to your app so developers know what it's for. Once you create the app, you'll get credentials for developers to access the gateway API and work directly with the LLM.
        </Typography>
      </Box>
      <Box sx={{ mt: 3, px: 25 }}>
        <Box sx={{ mb: 3 }}>
          <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
            App Name*
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
            Description
          </Typography>
          <StyledTextField
            fullWidth
            name="description"
            value={formData.description}
            onChange={handleChange}
            autoComplete="off"
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
              xxx characters max
            </Typography>
          </Box>
        </Box>

        <Divider sx={{ borderColor: 'border.neutralDefault', mb: 3 }} />

        <Box sx={{ mb: 3 }}>
          <FormControlLabel
            control={
              <Switch
                checked={formData.setBudget}
                onChange={handleBudgetToggle}
                sx={{
                  '& .MuiSwitch-switchBase.Mui-checked': {
                    color: theme => theme.palette.background.buttonPrimaryDefault
                  },
                  '& .MuiSwitch-switchBase.Mui-checked + .MuiSwitch-track': {
                    backgroundColor: theme => theme.palette.background.buttonPrimaryDefault
                  }
                }}
              />
            }
            label={
              <Typography variant="bodyLargeBold" color="text.primary">
                Set budget limit (optional)
              </Typography>
            }
          />
        </Box>

        {formData.setBudget && (
          <>
            <Box sx={{ display: 'flex', gap: 2, mb: 4 }}>
              <Box sx={{ flex: 1 }}>
                <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
                  Monthly budget*
                </Typography>
                <StyledTextField
                  fullWidth
                  name="monthlyBudget"
                  type="number"
                  value={formData.monthlyBudget}
                  onChange={handleChange}
                  InputProps={{
                    startAdornment: <InputAdornment position="start">$</InputAdornment>,
                  }}
                  required={formData.setBudget}
                  autoComplete="off"
                />
              </Box>

              <Box sx={{ flex: 1 }}>
                <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
                  Budget cycle start date
                </Typography>
                <StyledTextField
                  fullWidth
                  name="budgetStartDate"
                  type="date"
                  value={formData.budgetStartDate}
                  onChange={handleChange}
                  InputLabelProps={{
                    shrink: true,
                  }}
                  autoComplete="off"
                />
              </Box>
            </Box>

            <CustomNote
              message="To track expenses within this budget, set the provider's costs in the Model Prices section."
            />
          </>
        )}
      </Box>
      
      <ActionsContainer sx={{ justifyContent: 'space-between'}}>
        <SecondaryLinkButton onClick={goToPreviousStep}>
          Back
        </SecondaryLinkButton>
        <PrimaryButton
          onClick={handleNextClick}
          disabled={!isFormValid || loading}
        >
          {loading ? <CircularProgress size={24} color="inherit" /> : 'Create app'}
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

export default AppDetailsStep;