import React, { useState, useEffect, memo, useCallback } from 'react';
import {
  Box,
  Typography,
  Divider,
  CircularProgress,
  Snackbar,
  Alert,
  Button,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormControl,
} from '@mui/material';
import { StyledTextField } from '../../../styles/sharedStyles';
import { useQuickStart } from './QuickStartContext';
import { createUser, updateUser } from '../../../services';
import { ActionsContainer } from './styles';
import { PrimaryButton, SecondaryLinkButton } from '../../../styles/sharedStyles';
import CustomSelect from '../../common/CustomSelect';
import CustomSelectBadge from '../../common/CustomSelectBadge';
import RadioSelectionGroup from '../../common/RadioSelectionGroup';
import { validateEmail, validatePassword } from './utils';

const AssignOwnerStep = () => {
  const {
    setStepValid,
    goToNextStep,
    goToPreviousStep,
    skipQuickStart,
    ownerData,
    setOwnerData,
    createdOwnerId,
    setCreatedOwnerId,
    currentUser
  } = useQuickStart();
  
  const [ownerType, setOwnerType] = useState(ownerData.ownerType || 'current');
  const [formData, setFormData] = useState({
    name: '',
    email: '',
    password: '',
    role: 'developer'
  });
  const [errors, setErrors] = useState({});
  const [isFormValid, setIsFormValid] = useState(false);
  const [loading, setLoading] = useState(false);
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
    lowercase: false,
  });
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: '',
    severity: 'success'
  });

  const roleConfigs = {
    chatUser: {
      icon: 'comment',
      text: 'Chat User',
      textColor: 'text.defaultSubdued',
      bgColor: 'background.buttonPrimaryOutlineHover'
    },
    developer: {
      icon: 'code',
      text: 'Developer',
      textColor: 'text.defaultSubdued',
      bgColor: 'background.surfaceBrandDefaultPortal'
    },
    admin: {
      icon: 'shield',
      text: 'Admin',
      textColor: 'text.defaultSubdued',
      bgColor: 'background.surfaceBrandDefaultDashboard'
    }
  };

  const checkRequiredFields = useCallback(() => {
    if (ownerType === 'current') {
      setIsFormValid(true);
      setStepValid('assign-owner', true);
      return true;
    }

    const isValid = 
      formData.name.trim() !== '' && 
      formData.email.trim() !== '' && 
      formData.password.trim() !== '' && 
      formData.role.trim() !== '';
    
    setIsFormValid(isValid);
    setStepValid('assign-owner', isValid);
    return isValid;
  }, [formData, ownerType, setStepValid]);

  useEffect(() => {
    checkRequiredFields();
  }, [checkRequiredFields]);

  useEffect(() => {
    setPasswordCriteria({
      length: formData.password.length >= 8,
      number: /\d/.test(formData.password),
      special: /[!@#$%^&*(),.?":{}|<>_+=\-~]/.test(formData.password),
      uppercase: /[A-Z]/.test(formData.password),
      lowercase: /[a-z]/.test(formData.password),
    });
  }, [formData.password]);
  
  useEffect(() => {
    if (ownerData && Object.keys(ownerData).length > 0) {
      setOwnerType(ownerData.ownerType || 'current');
      if (ownerData.formData) {
        setFormData(ownerData.formData);
      }
    }
  }, [ownerData]);

  const validateForm = () => {
    if (ownerType === 'current') {
      return true;
    }

    const newErrors = {};
    
    const emailError = validateEmail(formData.email);
    if (emailError) {
      newErrors.email = emailError;
    }
    
    const passwordError = validatePassword(passwordCriteria);
    if (passwordError) {
      newErrors.password = passwordError;
    }
    
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };
  
  const createOrUpdateOwner = async () => {
    setLoading(true);
    
    try {
      if (ownerType === 'current') {
        setOwnerData({
          ownerType: 'current',
          userId: currentUser?.id,
          name: currentUser?.name,
          email: currentUser?.email,
          role: 'admin'
        });
        goToNextStep();
        return;
      }

      const userDataForApi = {
        name: formData.name,
        email: formData.email,
        password: formData.password,
        isAdmin: formData.role === 'admin',
        showPortal: formData.role === 'developer' || formData.role === 'admin',
        showChat: true
      };
      
      let response;
      
      if (createdOwnerId) {
        if (JSON.stringify(formData) !== JSON.stringify(ownerData.formData)) {
          await updateUser(createdOwnerId, userDataForApi);
        }
      } else {
        response = await createUser(userDataForApi);
        const newUserId = response.id;
        setCreatedOwnerId(newUserId);
      }
      
      setOwnerData({
        ownerType: 'new',
        formData: formData,
        userId: createdOwnerId || (response?.id)
      });
      goToNextStep();
    } catch (error) {
      setSnackbar({
        open: true,
        message: `Failed to ${createdOwnerId ? 'update' : 'create'} user: ${error.message}`,
        severity: 'error'
      });
    } finally {
      setLoading(false);
    }
  };
  
  const handleNextClick = () => {
    if (validateForm()) {
      createOrUpdateOwner();
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
    
    if (errors[name]) {
      setErrors(prev => ({
        ...prev,
        [name]: undefined
      }));
    }
  };

  const handleOwnerTypeChange = (e) => {
    setOwnerType(e.target.value);
  };

  const roleOptions = [
    { value: 'chatUser', label: 'Chat User', description: 'Can only access the chat interface' },
    { value: 'developer', label: 'Developer', description: 'Can access chat and portal interfaces' },
    { value: 'admin', label: 'Admin', description: 'Full access to all features and administration' }
  ];

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
        Now let's choose who will own this app to directly access the LLM provider. You can assign yourself as the app owner or add a new user.
        </Typography>
      </Box>
      <Box sx={{
        my: 3,
        px: {
          xs: 2,
          sm: 4,
          md: 10,
          lg: 25
        }
      }}>
        <RadioSelectionGroup
          options={[
            {
              value: 'current',
              label: 'Set me as owner',
            },
            {
              value: 'new',
              label: 'Add a new user',
            }
          ]}
          value={ownerType}
          onChange={handleOwnerTypeChange}
          renderContent={(option) => {
            if (option.value === 'new') {
              return (
                <Box sx={{ ml: 4, mt: 2, mb: 2 }}>
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

                  <Box sx={{
                    display: 'flex',
                    flexDirection: { xs: 'column', sm: 'row' },
                    gap: 2,
                    mb: 4
                  }}>
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        variant="bodyLargeBold"
                        color="text.primary"
                        sx={{ mb: 1 }}
                      >
                        Email*
                      </Typography>
                      <StyledTextField
                        fullWidth
                        name="email"
                        type="email"
                        value={formData.email}
                        onChange={handleChange}
                        error={!!errors.email}
                        helperText={errors.email}
                        required
                        autoComplete="off"
                      />
                    </Box>
          
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        variant="bodyLargeBold"
                        color="text.primary"
                        sx={{ mb: 1 }}
                      >
                        Password*
                      </Typography>
                      <StyledTextField
                        fullWidth
                        name="password"
                        type="password"
                        value={formData.password}
                        onChange={handleChange}
                        error={!!errors.password}
                        helperText={errors.password}
                        required
                        autoComplete="new-password"
                      />
                    </Box>
                  </Box>

                  <Box sx={{ mb: 6 }}>
                    <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
                      Role*
                    </Typography>
                    <CustomSelect
                      name="role"
                      value={formData.role}
                      onChange={handleChange}
                      options={roleOptions}
                      error={!!errors.role}
                      helperText={errors.role}
                      required
                      renderOption={(option) => (
                        <Box sx={{ display: 'flex', alignItems: 'center', width: '100%' }}>
                          <Box sx={{ mr: 2 }}>
                            <CustomSelectBadge config={roleConfigs[option.value]} />
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
                </Box>
              );
            }
            return null;
          }}
        />
      </Box>
      
      <ActionsContainer sx={{
        flexWrap: 'wrap',
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 2,
        width: '100%',
        padding: { xs: 2, sm: 0 },
        mt: 1
      }}>
        <SecondaryLinkButton
          onClick={skipQuickStart}
          sx={{
            minWidth: '120px',
            flex: { xs: '1 1 100%', sm: '0 1 auto' }
          }}
        >
          Skip quick start
        </SecondaryLinkButton>
        
        <Box sx={{
          display: 'flex',
          gap: 2,
          flex: { xs: '1 1 100%', sm: '0 1 auto' },
          justifyContent: { xs: 'space-between', sm: 'flex-end' }
        }}>
          <Button
            onClick={goToPreviousStep}
            sx={{ minWidth: '80px' }}
          >
            Back
          </Button>
          <PrimaryButton
            onClick={handleNextClick}
            disabled={!isFormValid || loading}
            sx={{ minWidth: '100px' }}
          >
            {loading ? <CircularProgress size={24} color="inherit" /> : 'Continue'}
          </PrimaryButton>
        </Box>
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

export default memo(AssignOwnerStep);