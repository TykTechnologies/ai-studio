import React, { useState } from 'react';
import {
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Typography,
  Box,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import NamespaceSelector from './NamespaceSelector';

const EdgeAvailabilitySection = ({
  value = [],
  onChange,
  label = 'Edge Availability',
  helperText = 'Select which edge namespaces this configuration should be available to. Leave empty for global availability.',
  error = false,
  disabled = false,
  required = false,
  defaultExpanded = false,
  ...props
}) => {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const handleExpansionChange = (event, isExpanded) => {
    setExpanded(isExpanded);
  };

  return (
    <Accordion 
      expanded={expanded} 
      onChange={handleExpansionChange}
      sx={{ 
        mt: 2,
        boxShadow: 'none',
        border: '1px solid',
        borderColor: 'divider',
        '&:before': {
          display: 'none',
        },
        '&.Mui-expanded': {
          margin: '16px 0 0 0',
        }
      }}
    >
      <AccordionSummary 
        expandIcon={<ExpandMoreIcon />}
        sx={{ 
          backgroundColor: 'grey.50',
          minHeight: 48,
          '&.Mui-expanded': {
            minHeight: 48,
          },
          '& .MuiAccordionSummary-content': {
            margin: '12px 0',
            '&.Mui-expanded': {
              margin: '12px 0',
            }
          }
        }}
      >
        <Typography variant="subtitle1" fontWeight="medium">
          {label}
          {required && <span style={{ color: 'red' }}> *</span>}
        </Typography>
      </AccordionSummary>
      <AccordionDetails sx={{ pt: 2 }}>
        <Box>
          <NamespaceSelector
            value={value}
            onChange={onChange}
            label="Available Namespaces"
            helperText={helperText}
            error={error}
            disabled={disabled}
            required={required}
            onlyWithEdges={true}
            {...props}
          />
        </Box>
      </AccordionDetails>
    </Accordion>
  );
};

export default EdgeAvailabilitySection;