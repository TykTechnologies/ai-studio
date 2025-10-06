import React, { useState, useEffect } from 'react';
import {
  Typography,
  Grid,
  Card,
  CardContent,
  CardActions,
  CircularProgress,
  Box,
  Button,
  Alert,
} from '@mui/material';
import { useNavigate } from 'react-router-dom';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import agentService from '../services/agentService';
import {
  TitleBox,
  ContentBox,
} from '../../admin/styles/sharedStyles';

const AgentDashboard = () => {
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [user, setUser] = useState(null);
  const navigate = useNavigate();

  useEffect(() => {
    fetchAgents();
  }, []);

  const fetchAgents = async () => {
    try {
      setLoading(true);
      const data = await agentService.listAccessibleAgents();

      // Filter to only active agents
      const activeAgents = data.filter(agent => agent.isActive);
      setAgents(activeAgents);

      // Get user info (optional - for greeting)
      try {
        const userResponse = await import('../../admin/utils/pubClient').then(
          module => module.default.get('/common/me')
        );
        setUser(userResponse.data.attributes);
      } catch (err) {
        console.error('Error fetching user info:', err);
      }
    } catch (err) {
      console.error('Error fetching agents:', err);
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleStartAgent = (agentId) => {
    navigate(`/agent/${agentId}`);
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Agents</Typography>
      </TitleBox>
      <ContentBox>
        {user && (
          <Box sx={{ p: 7 }}>
            <Typography variant="headingXLarge">
              Hi {user.name}, welcome!
            </Typography>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mt: 1 }}>
              Start a conversation with an AI agent
            </Typography>
          </Box>
        )}

        {error && (
          <Box sx={{ p: 7, pt: user ? 0 : 7 }}>
            <Alert severity="error">{error}</Alert>
          </Box>
        )}

        {agents.length > 0 ? (
          <Box sx={{ p: 7, pt: user || error ? 0 : 7 }}>
            <Typography variant="headingLarge" gutterBottom>
              Available Agents
            </Typography>
            <Grid container spacing={2} sx={{ mt: 1 }}>
              {agents
                .sort((a, b) => a.name.localeCompare(b.name))
                .map((agent) => (
                  <Grid item xs={12} sm={6} md={4} lg={3} key={agent.id}>
                    <Card
                      sx={{
                        height: '100%',
                        display: 'flex',
                        flexDirection: 'column',
                        justifyContent: 'space-between',
                        transition: 'transform 0.2s, box-shadow 0.2s',
                        '&:hover': {
                          transform: 'translateY(-4px)',
                          boxShadow: 3,
                        },
                      }}
                    >
                      <CardContent>
                        <Box>
                          <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                            <SmartToyIcon
                              sx={{
                                mr: 1,
                                fontSize: 24,
                                color: 'primary.main',
                              }}
                            />
                            <Typography variant="headingMedium" component="div" noWrap>
                              {agent.name}
                            </Typography>
                          </Box>
                          {agent.description && (
                            <Typography
                              variant="bodyLargeDefault"
                              color="text.defaultSubdued"
                              sx={{
                                display: '-webkit-box',
                                WebkitLineClamp: 3,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                mt: 2,
                                minHeight: '4.5em',
                              }}
                            >
                              {agent.description}
                            </Typography>
                          )}
                        </Box>
                      </CardContent>
                      <CardActions
                        sx={{
                          justifyContent: 'flex-end',
                          p: 2,
                          mt: 2,
                          borderTop: (theme) =>
                            `1px solid ${theme.palette.border.neutralDefaultSubdued}`,
                        }}
                      >
                        <Button onClick={() => handleStartAgent(agent.id)}>
                          Start
                        </Button>
                      </CardActions>
                    </Card>
                  </Grid>
                ))}
            </Grid>
          </Box>
        ) : (
          <Box sx={{ p: 7, pt: user || error ? 0 : 7, textAlign: 'center' }}>
            <SmartToyIcon sx={{ fontSize: 64, color: 'text.secondary', mb: 2 }} />
            <Typography variant="headingLarge" gutterBottom>
              No agents available
            </Typography>
            <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
              Contact your administrator to set up AI agents
            </Typography>
          </Box>
        )}
      </ContentBox>
    </>
  );
};

export default AgentDashboard;
