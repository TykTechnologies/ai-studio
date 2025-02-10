import React, { useState } from 'react';
import {
  Box,
  Typography,
  TextField,
  Button,
  Alert,
  Stack,
  Divider,
  CircularProgress
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import LinkIcon from '@mui/icons-material/Link';
import { extractOperations, extractAuthDetails, detectFormat } from '../utils/specUtils';

const DirectImportSpec = ({ onSpecProvided, error, loading }) => {
  const [specUrl, setSpecUrl] = useState('');
  const [selectedFile, setSelectedFile] = useState(null);
  const [urlError, setUrlError] = useState('');
  const [fileError, setFileError] = useState('');

  const handleUrlChange = (e) => {
    const url = e.target.value;
    setSpecUrl(url);
    setUrlError('');
    
    if (url.trim()) {
      // We'll fetch and process the spec when the URL is submitted
      // For now, just pass the URL
      onSpecProvided({ 
        type: 'url', 
        spec: url,
        // Operations and security details will be extracted after fetching the spec
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
        <Box>
          <Box display="flex" alignItems="center" gap={1} mb={2}>
            <LinkIcon color="primary" />
            <Typography variant="subtitle1">
              Specification URL
            </Typography>
          </Box>
          <TextField
            fullWidth
            placeholder="Enter URL to spec.json or spec.yaml"
            value={specUrl}
            onChange={handleUrlChange}
            error={!!urlError}
            helperText={urlError || "Enter a URL to an OpenAPI specification"}
          />
        </Box>

        <Divider>OR</Divider>

        <Box>
          <Box display="flex" alignItems="center" gap={1} mb={2}>
            <UploadFileIcon color="primary" />
            <Typography variant="subtitle1">
              Upload Specification File
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
        </Stack>
      )}
    </Box>
  );
};

export default DirectImportSpec;
