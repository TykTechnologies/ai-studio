import React from 'react';
import { Typography, Box, styled, CircularProgress } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { TitleBox, ContentBox, PrimaryButton } from '../styles/sharedStyles';
import useOverviewData from '../hooks/useOverviewData';
import WarningBanner from '../components/common/WarningBanner';
import useQuickStart from '../hooks/useQuickStart';
import BasicCard from '../components/common/BasicCard';
import IconBadge from '../components/common/IconBadge';
import { createDocsLinkHandler } from '../utils/docsLinkUtils';
import VideoPlayer from '../components/common/VideoPlayer';
import { QuickStartContainer } from '../components/wizards/quick-start';

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
  width: '100%',
  boxSizing: 'border-box',
  overflowX: 'hidden',
}));

const CardText = styled(Typography)(({ theme }) => ({
  marginBottom: theme.spacing(1),
  wordBreak: 'break-word',
  maxWidth: '100%',
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
  width: '100%',
  boxSizing: 'border-box',
  overflowX: 'hidden',
}));

const Overview = () => {
  const { userName, features, hasLLMs, getDocsLink, loading, error, licenseDaysLeft } = useOverviewData();
  const quickStartState = useQuickStart();
  const { setShowQuickStart } = quickStartState;
  const navigate = useNavigate();

  const showChatCard = features?.feature_chat;
  const showAppsCard = features?.feature_gateway || features?.feature_portal;

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
      <QuickStartContainer quickStartState={quickStartState} />
      <TitleBox top="64px">
        <Typography variant="headingXLarge">
          Hi {userName || '[user name]'}, welcome to Tyk AI Studio!
        </Typography>
        <PrimaryButton
          variant="contained"
          onClick={() => {
            setShowQuickStart(true);
          }}
        >
          Quick start
        </PrimaryButton>
      </TitleBox>
      <ContentBox>
        {licenseDaysLeft && (
          <WarningBanner
            title={`You have ${licenseDaysLeft} days left in your trial.`}
            buttonText="Get in touch"
            onButtonClick={createDocsLinkHandler(getDocsLink, 'get_intouch_form')}
            showCloseButton={false}
            sx={{ mb: 3 }}
          />
        )}
        <DescriptionSection>
          <Box sx={{
            display: 'flex',
            flexDirection: { xs: 'column', md: 'row' },
            width: '100%',
            boxSizing: 'border-box',
            alignItems: 'center',
            gap: 4,
          }}>
            <Box sx={{
              width: { xs: '100%', md: '50%' },
              display: 'flex',
              flexDirection: 'column',
              gap: 2,
              boxSizing: 'border-box',
            }}>
              <Typography variant="headingxLarge" color="text.primary" gutterBottom>
                Explore AI studio
              </Typography>
              <Typography variant="bodyXLargeDefault" color="text.defaultSubdued" sx={{ wordBreak: 'break-word' }}>
                Empower your team with AI securely and effortlessly.
                Control access, track costs, protect data, and enable fast
                adoption with flexible, user-friendly AI platforms.
              </Typography>
            </Box>
            <Box sx={{
              width: { xs: '100%', md: '50%' },
              boxSizing: 'border-box',
              overflow: 'hidden',
              borderRadius: '8px'
            }}>
              <VideoPlayer
                url={getDocsLink('demo_video')}
                thumbnailUrl={getDocsLink('demo_video_hqdefault')}
                sx={{
                  width: '100%',
                  height: 'auto',
                  paddingTop: '56.25%', // Standard 16:9 aspect ratio
                  position: 'relative'
                }}
              />
            </Box>
          </Box>
        </DescriptionSection>

        {/* Start building your AI infrastructure section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Start building your AI infrastructure
          </SectionTitle>
          <Box sx={{ 
              display: 'flex',
              flexWrap: 'wrap',
              gap: 3,
              mt: 2,
              width: '100%',
              boxSizing: 'border-box'
            }}>
            <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add LLM provider', 
                  onClick: () => navigate('/admin/llms/new') 
                }}
                secondaryAction={{
                  label: 'Learn more',
                  onClick: createDocsLinkHandler(getDocsLink, 'llm_providers')
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
                  <IconBadge iconName="microchip-ai" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Configure <BoldText>Large language Models providers</BoldText>, control usage and cost...
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add Data source', 
                  onClick: () => navigate('/admin/datasources/new')
                }}
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
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Enhance AI responses, add relevant context with <BoldText>Data sources</BoldText>
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add Tool', 
                  onClick: () => navigate('/admin/tools/new')
                }}
                secondaryAction={{
                  label: 'Learn more',
                  onClick: createDocsLinkHandler(getDocsLink, 'tools')
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
                  <IconBadge iconName="screwdriver-wrench" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Integrate your systems with <BoldText>Tools</BoldText>, to boost AI capabilities
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
          </Box>
        </SectionContainer>

        {/* Govern AI section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Govern AI
          </SectionTitle>
          <Box sx={{ 
              display: 'flex',
              flexWrap: 'wrap',
              gap: 3,
              mt: 2,
              width: '100%',
              boxSizing: 'border-box'
            }}>
            <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Add user', 
                  onClick: () => navigate('/admin/users/new') 
                }}
                secondaryAction={{
                  label: 'Learn more',
                  onClick: createDocsLinkHandler(getDocsLink, 'rbac_user_groups')
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
                  <IconBadge iconName="users" />
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Invite users and control who access to what with <BoldText>RBAC and user groups</BoldText>
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
            <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
              <BasicCard
                primaryAction={{ 
                  label: 'Learn Filters', 
                  onClick: createDocsLinkHandler(getDocsLink, 'filters')
                }}
                secondaryAction={{
                  label: 'Learn Privacy Levels',
                  onClick: createDocsLinkHandler(getDocsLink, 'privacy_levels')
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
                  <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                    Keep data safe with <BoldText>filters and privacy levels</BoldText>, ensuring it's used only with approved LLM providers.
                  </CardText>
                </Box>
              </BasicCard>
            </Box>
          </Box>
        </SectionContainer>

        {/* Provide AI platforms for your team section */}
        <SectionContainer>
          <SectionTitle variant="headingMedium">
            Provide AI platforms for your team
          </SectionTitle>
          <Box sx={{ 
              display: 'flex',
              flexWrap: 'wrap',
              gap: 3,
              mt: 2,
              width: '100%',
              boxSizing: 'border-box'
            }}>
            {showAppsCard && (
              <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
                <BasicCard
                  primaryAction={{
                    label: 'Add Apps',
                    onClick: () => navigate('/admin/apps/new'),
                    disabled: !hasLLMs
                  }}
                  secondaryAction={{
                    label: 'Learn more',
                    onClick: createDocsLinkHandler(getDocsLink, 'apps')
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
                    <IconBadge iconName="grid-2-plus" />
                    <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                      <BoldText>Apps</BoldText> enable your devs to use any tooling to directly interact with AI through the gateway
                    </CardText>
                  </Box>
                </BasicCard>
              </Box>
            )}
            {showChatCard && (
              <Box sx={{ 
                flex: '1 0 370px',
                maxWidth: '100%',
                boxSizing: 'border-box'
              }}>
                <BasicCard
                  primaryAction={{
                    label: 'Add Chats',
                    onClick: () => navigate('/admin/chats/new'),
                    disabled: !hasLLMs
                  }}
                  secondaryAction={{
                    label: 'Learn more',
                    onClick: createDocsLinkHandler(getDocsLink, 'chats')
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
                    <IconBadge iconName="message-lines" />
                    <CardText variant="bodyXLargeMedium" sx={{ mb: 0 }}>
                      <BoldText>Chats</BoldText> provide an easy-to-use interface for everyone to interact with multiple LLM providers, curated data, and tools
                    </CardText>
                  </Box>
                </BasicCard>
              </Box>
            )}
          </Box>
        </SectionContainer>
      </ContentBox>
    </>
  );
};

export default Overview;