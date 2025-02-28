import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  TextField,
  Button,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  CircularProgress,
  Paper,
  Divider
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import AddIcon from '@mui/icons-material/Add';
import apiClient from '../../utils/apiClient';

const PromptTemplateManager = ({ chatId, onError }) => {
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState(null);
  const [formData, setFormData] = useState({
    name: '',
    prompt: ''
  });

  const fetchTemplates = async () => {
    if (!chatId) return;

    setLoading(true);
    try {
      // Get the chat data which includes prompt templates
      const response = await apiClient.get(`/chats/${chatId}`);
      const chatData = response.data.data;

      // Extract prompt templates from the chat response
      const templatesData = chatData.attributes.prompt_templates || [];
      setTemplates(templatesData);

      // Also update localStorage for the frontend components
      localStorage.setItem(`chat_${chatId}_templates`, JSON.stringify(templatesData));
    } catch (error) {
      console.error('Error fetching prompt templates:', error);
      onError && onError('Failed to load prompt templates');
      // Set empty array on error
      setTemplates([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTemplates();
  }, [chatId]);

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleOpenDialog = (template = null) => {
    if (template) {
      setEditingTemplate(template);
      setFormData({
        name: template.attributes.name,
        prompt: template.attributes.prompt
      });
    } else {
      setEditingTemplate(null);
      setFormData({
        name: '',
        prompt: ''
      });
    }
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setEditingTemplate(null);
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!formData.name || !formData.prompt) {
      onError && onError('Name and prompt are required');
      return;
    }

    setLoading(true);
    try {
      const payload = {
        data: {
          type: "prompt_template",
          attributes: {
            name: formData.name,
            prompt: formData.prompt,
            chat_id: parseInt(chatId)
          }
        }
      };

      console.log('Submitting prompt template:', payload);

      // Get current templates
      const currentTemplates = [...templates];

      // Update or add template
      let updatedTemplates;
      if (editingTemplate) {
        updatedTemplates = currentTemplates.map(t =>
          t.id === editingTemplate.id ? {
            ...t,
            attributes: {
              name: formData.name,
              prompt: formData.prompt,
              chat_id: parseInt(chatId)
            }
          } : t
        );
      } else {
        // Add new template with temporary ID
        const newTemplate = {
          id: `temp_${Date.now()}`,
          type: "prompt_templates",
          attributes: {
            name: formData.name,
            prompt: formData.prompt,
            chat_id: parseInt(chatId)
          }
        };
        updatedTemplates = [...currentTemplates, newTemplate];
      }

      // Update templates on the server
      await apiClient.patch(`/chats/${chatId}/prompt-templates`, {
        templates: updatedTemplates.map(t => ({
          id: t.id.startsWith('temp_') ? 0 : parseInt(t.id),
          name: t.attributes.name,
          prompt: t.attributes.prompt
        }))
      });

      // Fetch updated templates
      fetchTemplates();
      handleCloseDialog();
    } catch (error) {
      console.error('Error saving prompt template:', error);
      onError && onError('Failed to save prompt template: ' + (error.response?.data?.errors?.[0]?.detail || error.message));
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (templateId) => {
    if (!window.confirm('Are you sure you want to delete this prompt template?')) {
      return;
    }

    setLoading(true);
    try {
      const updatedTemplates = templates.filter(t => t.id !== templateId);

      await apiClient.patch(`/chats/${chatId}/prompt-templates`, {
        templates: updatedTemplates.map(t => ({
          id: parseInt(t.id),
          name: t.attributes.name,
          prompt: t.attributes.prompt
        }))
      });

      fetchTemplates();
    } catch (error) {
      console.error('Error deleting prompt template:', error);
      onError && onError('Failed to delete prompt template');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h6">Prompt Templates</Typography>
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={() => handleOpenDialog()}
          disabled={loading}
        >
          Add Template
        </Button>
      </Box>

      {loading && templates.length === 0 ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress size={24} />
        </Box>
      ) : templates.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center' }}>
          <Typography color="textSecondary">
            No prompt templates available. Add templates to help users start conversations.
          </Typography>
        </Paper>
      ) : (
        <List component={Paper}>
          {templates.map((template) => (
            <React.Fragment key={template.id}>
              <ListItem>
                <ListItemText
                  primary={template.attributes.name}
                  secondary={
                    <Typography
                      component="span"
                      variant="body2"
                      color="textSecondary"
                      sx={{
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis'
                      }}
                    >
                      {template.attributes.prompt}
                    </Typography>
                  }
                />
                <ListItemSecondaryAction>
                  <IconButton edge="end" onClick={() => handleOpenDialog(template)} disabled={loading}>
                    <EditIcon />
                  </IconButton>
                  <IconButton edge="end" onClick={() => handleDelete(template.id)} disabled={loading}>
                    <DeleteIcon />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
              <Divider component="li" />
            </React.Fragment>
          ))}
        </List>
      )}

      <Dialog open={dialogOpen} onClose={handleCloseDialog} maxWidth="md" fullWidth scroll="paper">
        <DialogTitle>
          {editingTemplate ? 'Edit Prompt Template' : 'Add Prompt Template'}
        </DialogTitle>
        <DialogContent dividers>
          <TextField
            autoFocus
            margin="dense"
            name="name"
            label="Template Name"
            type="text"
            fullWidth
            value={formData.name}
            onChange={handleInputChange}
            disabled={loading}
            sx={{ mb: 2 }}
          />
          <TextField
            margin="dense"
            name="prompt"
            label="Prompt"
            multiline
            rows={4}
            fullWidth
            value={formData.prompt}
            onChange={handleInputChange}
            disabled={loading}
            placeholder="Enter the prompt template text..."
            helperText="This is the template that will be inserted into the chat input when selected by a user."
            sx={{
              '& textarea': {
                maxHeight: '100px',
                overflowY: 'auto'
              }
            }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog} disabled={loading}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            color="primary"
            disabled={loading || !formData.name.trim() || !formData.prompt.trim()}
          >
            {loading ? <CircularProgress size={24} /> : 'Save'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default PromptTemplateManager;
