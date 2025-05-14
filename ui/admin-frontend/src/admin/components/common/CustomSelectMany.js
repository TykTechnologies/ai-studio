import React from 'react';
import {
  MenuItem,
  Typography,
  Box,
  Chip,
  InputLabel,
} from '@mui/material';
import ClearIcon from '@mui/icons-material/Clear';
import { StyledFormControl, StyledSelectMany } from '../../styles/sharedStyles';

const CustomSelectMany = ({
  label,
  value = [],
  onChange,
  options,
  required = false,
  error = false,
  helperText = '',
  renderOption,
  ...props
}) => {
  const selectValues = value.map(val =>
    val ? val.value : val
  );

  const handleChange = (event) => {    
    const {
      target: { value: newValue },
    } = event;
    
    const selectedValues = typeof newValue === 'string' ? newValue.split(',') : newValue;
    const newSelectedObjects = selectedValues.map(val => {
      const option = options.find(opt => String(opt.value) === String(val));

      return option || { value: val, label: String(val) };
    });
    
    onChange(newSelectedObjects);
  };

  return (
    <Box sx={{ width: '100%' }}>
      <StyledFormControl fullWidth required={required} error={error}>
        <InputLabel>{label}</InputLabel>
        <StyledSelectMany
          multiple
          value={selectValues}
          onChange={handleChange}
          label={label}
          renderValue={(selected) => {
            return (
              <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                {selected?.map((val) => {
                  const option = options?.find(opt => String(opt.value) === String(val));
                  return (
                    <Chip
                      key={val}
                      label={option ? option.label : String(val)}
                      sx={{
                        bgcolor: theme => theme.palette.background.buttonPrimaryOutlineHover,
                        borderRadius: '6px',
                        maxHeight: 'fit-content',
                        '& .MuiChip-label': {
                          color: theme => theme.palette.text.defaultSubdued,
                          padding: '2px 6px',
                          marginRight: '6px',
                        },
                        '& .MuiChip-deleteIcon': {
                          color: theme => theme.palette.text.defaultSubdued,
                          '&:hover': {
                            color: theme => theme.palette.text.defaultSubdued,
                          }
                        },
                      }}
                      deleteIcon={<ClearIcon />}
                      onMouseDown={(e) => e.stopPropagation()}
                      onDelete={(e) => {
                        e.stopPropagation();

                        const newSelected = selected.filter(item => item !== val);

                        const newSelectedObjects = newSelected.map(selVal => {
                          const opt = options.find(o => String(o.value) === String(selVal));

                          return opt || { value: selVal, label: String(selVal) };
                        });

                        onChange(newSelectedObjects);
                        e.target.closest('.MuiChip-root').blur();
                      }}
                    />
                  );
                })}
              </Box>
            );
          }}
          MenuProps={{
            PaperProps: {
              style: {
                maxHeight: 250,
              },
            },
          }}
          {...props}
        >
          {options.map((option) => {
            const isSelected = selectValues.includes(option.value);

            return (
              <MenuItem
                key={option.value}
                value={option.value}
                selected={isSelected}
                sx={{
                  padding: '8px 16px',
                  '&.Mui-selected': {
                    backgroundColor: theme => `${theme.palette.background.surfaceNeutralHover} !important`,
                    '&:hover': {
                      backgroundColor: theme => `${theme.palette.background.surfaceNeutralHover} !important`
                    }
                  },
                  '&.MuiMenuItem-root.Mui-selected': {
                    backgroundColor: theme => `${theme.palette.background.surfaceNeutralHover} !important`,
                  },
                  '&.MuiMenuItem-root.Mui-selected:hover': {
                    backgroundColor: theme => `${theme.palette.background.surfaceNeutralHover} !important`,
                  }
                }}
              >
                {renderOption ? renderOption(option) : option.label}
              </MenuItem>
            );
          })}
        </StyledSelectMany>
        {helperText && (
          <Typography variant="bodySmallDefault" color="text.defaultSubdued" sx={{ mt: 0.5 }}>
            {helperText}
          </Typography>
        )}
      </StyledFormControl>
    </Box>
  );
};

export default CustomSelectMany;