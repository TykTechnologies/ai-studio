import React from 'react';
import {
  MenuItem,
  Typography,
  Box,
} from '@mui/material';
import { StyledFormControl, StyledSelect } from '../../styles/sharedStyles';


const CustomSelect = ({ 
  label, 
  value, 
  onChange, 
  options, 
  required = false,
  error = false,
  helperText = '',
  renderOption,
  ...props 
}) => {
  return (
    <Box sx={{ width: '100%' }}>
      <StyledFormControl fullWidth required={required} error={error}>
        <StyledSelect
          value={value}
          onChange={onChange}
          label={label}
          MenuProps={{
            sx: {
              zIndex: 9999,
              '& .MuiPaper-root': {
                borderRadius: '8px'
              }
            }
          }}
          {...props}
        >
          {options.map((option) => (
            <MenuItem
              key={option.value}
              value={option.value}
              sx={{
                '&.Mui-selected': {
                  backgroundColor: theme => theme.palette.background.surfaceNeutralHover,
                  '&:hover': {
                    backgroundColor: theme => theme.palette.background.surfaceNeutralHover
                  }
                }
              }}
            >
              {renderOption ? renderOption(option) : option.label}
            </MenuItem>
          ))}
        </StyledSelect>
        {helperText && (
          <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mt: 0.5 }}>
            {helperText}
          </Typography>
        )}
      </StyledFormControl>
    </Box>
  );
};

export default CustomSelect;