import React, { useState, useEffect, useRef, useCallback, memo } from 'react';
import {
  Box,
  Typography,
  Button,
  CircularProgress,
  Stack,
  Paper
} from '@mui/material';

const PromptTemplateSelector = memo(({ chatId, onSelectTemplate, disabled, sx = {} }) => {
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(false);
  // Use refs to prevent unnecessary re-renders
  const templatesRef = useRef(templates);

  // Update the ref when templates change
  useEffect(() => {
    templatesRef.current = templates;
  }, [templates]);

  // Load templates from localStorage (which are stored from the chat response)
  useEffect(() => {
    if (!chatId) return;

    setLoading(true);
    try {
      // Get templates from localStorage
      const storedTemplates = localStorage.getItem(`chat_${chatId}_templates`);

      if (storedTemplates) {
        const templatesData = JSON.parse(storedTemplates);
        setTemplates(Array.isArray(templatesData) ? templatesData : []);
        console.log('Templates loaded from localStorage:', templatesData);
      } else {
        console.log('No templates found in localStorage for chat:', chatId);
        setTemplates([]);
      }
    } catch (error) {
      console.error('Error loading prompt templates:', error);
      setTemplates([]);
    } finally {
      setLoading(false);
    }
  }, [chatId]);

  const handleSelectTemplate = useCallback((template) => {
    console.log('Selected template:', template);
    if (template && template.attributes && template.attributes.prompt) {
      onSelectTemplate(template.attributes.prompt);
    } else {
      console.error('Invalid template format:', template);
    }
  }, [onSelectTemplate]);

  // Don't render anything if there are no templates
  if (templates.length === 0 && !loading) {
    return null;
  }

  return (
    <Box sx={{
      width: '100%',
      display: 'flex',
      justifyContent: 'center',
      ...sx
    }}>
      <Box sx={{
        maxWidth: '800px',
        width: '100%',
        p: 0.5,
        mt: 0,
      }}>
        <Stack
          direction="row"
          spacing={1}
          sx={{
            flexWrap: 'wrap',
            gap: 1,
            justifyContent: 'center'
          }}
        >
          {loading ? (
            <CircularProgress size={24} />
          ) : (
            templates.map((template) => (
              <Paper
                key={template.id}
                elevation={0}
                sx={{
                  p: 1.5,
                  cursor: 'pointer',
                  borderRadius: '25px',
                  border: '1px solid rgba(0, 0, 0, 0.12)',
                  backgroundColor: 'rgba(0, 0, 0, 0.0)',
                  '&:hover': {
                    backgroundColor: 'rgba(0, 0, 0, 0.03)',
                  },
                }}
                onClick={() => handleSelectTemplate(template)}
              >
                <Typography variant="body2" color="text.secondary">
                  {template.attributes.name}
                </Typography>
              </Paper>
            ))
          )}
        </Stack>
      </Box>
    </Box >
  );
});

export default PromptTemplateSelector;
