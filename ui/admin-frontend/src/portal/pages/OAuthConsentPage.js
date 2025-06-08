import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button, Typography, Container, Paper, Box, CircularProgress, List, ListItem, ListItemText, Divider } from '@mui/material';
import apiClient from '../../admin/utils/apiClient'; // Corrected import path

function OAuthConsentPage() {
  const [clientName, setClientName] = useState('');
  const [scopes, setScopes] = useState([]);
  const [authRequestID, setAuthRequestID] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
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

    apiClient.get(`/oauth/consent_details?auth_req_id=${reqId}`)
      .then(response => {
        setClientName(response.data.client_name);
        setScopes(response.data.scopes || []);
        setLoading(false);
      })
      .catch(err => {
        const errorMsg = err.response?.data?.errors?.[0]?.detail || err.message || 'Failed to fetch consent details.';
        setError(errorMsg);
        setLoading(false);
      });
  }, [location.search]);

  const handleSubmitConsent = (decision) => {
    setLoading(true);
    apiClient.post('/oauth/submit_consent', {
      auth_req_id: authRequestID,
      decision: decision,
    })
    .then(response => {
      // Assuming the backend handles the redirect directly or provides a URL.
      // If backend sends redirect URL:
      if (response.data && response.data.redirect_url) {
        window.location.href = response.data.redirect_url;
      } else {
        // If no redirect_url in response, it implies backend handled redirect
        // or the test environment doesn't show XHR redirects easily.
        // For robustness, could add a fallback or success message if no redirect_url.
        // For now, if it doesn't redirect, it will just stop loading.
        // Potentially, the browser might have already redirected if it was a non-XHR POST response.
        // This part might need adjustment based on actual backend behavior for XHR POSTs.
         setLoading(false); // Stop loading if no explicit redirect URL given
      }
    })
    .catch(err => {
      const errorMsg = err.response?.data?.errors?.[0]?.detail || err.message || 'Failed to submit consent.';
      setError(errorMsg);
      setLoading(false);
    });
  };

  if (loading) {
    return (
      <Container component="main" maxWidth="xs" sx={{ mt: 8, display: 'flex', justifyContent: 'center' }}>
        <CircularProgress />
      </Container>
    );
  }

  if (error) {
    return (
      <Container component="main" maxWidth="xs" sx={{ mt: 8 }}>
        <Paper elevation={3} sx={{ p: 4, textAlign: 'center' }}>
          <Typography component="h1" variant="h5" color="error">
            Error
          </Typography>
          <Typography sx={{ mt: 2 }}>{error}</Typography>
          <Button
            fullWidth
            variant="contained"
            sx={{ mt: 3 }}
            onClick={() => navigate('/')}
          >
            Go to Homepage
          </Button>
        </Paper>
      </Container>
    );
  }

  return (
    <Container component="main" maxWidth="sm" sx={{ mt: 8 }}>
      <Paper elevation={3} sx={{ p: 4 }}>
        <Typography component="h1" variant="h5" align="center" gutterBottom>
          Authorize Application
        </Typography>
        <Typography variant="body1" sx={{ mt: 2, mb: 1 }}>
          The application <Box component="strong" sx={{ fontWeight: 'bold' }}>{clientName}</Box> is requesting permission to:
        </Typography>
        <List dense>
          {scopes.length > 0 ? scopes.map((scope, index) => (
            <ListItem key={index}>
              <ListItemText primary={scope} />
            </ListItem>
          )) : (
            <ListItem>
              <ListItemText primary="Access your basic information." />
            </ListItem>
          )}
        </List>
        <Divider sx={{ my: 2 }} />
        <Typography variant="caption" display="block" gutterBottom>
          By clicking "Allow", you allow this application to access your data as specified above.
        </Typography>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', mt: 3 }}>
          <Button
            variant="outlined"
            color="error"
            onClick={() => handleSubmitConsent('denied')}
            sx={{ flexGrow: 1, mr: 1 }}
          >
            Deny
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={() => handleSubmitConsent('approved')}
            sx={{ flexGrow: 1, ml: 1 }}
          >
            Allow
          </Button>
        </Box>
      </Paper>
    </Container>
  );
}

export default OAuthConsentPage;
