import React, { useState, useEffect } from 'react';
import {
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  Box,
  OutlinedInput,
  CircularProgress,
  Typography,
  Checkbox,
  ListItemText,
} from '@mui/material';
import useNamespaces from '../../hooks/useNamespaces';

const NamespaceSelector = ({
  value = [],
  onChange,
  label = 'Namespaces',
  required = false,
  error = false,
  helperText = '',
  disabled = false,
  onlyWithEdges = true,
  ...props
}) => {
  const { 
    namespaces, 
    loading, 
    error: namespacesError,
    getAvailableNamespaces,
    parseNamespaceString,
    formatNamespaceArray 
  } = useNamespaces();

  const [selectedNamespaces, setSelectedNamespaces] = useState([]);

  // Convert value to array format for internal use
  useEffect(() => {
    if (typeof value === 'string') {
      setSelectedNamespaces(parseNamespaceString(value));
    } else if (Array.isArray(value)) {
      setSelectedNamespaces(value);
    } else {
      setSelectedNamespaces([]);
    }
  }, [value, parseNamespaceString]);

  // Get the namespaces to show based on onlyWithEdges prop
  const availableNamespaces = onlyWithEdges ? getAvailableNamespaces() : namespaces;

  const handleChange = (event) => {
    const selectedValues = event.target.value;
    setSelectedNamespaces(selectedValues);
    
    // Call onChange with formatted string or array based on what parent expects
    if (onChange) {
      if (typeof value === 'string') {
        onChange(formatNamespaceArray(selectedValues));
      } else {
        onChange(selectedValues);
      }
    }
  };

  const handleDelete = (namespaceToDelete) => {
    const updatedNamespaces = selectedNamespaces.filter(ns => ns !== namespaceToDelete);
    setSelectedNamespaces(updatedNamespaces);
    
    if (onChange) {
      if (typeof value === 'string') {
        onChange(formatNamespaceArray(updatedNamespaces));
      } else {
        onChange(updatedNamespaces);
      }
    }
  };

  if (loading) {
    return (
      <Box display="flex" alignItems="center" gap={1}>
        <CircularProgress size={20} />
        <Typography variant="body2">Loading namespaces...</Typography>
      </Box>
    );
  }

  if (namespacesError) {
    return (
      <Typography variant="body2" color="error">
        Error loading namespaces: {namespacesError}
      </Typography>
    );
  }

  return (
    <FormControl fullWidth required={required} error={error} disabled={disabled} {...props}>
      <InputLabel>{label}</InputLabel>
      <Select
        multiple
        value={selectedNamespaces}
        onChange={handleChange}
        input={<OutlinedInput label={label} />}
        renderValue={(selected) => (
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
            {selected.map((namespace) => (
              <Chip
                key={namespace}
                label={namespace}
                size="small"
                onDelete={() => handleDelete(namespace)}
                onMouseDown={(event) => event.stopPropagation()}
              />
            ))}
          </Box>
        )}
        MenuProps={{
          PaperProps: {
            style: {
              maxHeight: 300,
            },
          },
        }}
      >
        {availableNamespaces.map((namespace) => (
          <MenuItem key={namespace.name} value={namespace.name}>
            <Checkbox checked={selectedNamespaces.includes(namespace.name)} />
            <ListItemText 
              primary={namespace.name}
              secondary={`${namespace.edgeCount} edges, ${namespace.llmCount} LLMs`}
            />
          </MenuItem>
        ))}
      </Select>
      {helperText && (
        <Typography variant="caption" color={error ? 'error' : 'textSecondary'} sx={{ mt: 0.5, ml: 1.5 }}>
          {helperText}
        </Typography>
      )}
    </FormControl>
  );
};

export default NamespaceSelector;