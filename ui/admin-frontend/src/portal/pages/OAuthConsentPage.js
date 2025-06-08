import React, { useState, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button, Typography, Container, Paper, Box, CircularProgress, List, ListItem, ListItemText, Divider } from '@mui/material';
import apiClient from '../../common/api/apiClient'; // Assuming apiClient is configured

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
      // The backend will respond with a redirect. If the request is XHR,
      // the browser won't follow it directly. We need to check where the backend
      // wants to redirect. For simplicity, we assume the backend handles the redirect
      // correctly and the browser will follow if this POST was a normal form post.
      // If this is an XHR post (typical for React SPAs), the backend should return
      // the redirect URL in the response body for the frontend to navigate to.
      // For this implementation, we assume the backend's redirect is followed,
      // or if not, the user might be stuck. A more robust solution would handle
      // the redirect URL from the response.
      // For now, if the call succeeds but no explicit redirect from backend via JS,
      // it might just show loading. The task implies backend handles redirect.
      // If a redirect happens, this component might unmount.
      // If it's a same-origin redirect, it might work seamlessly.
      // If it's a cross-origin redirect from an XHR, it will be blocked.
      // The form POST method is usually better for cross-origin redirects.
      // Let's assume the backend correctly redirects the top-level window.
      // If the backend can't directly redirect after a POST from XHR,
      // it should send back the URL and the frontend does window.location.href = url;
      if (response.data && response.data.redirect_url) {
        window.location.href = response.data.redirect_url;
      } else {
        // If backend doesn't provide a redirect URL, it implies it handled the redirect.
        // If still loading, it means the page hasn't been redirected by the browser yet.
        // This state might not be visible if redirect is immediate.
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
            onClick={() => navigate('/')} // Navigate to a safe page, e.g., home
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
