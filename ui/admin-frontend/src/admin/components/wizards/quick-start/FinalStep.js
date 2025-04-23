import React from 'react';
import { Box, Typography, Button } from '@mui/material';
import { styled } from '@mui/material/styles';
import { useNavigate } from 'react-router-dom';
import { PrimaryButton } from '../../../styles/sharedStyles';
import { ActionsContainer } from './styles';
import { useQuickStart } from './QuickStartContext';
import BasicCard from '../../../components/common/BasicCard';
import IconBadge from '../../../components/common/IconBadge';
import { createDocsLinkHandler } from '../../../utils/docsLinkUtils';
import useConfig from '../../../hooks/useConfig';
import finalStepImage from './final_step.png';

const ImageContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'center',
  position: 'relative',
  padding: theme.spacing(4),
}));

const FinalImage = styled('img')({
  maxWidth: '100%',
  maxHeight: '30vh',
  height: 'auto',
  objectFit: 'contain',
});

const BoldText = styled('span')(({ theme }) => ({
  fontFamily: 'Inter-Bold',
}));

const CardsContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexWrap: 'wrap',
  gap: theme.spacing(2),
  width: '100%',
  boxSizing: 'border-box',
  marginTop: theme.spacing(4),
}));

const CardWrapper = styled(Box)(({ theme }) => ({
  flex: '1 0 340px',
  maxWidth: '100%',
  boxSizing: 'border-box',
}));

const FinalStep = () => {
  const { skipQuickStart, createdAppId } = useQuickStart();
  const navigate = useNavigate();
  const { getDocsLink } = useConfig();
  
  const handleGoToApp = () => {
    if (createdAppId) {
      navigate(`/admin/apps/${createdAppId}`);
    }
  };
  
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', mt: 3 }}>
      <Typography variant="headingXLarge" color="text.primary" align="center">
        Congratulations on creating your first App!
      </Typography>
      
      <ImageContainer>
        <FinalImage src={finalStepImage} alt="Congratulations on creating your first App!" />
      </ImageContainer>
      
      <ActionsContainer>
        <Button onClick={skipQuickStart}>
          Proceed to overview
        </Button>
        <PrimaryButton onClick={handleGoToApp}>
          Go to my app
        </PrimaryButton>
      </ActionsContainer>
      
      <Typography variant="headingMedium" color="text.primary" align="center" sx={{ mt: 4 }}>
        Wondering what to do next? Explore more of Tyk AI Studio features
      </Typography>
      
      <CardsContainer>
        <CardWrapper>
          <BasicCard
            secondaryAction={{
              label: 'Learn more',
              onClick: createDocsLinkHandler(getDocsLink, 'data_sources')
            }}
          >
            <Box sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 2, 
              height: '100%', 
              justifyContent: 'flex-start',
              flexDirection: 'row',
              flexWrap: 'nowrap',
              padding: 1
            }}>
              <IconBadge iconName="book-sparkles" />
              <Typography variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                Enhance AI responses, add relevant context with <BoldText>Data sources</BoldText>
              </Typography>
            </Box>
          </BasicCard>
        </CardWrapper>
        
        <CardWrapper>
          <BasicCard
            secondaryAction={{
              label: 'Learn more',
              onClick: createDocsLinkHandler(getDocsLink, 'filters')
            }}
          >
            <Box sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 2, 
              height: '100%', 
              justifyContent: 'flex-start',
              flexDirection: 'row',
              flexWrap: 'nowrap',
              padding: 1
            }}>
              <IconBadge iconName="shield" />
              <Typography variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                Keep data safe with <BoldText>filters and privacy levels</BoldText>, ensure is used only by approved LLM providers
              </Typography>
            </Box>
          </BasicCard>
        </CardWrapper>

        <CardWrapper>
          <BasicCard
            secondaryAction={{
              label: 'Learn more',
              onClick: createDocsLinkHandler(getDocsLink, 'catalogs')
            }}
          >
            <Box sx={{ 
              display: 'flex', 
              alignItems: 'center', 
              gap: 2, 
              height: '100%', 
              justifyContent: 'flex-start',
              flexDirection: 'row',
              flexWrap: 'nowrap',
              padding: 1
            }}>
              <IconBadge iconName="rectangle-history" />
              <Typography variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                Manage which teams can have access to AI and data through <BoldText>Catalogs</BoldText>
              </Typography>
            </Box>
          </BasicCard>
        </CardWrapper>
      </CardsContainer>
    </Box>
  );
};

export default FinalStep;