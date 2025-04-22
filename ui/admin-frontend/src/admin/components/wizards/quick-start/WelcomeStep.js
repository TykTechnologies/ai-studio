import React from 'react';
import { Box, Typography, Button, useMediaQuery } from '@mui/material';
import { styled } from '@mui/material/styles';
import { PrimaryButton } from '../../../styles/sharedStyles';
import { ActionsContainer } from './styles';
import { useQuickStart } from './QuickStartContext';

// Import the welcome step image
import welcomeStepImage from './welcome_step.png';

const ImageContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'center',
  position: 'relative',
  padding: theme.spacing(4),
  '@media (max-width: 600px)': {
    padding: theme.spacing(2),
  },
}));

const WelcomeImage = styled('img')({
  maxWidth: '100%',
  maxHeight: '30vh',
  height: 'auto',
  objectFit: 'contain',
});

const WelcomeStep = ({ userName = 'User' }) => {
  const { goToNextStep, skipQuickStart } = useQuickStart();
  const isMobile = useMediaQuery('(max-width:600px)');
  
  return (
    <Box sx={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      padding: isMobile ? 2 : 3
    }}>
      <Typography
        variant="headingXLarge"
        color="text.primary"
        align="center"
        sx={{
          '@media (max-width: 600px)': {
            fontSize: '1.75rem'
          }
        }}
      >
        Welcome to Tyk, {userName}
      </Typography>
      
      <Typography
        variant="headingMedium"
        color="text.defaultSubdued"
        align="center"
        sx={{
          mt: 2,
          '@media (max-width: 600px)': {
            fontSize: '1.25rem'
          }
        }}
      >
        Empower your team to build AI Apps with direct access to LLM providers and data sources,
      </Typography>
      
      <Typography
        variant="bodyXLargeDefault"
        color="text.defaultSubdued"
        align="center"
        sx={{
          mt: 1,
          '@media (max-width: 600px)': {
            fontSize: '1rem'
          }
        }}
      >
        which can be used for code editors, knowledge search, and task automation keeping centralized control, usage tracking, and security.
      </Typography>
      
      <ImageContainer>
        <WelcomeImage src={welcomeStepImage} alt="Welcome to Tyk AI Studio" />
      </ImageContainer>
      
      <Box sx={{
        mt: 2,
        maxWidth: isMobile ? '90%' : '60%',
        textAlign: 'center'
      }}>
        <Typography
          variant="bodyLargeDefault"
          color="text.defaultSubdued"
        >
          Start by creating an <Typography component="span" variant="bodyLargeBold" display="inline">AI studio App</Typography> in three easy steps: configure AI infrastructure, manage access, and add details. Get credentials and access details to work directly with AI.
        </Typography>
      </Box>
      <Box sx={{
        mt: 3,
        width: isMobile ? '100%' : 'auto',
        px: isMobile ? 2 : 0,
        display: 'flex',
        justifyContent: 'center'
      }}>
        <ActionsContainer>
          <Button onClick={skipQuickStart}>
            Explore by myself
          </Button>
          <PrimaryButton onClick={goToNextStep}>
            Quick start
          </PrimaryButton>
        </ActionsContainer>
      </Box>
    </Box>
  );
};

export default WelcomeStep;