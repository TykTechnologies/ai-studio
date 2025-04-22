import React, { useState } from 'react';
import {
  Box,
  Typography,
  Divider,
  Collapse,
  IconButton,
  Paper,
  Stack,
  Button
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
    skipQuickStart,
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
              xs: '0.95rem',
              sm: 'inherit'
            }
          }}
        >
          Your app has been created. Please review the details below and copy the access information and credentials to interact with the LLM in your app.
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
        <Stack spacing={0}>
          <Stack
            direction="row"
            alignItems={{ xs: "center", sm: "end" }}
            mb={1.5}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                LLM provider
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {llmData.llmProvider || 'Not specified'}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Stack
            direction="row"
            alignItems={{ xs: "center", sm: "end" }}
            my={1.5}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Owner
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {getOwnerName(ownerData)}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Stack
            direction="row"
            alignItems={{ xs: "center", sm: "end" }}
            mt={1.5}
            mb={1}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                App name
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {appData.name || 'Not specified'}
              </Typography>
            </Box>
          </Stack>
          
          <Stack
            direction="row"
            alignItems={{ xs: "center", sm: "end" }}
            mb={1}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Description
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {appData.description || 'No description'}
              </Typography>
            </Box>
          </Stack>
          
          <Stack
            direction="row"
            alignItems={{ xs: "center", sm: "end" }}
            mb={1.5}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Budget limit
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
                {getBudgetLimitText(appData)}
              </Typography>
            </Box>
          </Stack>
          
          <Divider sx={{ borderColor: 'border.neutralDefault' }} />
          
          <Typography variant="bodyLargeBold" color="text.primary" mt={1.5}>
            Credentials
          </Typography>
          
          <Stack
            direction="row"
            alignItems="center"
            mt={0.75}
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography variant="bodyLargeBold" color="text.primary">
                Key ID
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Box sx={{
                display: "flex",
                alignItems: "center",
                mt: { xs: 0.5, sm: 0 }
              }}>
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
                        top: { xs: -25, sm: -30 },
                        left: { xs: -5, sm: -10 },
                        bgcolor: 'background.paper',
                        p: 0.5,
                        borderRadius: 1,
                        boxShadow: 1,
                        zIndex: 10
                      }}>
                        <Typography variant="caption">Copied!</Typography>
                      </Box>
                    )}
                  </IconButton>
                )}
              </Box>
            </Box>
          </Stack>
          
          <Stack
            direction="row"
            alignItems="center"
            spacing={0}
          >
            <Box sx={{ width: '25%', minWidth: '100px' }}>
              <Typography
                variant="bodyLargeBold"
                color="text.primary"
                sx={{
                  fontSize: {
                    xs: '1rem',
                    sm: 'inherit'
                  }
                }}
              >
                Secret
              </Typography>
            </Box>
            <Box sx={{ width: '75%' }}>
              <Box sx={{
                display: "flex",
                alignItems: "center",
                mt: { xs: 0.5, sm: 0 }
              }}>
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
                        top: { xs: -25, sm: -30 },
                        left: { xs: -5, sm: -10 },
                        bgcolor: 'background.paper',
                        p: 0.5,
                        borderRadius: 1,
                        boxShadow: 1,
                        zIndex: 10
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
            <Typography
              variant="bodySmallDefault"
              color="text.defaultSubdued"
              sx={{
                mb: 2,
                display: 'block',
                fontSize: {
                  xs: '0.85rem',
                  sm: 'inherit'
                }
              }}
            >
              Use these URLs in your app with the right vendor SDK or API.
            </Typography>
          </Box>
          
          <Box sx={{ mb: 2 }}>
            <Box sx={{
              display: 'flex',
              flexDirection: { xs: 'column', sm: 'row' },
              alignItems: { xs: 'flex-start', sm: 'center' },
              mb: 1
            }}>
              <Typography
                variant="bodyLargeMedium"
                color="text.defaultSubdued"
                sx={{
                  width: 120,
                  minWidth: '80px',
                  fontSize: {
                    xs: '0.9rem',
                    sm: 'inherit'
                  }
                }}
              >
                REST API:
              </Typography>
              <Box sx={{
                display: 'flex',
                alignItems: 'center',
                flex: 1
              }}>
                <Typography
                  variant="bodyLargeDefault"
                  color="text.defaultSubdued"
                  sx={{
                    mr: 1,
                    fontSize: {
                      xs: '0.9rem',
                      sm: 'inherit'
                    },
                    wordBreak: 'break-all'
                  }}
                >
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
                      top: { xs: -25, sm: -30 },
                      left: { xs: -5, sm: -10 },
                      bgcolor: 'background.paper',
                      p: 0.5,
                      borderRadius: 1,
                      boxShadow: 1,
                      zIndex: 10
                    }}>
                      <Typography variant="caption">Copied!</Typography>
                    </Box>
                  )}
                </IconButton>
              </Box>
            </Box>
            
            <Box sx={{
              display: 'flex',
              flexDirection: { xs: 'column', sm: 'row' },
              alignItems: { xs: 'flex-start', sm: 'center' }
            }}>
              <Typography
                variant="bodyLargeMedium"
                color="text.defaultSubdued"
                sx={{
                  width: 120,
                  minWidth: '80px',
                  fontSize: {
                    xs: '0.9rem',
                    sm: 'inherit'
                  }
                }}
              >
                STREAM API:
              </Typography>
              <Box sx={{
                display: 'flex',
                alignItems: 'center',
                flex: 1
              }}>
                <Typography
                  variant="bodyLargeDefault"
                  color="text.defaultSubdued"
                  sx={{
                    mr: 1,
                    fontSize: {
                      xs: '0.9rem',
                      sm: 'inherit'
                    },
                    wordBreak: 'break-all'
                  }}
                >
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
                      top: { xs: -25, sm: -30 },
                      left: { xs: -5, sm: -10 },
                      bgcolor: 'background.paper',
                      p: 0.5,
                      borderRadius: 1,
                      boxShadow: 1,
                      zIndex: 10
                    }}>
                      <Typography variant="caption">Copied!</Typography>
                    </Box>
                  )}
                </IconButton>
              </Box>
            </Box>
          </Box>
          
          <Box>
            <Typography
              variant="bodyLargeMedium"
              color="text.primary"
              sx={{
                mb: 1,
                mt: 3,
                fontSize: {
                  xs: '0.95rem',
                  sm: 'inherit'
                }
              }}
            >
              OpenAI compatible Endpoint
            </Typography>
            <Typography
              variant="bodySmallDefault"
              color="text.defaultSubdued"
              sx={{
                mb: 2,
                display: 'block',
                fontSize: {
                  xs: '0.85rem',
                  sm: 'inherit'
                }
              }}
            >
              The Unified Endpoint is an OpenAI-compatible endpoint that converts your API calls for each vendor. It doesn't support streams.
            </Typography>
          </Box>
          
          <Box sx={{
            display: 'flex',
            flexDirection: { xs: 'column', sm: 'row' },
            alignItems: { xs: 'flex-start', sm: 'center' }
          }}>
            <Typography
              variant="bodyLargeMedium"
              color="text.defaultSubdued"
              sx={{
                width: 120,
                minWidth: '80px',
                fontSize: {
                  xs: '1rem',
                  sm: 'inherit'
                }
              }}
            >
              Unified API:
            </Typography>
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              flex: 1
            }}>
              <Typography
                variant="bodyLargeDefault"
                color="text.defaultSubdued"
                sx={{
                  mr: 1,
                  fontSize: {
                    xs: '0.9rem',
                    sm: 'inherit'
                  },
                  wordBreak: 'break-all'
                }}
              >
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
                    top: { xs: -25, sm: -30 },
                    left: { xs: -5, sm: -10 },
                    bgcolor: 'background.paper',
                    p: 0.5,
                    borderRadius: 1,
                    boxShadow: 1,
                    zIndex: 10
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
              mb: 1,
              flexDirection: 'row'
            }}
            onClick={toggleCurlExpanded}
          >
            <Box sx={{
              display: 'flex',
              alignItems: 'center',
              mr: 1
            }}>
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
            <Typography
              variant="bodyLargeBold"
              color="text.primary"
              sx={{
                fontSize: {
                  xs: '1rem',
                  sm: 'inherit'
                }
              }}
            >
              Curl example
            </Typography>
            <IconButton 
              size="small" 
              onClick={(e) => {
                e.stopPropagation();
                copyToClipboard(getCurlExample(llmData.llmProvider), 'curl');
              }}
              sx={{
                ml: 1,
                position: 'relative'
              }}
            >
              <ContentCopyIcon fontSize="small" />
              {copyTooltips.curl && (
                <Box sx={{ 
                  position: 'absolute',
                  top: { xs: -25, sm: -30 },
                  left: { xs: -5, sm: -10 },
                  bgcolor: 'background.paper',
                  p: 0.5,
                  borderRadius: 1,
                  boxShadow: 1,
                  zIndex: 10
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
                p: { xs: 1.5, sm: 2 },
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
                  fontSize: { xs: '13px', sm: '12px' },
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
      
      <ActionsContainer sx={{
        flexWrap: 'wrap',
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 2,
        width: '100%',
        padding: { xs: 2, sm: 0 },
        mt: 2
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
            onClick={goToNextStep}
            sx={{ minWidth: '100px' }}
          >
            Finish
          </PrimaryButton>
        </Box>
      </ActionsContainer>
    </Box>
  );
};

export default SummaryStep;