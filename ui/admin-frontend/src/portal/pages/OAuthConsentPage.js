import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Typography, Box, CircularProgress, List, ListItem, ListItemText, FormControl, InputLabel, Select, MenuItem, Alert, Chip, useTheme } from '@mui/material';
import axios from 'axios';
import { fetchCSRFToken } from '../../admin/utils/urlUtils';
import AuthLayout from './AuthLayout';
import { PrimaryButton, DangerOutlineButton } from '../../admin/styles/sharedStyles';
import { FormLabel, FormText, StyledTextField } from '../styles/authStyles';

function OAuthConsentPage() {
  const theme = useTheme();
  const [clientName, setClientName] = useState('');
  const [scopes, setScopes] = useState([]);
  const [authRequestID, setAuthRequestID] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [availableApps, setAvailableApps] = useState([]);
  const [selectedAppId, setSelectedAppId] = useState('');
  const [noAppsMessage, setNoAppsMessage] = useState('');
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    const queryParams = new URLSearchParams(location.search);
    const reqId = queryParams.get('auth_req_id');

    if (!reqId) {
      setError('Authorization request ID is missing.');
      setLoading(false);
      return;
    }
    setAuthRequestID(reqId);

    axios.get(`/oauth/consent_details?auth_req_id=${reqId}`, {
        withCredentials: true
      })
      .then(response => {
        setClientName(response.data.client_name);
        setScopes(response.data.scopes || []);
        setAvailableApps(response.data.available_apps || []);
        setNoAppsMessage(response.data.no_apps_message || '');
        
        // Auto-select first app if only one available
        if (response.data.available_apps && response.data.available_apps.length === 1) {
          setSelectedAppId(response.data.available_apps[0].id);
        }
        
        setLoading(false);
      })
      .catch(err => {
        const errorMsg = err.response?.data?.errors?.[0]?.detail || err.message || 'Failed to fetch consent details.';
        setError(errorMsg);
        setLoading(false);
      });
  }, [location.search]);

  const handleSubmitConsent = async (decision) => {
    setLoading(true);
    
    // Always validate app selection for OAuth flows
    if (decision === 'approved' && !selectedAppId) {
      setError('Please select an app for OAuth access.');
      setLoading(false);
      return;
    }
    
    // Create a form to submit the consent decision
    // This allows the browser to naturally follow the 302 redirect
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/oauth/submit_consent';
    
    // Add auth_req_id field
    const authReqField = document.createElement('input');
    authReqField.type = 'hidden';
    authReqField.name = 'auth_req_id';
    authReqField.value = authRequestID;
    form.appendChild(authReqField);
    
    // Add decision field  
    const decisionField = document.createElement('input');
    decisionField.type = 'hidden';
    decisionField.name = 'decision';
    decisionField.value = decision;
    form.appendChild(decisionField);
    
    // Add selected app ID for OAuth flows
    if (decision === 'approved' && selectedAppId) {
      const appIdField = document.createElement('input');
      appIdField.type = 'hidden';
      appIdField.name = 'selected_app_id';
      appIdField.value = selectedAppId;
      form.appendChild(appIdField);
    }
    
    // Add CSRF token if needed
    try {
      const csrfToken = await fetchCSRFToken();
      if (csrfToken) {
        const csrfField = document.createElement('input');
        csrfField.type = 'hidden';
        csrfField.name = 'X-CSRF-Token';
        csrfField.value = csrfToken;
        form.appendChild(csrfField);
      }
    } catch (err) {
      console.warn('Could not fetch CSRF token:', err);
    }
    
    // Submit the form
    document.body.appendChild(form);
    form.submit();
  };

  if (loading) {
    return (
      <AuthLayout>
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
          <CircularProgress />
        </Box>
      </AuthLayout>
    );
  }

  if (error) {
    return (
      <AuthLayout>
        <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
          Authorization Error
        </Typography>
        
        <Alert severity="error" sx={{ mb: 3 }}>
          {error}
        </Alert>
        
        <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%' }}>
          <PrimaryButton
            onClick={() => navigate('/')}
          >
            Go to Homepage
          </PrimaryButton>
        </Box>
      </AuthLayout>
    );
  }

  const canApprove = selectedAppId || availableApps.length === 0;

  return (
    <AuthLayout>
      <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
        Authorize Application
      </Typography>
      
      <FormText sx={{ mt: 2, mb: 3, textAlign: 'center' }}>
        The application <Box component="strong" sx={{ fontWeight: 'bold', color: theme.palette.text.primary }}>{clientName}</Box> is requesting permission to:
      </FormText>
      <Box sx={{ backgroundColor: 'rgba(255, 255, 255, 0.1)', borderRadius: 2, p: 2, mb: 3 }}>
        <List dense sx={{ py: 0 }}>
          {scopes.length > 0 ? scopes.map((scope, index) => (
            <ListItem key={index} sx={{ py: 0.5, px: 0 }}>
              <ListItemText 
                primary={scope} 
                primaryTypographyProps={{ 
                  color: theme.palette.text.primary,
                  variant: 'bodyMediumDefault'
                }}
              />
            </ListItem>
          )) : (
            <ListItem sx={{ py: 0.5, px: 0 }}>
              <ListItemText 
                primary="Access selected MCP tools" 
                primaryTypographyProps={{ 
                  color: theme.palette.text.primary,
                  variant: 'bodyMediumDefault'
                }}
              />
            </ListItem>
          )}
        </List>
      </Box>
        
      <Box mb={2}>
        <FormLabel component="label">
          App Selection
        </FormLabel>
        {noAppsMessage ? (
          <Alert severity="warning" sx={{ mt: 1, mb: 2 }}>
            {noAppsMessage}
          </Alert>
        ) : (
          <>
            <FormText sx={{ mt: 1, mb: 2 }}>
              Select which app to use for OAuth access:
            </FormText>
            <FormControl fullWidth sx={{ mb: 2 }}>
              <Select
                value={selectedAppId}
                onChange={(e) => setSelectedAppId(e.target.value)}
                displayEmpty
                sx={{
                  width: '100%',
                  backgroundColor: theme.palette.custom.white,
                  borderRadius: '8px',
                  '& .MuiOutlinedInput-root': {
                    borderRadius: '8px',
                  },
                  '& .MuiOutlinedInput-notchedOutline': {
                    borderRadius: '8px',
                    border: 'none',
                  },
                  '&:hover .MuiOutlinedInput-notchedOutline': {
                    border: 'none',
                  },
                  '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
                    border: 'none',
                  },
                }}
              >
                <MenuItem value="" disabled>
                  <Typography variant="body2" color="text.secondary">
                    Select an app...
                  </Typography>
                </MenuItem>
                {availableApps.map((app) => (
                  <MenuItem key={app.id} value={app.id}>
                    <Box>
                      <Typography variant="body1">{app.name}</Typography>
                      {app.description && (
                        <Typography variant="caption" color="text.secondary">
                          {app.description}
                        </Typography>
                      )}
                    </Box>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            
            {selectedAppId && availableApps.find(app => app.id === selectedAppId)?.tools.length > 0 && (
              <Box sx={{ mb: 2 }}>
                <FormText sx={{ mb: 1 }}>
                  Available tools:
                </FormText>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                  {availableApps
                    .find(app => app.id === selectedAppId)
                    ?.tools.map((tool, index) => (
                      <Chip 
                        key={index} 
                        label={tool} 
                        size="small"
                        sx={{
                          backgroundColor: 'rgba(255, 255, 255, 0.2)',
                          color: theme.palette.text.primary,
                          '& .MuiChip-label': {
                            fontSize: '0.75rem'
                          }
                        }}
                      />
                    ))}
                </Box>
              </Box>
            )}
          </>
        )}
      </Box>
        
      <FormText sx={{ mt: 3, mb: 3, textAlign: 'center', fontSize: '0.875rem' }}>
        By clicking "Allow", you allow this application to access your data as specified above.
      </FormText>
      
      <Box sx={{ display: 'flex', gap: 2, mt: 3 }}>
        <DangerOutlineButton
          onClick={() => handleSubmitConsent('denied')}
          sx={{ flex: 1 }}
        >
          Deny
        </DangerOutlineButton>
        <PrimaryButton
          onClick={() => handleSubmitConsent('approved')}
          sx={{ flex: 1 }}
          disabled={!canApprove}
        >
          Allow
        </PrimaryButton>
      </Box>
    </AuthLayout>
  );
}

export default OAuthConsentPage;
