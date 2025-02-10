import React, { useState } from 'react';
import {
  Box,
  Typography,
  TextField,
  Button,
  Alert,
  Stack,
  CircularProgress,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormControl,
  FormLabel
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import LinkIcon from '@mui/icons-material/Link';
import CodeIcon from '@mui/icons-material/Code';
import { extractOperations, extractAuthDetails, detectFormat } from '../utils/specUtils';

const IMPORT_METHODS = {
  URL: 'url',
  FILE: 'file',
  PASTE: 'paste'
};

const DirectImportSpec = ({ onSpecProvided, error, loading }) => {
  const [importMethod, setImportMethod] = useState('');
  const [specUrl, setSpecUrl] = useState('');
  const [selectedFile, setSelectedFile] = useState(null);
  const [pastedSpec, setPastedSpec] = useState('');
  const [urlError, setUrlError] = useState('');
  const [fileError, setFileError] = useState('');
  const [pasteError, setPasteError] = useState('');

  const handleMethodChange = (event) => {
    const method = event.target.value;
    setImportMethod(method);
    // Reset errors and values when switching methods
    setUrlError('');
    setFileError('');
    setPasteError('');
    setSpecUrl('');
    setSelectedFile(null);
    setPastedSpec('');
    onSpecProvided(null);
  };

  const handleUrlChange = (e) => {
    const url = e.target.value;
    setSpecUrl(url);
    setUrlError('');
    
    if (url.trim()) {
      onSpecProvided({ 
        type: 'url', 
        spec: url,
        operations: [],
        security_details: { type: '', name: '', in: '' }
      });
    } else {
      onSpecProvided(null);
    }
  };

  const handleFileChange = (event) => {
    const file = event.target.files[0];
    if (!file) {
      setSelectedFile(null);
      setFileError('');
      onSpecProvided(null);
      return;
    }

    const reader = new FileReader();
    reader.onload = async (e) => {
      try {
        const content = e.target.result;
        const format = detectFormat(content, file.name);
        const operations = extractOperations(content, format);
        const authDetails = extractAuthDetails(content, format);
        
        setSelectedFile(file);
        setFileError('');
        onSpecProvided({ 
          type: 'file', 
          file,
          operations,
          security_details: authDetails
        });
      } catch (err) {
        setFileError(err.message);
        onSpecProvided(null);
      }
    };
    reader.readAsText(file);
  };

  const handlePasteChange = (e) => {
    const content = e.target.value;
    setPastedSpec(content);
    setPasteError('');

    if (content.trim()) {
      try {
        const format = detectFormat(content);
        const operations = extractOperations(content, format);
        const authDetails = extractAuthDetails(content, format);

        onSpecProvided({
          type: 'paste',
          spec: content,
          operations,
          security_details: authDetails
        });
      } catch (err) {
        setPasteError(err.message);
        onSpecProvided(null);
      }
    } else {
      onSpecProvided(null);
    }
  };

  const renderImportMethod = () => {
    switch (importMethod) {
      case IMPORT_METHODS.URL:
        return (
          <Box>
            <Box display="flex" alignItems="center" gap={1} mb={2}>
              <LinkIcon color="primary" />
              <Typography variant="subtitle1">
                Specification URL
              </Typography>
            </Box>
            <TextField
              fullWidth
              placeholder="Enter URL to OpenAPI specification"
              value={specUrl}
              onChange={handleUrlChange}
              error={!!urlError}
              helperText={urlError || "Supports both JSON and YAML formats (.json, .yaml, .yml)"}
            />
          </Box>
        );

      case IMPORT_METHODS.FILE:
        return (
          <Box>
            <Box display="flex" alignItems="center" gap={1} mb={2}>
              <UploadFileIcon color="primary" />
              <Typography variant="subtitle1">
                Upload Specification File
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Supports both JSON and YAML formats
              </Typography>
            </Box>
            <Box>
              <input
                accept=".json,.yaml,.yml"
                style={{ display: 'none' }}
                id="spec-file-upload"
                type="file"
                onChange={handleFileChange}
              />
              <label htmlFor="spec-file-upload">
                <Button
                  variant="outlined"
                  component="span"
                  startIcon={<UploadFileIcon />}
                >
                  Choose File
                </Button>
              </label>
              {selectedFile && (
                <Typography variant="body2" sx={{ mt: 1, color: 'text.secondary' }}>
                  Selected file: {selectedFile.name}
                </Typography>
              )}
              {fileError && (
                <Typography color="error" variant="body2" sx={{ mt: 1 }}>
                  {fileError}
                </Typography>
              )}
            </Box>
          </Box>
        );

      case IMPORT_METHODS.PASTE:
        return (
          <Box>
            <Box display="flex" alignItems="center" gap={1} mb={2}>
              <CodeIcon color="primary" />
              <Typography variant="subtitle1">
                Paste Specification
              </Typography>
            </Box>
            <TextField
              fullWidth
              multiline
              rows={10}
              placeholder="Paste your OpenAPI specification here"
              value={pastedSpec}
              onChange={handlePasteChange}
              error={!!pasteError}
              helperText={pasteError || "Supports both JSON and YAML formats - format will be auto-detected"}
            />
          </Box>
        );

      default:
        return null;
    }
  };

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Import OpenAPI Specification
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {loading ? (
        <Box display="flex" justifyContent="center" my={4}>
          <CircularProgress />
        </Box>
      ) : (
        <Stack spacing={4}>
          <FormControl component="fieldset">
            <FormLabel component="legend">Select Import Method</FormLabel>
            <RadioGroup
              value={importMethod}
              onChange={handleMethodChange}
            >
              <FormControlLabel 
                value={IMPORT_METHODS.URL} 
                control={<Radio />} 
                label="Import from URL"
              />
              <FormControlLabel 
                value={IMPORT_METHODS.FILE} 
                control={<Radio />} 
                label="Upload File"
              />
              <FormControlLabel 
                value={IMPORT_METHODS.PASTE} 
                control={<Radio />} 
                label="Paste Content"
              />
            </RadioGroup>
          </FormControl>

          {importMethod && renderImportMethod()}
        </Stack>
      )}
    </Box>
  );
};

export default DirectImportSpec;
