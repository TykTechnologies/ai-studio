import React, { useState } from 'react';
import {
  Box,
  Typography,
  Divider,
  Collapse,
  IconButton,
  Paper,
  Stack
} from '@mui/material';
import { useQuickStart } from './QuickStartContext';
import { ActionsContainer } from './styles';
import { PrimaryButton, SecondaryLinkButton } from '../../../styles/sharedStyles';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import { 
  generateEndpointUrl, 
  getBudgetLimitText, 
  getOwnerName, 
  getCurlExample 
} from './utils';

const SummaryStep = () => {
  const {
    goToNextStep,
    goToPreviousStep,
    llmData,
    ownerData,
    appData,
    credentialData
  } = useQuickStart();
  
  const [curlExpanded, setCurlExpanded] = useState(false);
  const [copyTooltips, setCopyTooltips] = useState({
    keyID: false,
    secret: false,
    restAPI: false,
    streamAPI: false,
    unifiedAPI: false,
    curl: false
  });

  const toggleCurlExpanded = () => {
    setCurlExpanded(!curlExpanded);
  };

  const copyToClipboard = (text, field) => {
    navigator.clipboard.writeText(text);
    
    setCopyTooltips(prev => ({ ...prev, [field]: true }));
    
    setTimeout(() => {
      setCopyTooltips(prev => ({ ...prev, [field]: false }));
    }, 2000);
  };

  return (
    <Box sx={{ width: '100%', pt: 2, position: 'relative' }}>
      <Box sx={{ textAlign: 'center', mb: 4, px: 10 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Your app has been created. Please review the details below and copy the access information and credentials to interact with the LLM in your app.
        </Typography>
      </Box>
      
      <Box sx={{ mt: 3, px: 25 }}>
        <Stack spacing={0}>
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end" mb={1.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                LLM provider
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {llmData.llmProvider || 'Not specified'}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end" my={1.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Owner
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {getOwnerName(ownerData)}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end" mt={1.5} mb={0.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                App name
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {appData.name || 'Not specified'}
              </Typography>
            </Box>
          </Stack>
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end" mb={0.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Description
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {appData.description || 'No description'}
              </Typography>
            </Box>
          </Stack>
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="end" mb={1.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Budget limit
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {getBudgetLimitText(appData)}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Typography variant="bodyLargeBold" color="text.primary" mt={1.5}>
            Credentials
          </Typography>
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center" mt={0.5}>
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Key ID
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Box sx={{ display: "flex", alignItems: "center" }}>
                <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mr: 0.5 }}>
                  {credentialData.keyID || 'Not available'}
                </Typography>
                {credentialData.keyID && (
                  <IconButton 
                    size="small" 
                    onClick={() => copyToClipboard(credentialData.keyID, 'keyID')}
                    sx={{ position: 'relative' }}
                  >
                    <ContentCopyIcon fontSize="small" />
                    {copyTooltips.keyID && (
                      <Box sx={{ 
                        position: 'absolute', 
                        top: -30, 
                        left: -10, 
                        bgcolor: 'background.paper', 
                        p: 0.5, 
                        borderRadius: 1,
                        boxShadow: 1
                      }}>
                        <Typography variant="caption">Copied!</Typography>
                      </Box>
                    )}
                  </IconButton>
                )}
              </Box>
            </Box>
          </Stack>
          
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems="center">
            <Box sx={{ width: { xs: '100%', md: '25%' } }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Secret
              </Typography>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '75%' } }}>
              <Box sx={{ display: "flex", alignItems: "center" }}>
                <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mr: 1 }}>
                  {credentialData.secret ? '••••••••••••••••' : 'Not available'}
                </Typography>
                {credentialData.secret && (
                  <IconButton 
                    size="small" 
                    onClick={() => copyToClipboard(credentialData.secret, 'secret')}
                    sx={{ position: 'relative' }}
                  >
                    <ContentCopyIcon fontSize="small" />
                    {copyTooltips.secret && (
                      <Box sx={{ 
                        position: 'absolute', 
                        top: -30, 
                        left: -10, 
                        bgcolor: 'background.paper', 
                        p: 0.5, 
                        borderRadius: 1,
                        boxShadow: 1
                      }}>
                        <Typography variant="caption">Copied!</Typography>
                      </Box>
                    )}
                  </IconButton>
                )}
              </Box>
            </Box>
          </Stack>
        </Stack>
        
        <Box
          sx={(theme) => ({
            mt: 1,
            p: 2,
            position: 'relative',
            border: '1px solid transparent',
            borderRadius: '8px',
            backgroundImage: `linear-gradient(white, white), linear-gradient(163.33deg, ${theme.palette.primary.main} 46.22%, ${theme.palette.custom.purpleExtraDark} 161.35%)`,
            backgroundOrigin: 'border-box',
            backgroundClip: 'padding-box, border-box'
          })}
        >
          <Typography variant="bodyLargeBold" color="text.primary">
            LLM Access Details
          </Typography>
          
          <Box sx={{ mt: 2 }}>
            <Typography variant="bodyLargeMedium" color="text.primary" sx={{ mb: 1 }}>
              SDK-Specific Endpoints
            </Typography>
            <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mb: 2, display: 'block' }}>
              Use these URLs in your app with the right vendor SDK or API.
            </Typography>
          </Box>
          
          <Box sx={{ mb: 2 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <Typography variant="bodyLargeMedium" color="text.defaultSubdued" sx={{ width: 120 }}>
                REST API:
              </Typography>
              <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
                <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mr: 1 }}>
                  {generateEndpointUrl('/llm/rest/', llmData.llmProvider || 'default')}
                </Typography>
                <IconButton 
                  size="small" 
                  onClick={() => copyToClipboard(generateEndpointUrl('/llm/rest/', llmData.llmProvider || 'default'), 'restAPI')}
                  sx={{ position: 'relative' }}
                >
                  <ContentCopyIcon fontSize="small" />
                  {copyTooltips.restAPI && (
                    <Box sx={{ 
                      position: 'absolute', 
                      top: -30, 
                      left: -10, 
                      bgcolor: 'background.paper', 
                      p: 0.5, 
                      borderRadius: 1,
                      boxShadow: 1
                    }}>
                      <Typography variant="caption">Copied!</Typography>
                    </Box>
                  )}
                </IconButton>
              </Box>
            </Box>
            
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <Typography variant="bodyLargeMedium" color="text.defaultSubdued" sx={{ width: 120 }}>
                STREAM API:
              </Typography>
              <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
                <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mr: 1 }}>
                  {generateEndpointUrl('/llm/stream/', llmData.llmProvider || 'default')}
                </Typography>
                <IconButton 
                  size="small" 
                  onClick={() => copyToClipboard(generateEndpointUrl('/llm/stream/', llmData.llmProvider || 'default'), 'streamAPI')}
                  sx={{ position: 'relative' }}
                >
                  <ContentCopyIcon fontSize="small" />
                  {copyTooltips.streamAPI && (
                    <Box sx={{ 
                      position: 'absolute', 
                      top: -30, 
                      left: -10, 
                      bgcolor: 'background.paper', 
                      p: 0.5, 
                      borderRadius: 1,
                      boxShadow: 1
                    }}>
                      <Typography variant="caption">Copied!</Typography>
                    </Box>
                  )}
                </IconButton>
              </Box>
            </Box>
          </Box>
          
          <Box>
            <Typography variant="bodyLargeMedium" color="text.primary" sx={{ mb: 1, mt: 3 }}>
              OpenAI compatible Endpoint
            </Typography>
            <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mb: 2, display: 'block' }}>
              The Unified Endpoint is an OpenAI-compatible endpoint that converts your API calls for each vendor. It doesn't support streams.
            </Typography>
          </Box>
          
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Typography variant="bodyLargeMedium" color="text.defaultSubdued" sx={{ width: 120 }}>
              Unified API:
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mr: 1 }}>
                {`${generateEndpointUrl('/ai/', llmData.llmProvider || 'default')}v1`}
              </Typography>
              <IconButton 
                size="small" 
                onClick={() => copyToClipboard(`${generateEndpointUrl('/ai/', llmData.llmProvider || 'default')}v1`, 'unifiedAPI')}
                sx={{ position: 'relative' }}
              >
                <ContentCopyIcon fontSize="small" />
                {copyTooltips.unifiedAPI && (
                  <Box sx={{ 
                    position: 'absolute', 
                    top: -30, 
                    left: -10, 
                    bgcolor: 'background.paper', 
                    p: 0.5, 
                    borderRadius: 1,
                    boxShadow: 1
                  }}>
                    <Typography variant="caption">Copied!</Typography>
                  </Box>
                )}
              </IconButton>
            </Box>
          </Box>
        </Box>
        
        <Box sx={{ mt: 1 }}>
          <Box 
            sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              cursor: 'pointer',
              mb: 1
            }}
            onClick={toggleCurlExpanded}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', mr: 1 }}>
              {curlExpanded ? (
                <KeyboardArrowUpIcon
                  sx={{
                    width: 18,
                    height: 18,
                    color: 'text.defaultSubdued'
                  }}
                />
              ) : (
                <KeyboardArrowDownIcon
                  sx={{
                    width: 18,
                    height: 18,
                    color: 'text.defaultSubdued'
                  }}
                />
              )}
            </Box>
            <Typography variant="bodyLargeBold" color="text.primary">
              Curl example
            </Typography>
            <IconButton 
              size="small" 
              onClick={(e) => {
                e.stopPropagation();
                copyToClipboard(getCurlExample(llmData.llmProvider), 'curl');
              }}
              sx={{ ml: 1, position: 'relative' }}
            >
              <ContentCopyIcon fontSize="small" />
              {copyTooltips.curl && (
                <Box sx={{ 
                  position: 'absolute', 
                  top: -30, 
                  left: -10, 
                  bgcolor: 'background.paper', 
                  p: 0.5, 
                  borderRadius: 1,
                  boxShadow: 1
                }}>
                  <Typography variant="caption">Copied!</Typography>
                </Box>
              )}
            </IconButton>
          </Box>
          
          <Collapse in={curlExpanded}>
            <Paper 
              elevation={0} 
              sx={{ 
                p: 2, 
                bgcolor: 'background.paper',
                border: '1px solid',
                borderColor: 'border.neutralDefault',
                borderRadius: 2
              }}
            >
              <Typography 
                variant="body1" 
                component="pre" 
                sx={{ 
                  fontFamily: 'Courier New',
                  fontSize: '12px',
                  fontWeight: 400,
                  lineHeight: '100%',
                  color: 'text.primary',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word'
                }}
              >
                {getCurlExample(llmData.llmProvider)}
              </Typography>
            </Paper>
          </Collapse>
        </Box>
      </Box>
      
      <ActionsContainer sx={{ justifyContent: 'space-between'}}>
        <SecondaryLinkButton onClick={goToPreviousStep}>
          Back
        </SecondaryLinkButton>
        <PrimaryButton onClick={goToNextStep}>
          Finish
        </PrimaryButton>
      </ActionsContainer>
    </Box>
  );
};

export default SummaryStep;