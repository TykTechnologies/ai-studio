import React from 'react';
import { Typography, Box, Stack, styled, CircularProgress } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { TitleBox, ContentBox, PrimaryButton } from '../styles/sharedStyles';
import useUserEntitlements from '../hooks/useUserEntitlements';
import useSystemFeatures from '../hooks/useSystemFeatures';
import BasicCard from '../components/common/BasicCard';
import IconBadge from '../components/common/IconBadge';

const SectionTitle = styled(Typography)(({ theme }) => ({
  marginBottom: theme.spacing(2),
  color: theme.palette.text.primary,
}));

const DescriptionSection = styled(Box)(({ theme }) => ({
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: '8px',
  padding: theme.spacing(3),
  marginBottom: theme.spacing(4),
  boxShadow: 'none',
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
}));

const DescriptionContent = styled(Box)(({ theme }) => ({
  width: '50%',
}));

const CardText = styled(Typography)(({ theme }) => ({
  marginBottom: theme.spacing(1),
}));

const BoldText = styled('span')(({ theme }) => ({
  fontFamily: 'Inter-Bold',
}));

const LoadingContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  justifyContent: 'center',
  alignItems: 'center',
  height: '100vh',
}));

const SectionContainer = styled(Box)(({ theme }) => ({
  marginBottom: theme.spacing(4),
}));

const Overview = () => {
  const { userEntitlements, userName, loading: entitlementsLoading, error: entitlementsError } = useUserEntitlements();
  const { features, loading: featuresLoading, error: featuresError } = useSystemFeatures();
  const navigate = useNavigate();

  const hasLLMs = userEntitlements?.llms?.length > 0;
  const showChatCard = features?.feature_chat;
  const showAppsCard = features?.feature_gateway || features?.feature_portal;
  
  const loading = entitlementsLoading || featuresLoading;
  const error = entitlementsError || featuresError;

  const handleNavigate = (path) => {
    navigate(path);
  };

  if (loading) {
    return (
      <LoadingContainer>
        <CircularProgress />
      </LoadingContainer>
    );
  }

  if (error) {
    return (
      <LoadingContainer>
        <Typography color="error">{error}</Typography>
      </LoadingContainer>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          Hi {userName || '[user name]'}, welcome to Tyk AI Studio!
        </Typography>
        <PrimaryButton
          variant="contained"
        >
          Quick start
        </PrimaryButton>
      </TitleBox>
      <ContentBox>
        <DescriptionSection>
          <DescriptionContent sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography variant="headingxLarge" color="text.primary" gutterBottom>
              Explore AI studio
            </Typography>
            <Typography variant="bodyXLargeDefault" color="text.defaultSubdued">
              Empower your team with AI securely and effortlessly.
              Control access, track costs, protect data, and enable fast
              adoption with flexible, user-friendly AI platforms.
            </Typography>
          </DescriptionContent>
          <Box sx={{ width: '50%' }}>
            {/* Placeholder for the play demo section */}
          </Box>
        </DescriptionSection>

        {/* Start building your AI infrastructure section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Start building your AI infrastructure
          </SectionTitle>
          <Stack direction="row" spacing={3} sx={{ flexWrap: { xs: 'wrap', md: 'nowrap' }, mt: 2 }}>
            <Box sx={{ width: { xs: '100%', md: '33.33%' }, mb: { xs: 3, md: 0 } }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add LLM provider', 
                  onClick: () => handleNavigate('/admin/llms/new') 
                }}
                secondaryAction={{ 
                  label: 'Learn more', 
                  onClick: () => {} 
                }}
              > 
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                  <IconBadge iconName="microchip-ai" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Configure <BoldText>Large language Models providers</BoldText>, control usage and cost...
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '33.33%' }, mb: { xs: 3, md: 0 } }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add Data source', 
                  onClick: () => handleNavigate('/admin/datasources/new')
                }}
                secondaryAction={{ 
                  label: 'Learn more', 
                  onClick: () => {} 
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                  <IconBadge iconName="book-sparkles" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Enhance AI responses, add relevant context with <BoldText>Data sources</BoldText>
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '33.33%' } }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add Tool', 
                  onClick: () => handleNavigate('/admin/tools/new')
                }}
                secondaryAction={{ 
                  label: 'Learn more', 
                  onClick: () => {} 
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                  <IconBadge iconName="screwdriver-wrench" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Integrate your systems with <BoldText>Tools</BoldText>, to boost AI capabilities
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
          </Stack>
        </SectionContainer>

        {/* Govern AI section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Govern AI
          </SectionTitle>
          <Stack direction="row" spacing={3} sx={{ flexWrap: { xs: 'wrap', md: 'nowrap' }, mt: 2 }}>
            <Box sx={{ width: { xs: '100%', md: '50%' }, mb: { xs: 3, md: 0 } }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add user', 
                  onClick: () => handleNavigate('/admin/users/new') 
                }}
                secondaryAction={{ 
                  label: 'Learn more', 
                  onClick: () => {} 
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                  <IconBadge iconName="users" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Invite users and control who access to what with <BoldText>RBAC and user groups</BoldText>
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '50%' } }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Learn Filters', 
                  onClick: () => {} 
                }}
                secondaryAction={{ 
                  label: 'Learn Privacy Levels', 
                  onClick: () => {} 
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                  <IconBadge iconName="shield" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Keep data safe with <BoldText>filters and privacy levels</BoldText>, ensuring it's used only with approved LLM providers.
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
          </Stack>
        </SectionContainer>

        {/* Provide AI platforms for your team section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Provide AI platforms for your team
          </SectionTitle>
          <Stack direction="row" spacing={3} sx={{ flexWrap: { xs: 'wrap', md: 'nowrap' }, mt: 2 }}>
            {showAppsCard && (
              <Box sx={{ width: { xs: '100%', md: showChatCard ? '50%' : '100%' }, mb: { xs: 3, md: 0 } }}>
                <BasicCard
                  primaryAction={{
                    label: 'Add Apps',
                    onClick: () => handleNavigate('/admin/apps/new'),
                    disabled: !hasLLMs
                  }}
                  secondaryAction={{
                    label: 'Learn more',
                    onClick: () => {}
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                    <IconBadge iconName="grid-2-plus" />
                    <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                      <BoldText>Apps</BoldText> enable your devs to use any tooling to directly interact with AI through the gateway
                    </CardText>
                  </Box>
                </BasicCard>
              </Box>
            )}
            {showChatCard && (
              <Box sx={{ width: { xs: '100%', md: showAppsCard ? '50%' : '100%' } }}>
                <BasicCard
                  primaryAction={{
                    label: 'Add Chats',
                    onClick: () => handleNavigate('/admin/chats/new'),
                    disabled: !hasLLMs
                  }}
                  secondaryAction={{
                    label: 'Learn more',
                    onClick: () => {}
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, height: '100%', justifyContent: 'center' }}>
                    <IconBadge iconName="message-lines" />
                    <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                      <BoldText>Chats</BoldText> provide an easy-to-use interface for everyone to interact with multiple LLM providers, curated data, and tools
                    </CardText>
                  </Box>
                </BasicCard>
              </Box>
            )}
          </Stack>
        </SectionContainer>
      </ContentBox>
    </>
  );
};

export default Overview;