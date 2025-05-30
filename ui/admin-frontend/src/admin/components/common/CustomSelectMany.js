import React from 'react';
import {
  MenuItem,
  Typography,
  Box,
  InputLabel,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import ClearIcon from '@mui/icons-material/Clear';
import { StyledFormControl, StyledSelectMany, StyledChip } from '../../styles/sharedStyles';
import { getColorsForVariant } from '../groups/components/styles';

const CustomSelectMany = ({
  label,
  value = [],
  onChange,
  options,
  required = false,
  error = false,
  helperText = '',
  renderOption,
  chipVariant = 'default',
  ...props
}) => {
  const theme = useTheme();
  const safeValue = value || [];
  const selectValues = safeValue.map(val =>
    val ? val.value : val
  );

  const handleChange = (event) => {    
    const {
      target: { value: newValue },
    } = event;
    
    const selectedValues = typeof newValue === 'string' ? newValue.split(',') : newValue;
    const newSelectedObjects = selectedValues.map(val => {
      const option = options.find(opt => String(opt.value) === String(val));

      return option || { value: val, label: val };
    });
    
    onChange(newSelectedObjects);
  };

  const { bgColor, textColor } = getColorsForVariant(theme, chipVariant);

  return (
    <Box sx={{ width: '100%' }}>
      <StyledFormControl fullWidth required={required} error={error ? "true" : undefined}>
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
                  const valueObj = safeValue.find(v => v && String(v.value) === String(val));
                  const displayLabel = option ? option.label : (valueObj ? valueObj.label : String(val));
                  
                  return (
                    <StyledChip
                      key={val}
                      label={displayLabel}
                      bgColor={bgColor}
                      textColor={textColor}
                      deleteIcon={<ClearIcon />}
                      onMouseDown={(e) => e.stopPropagation()}
                      onDelete={(e) => {
                        e.stopPropagation();

                        const newSelected = selected.filter(item => item !== val);

                        const newSelectedObjects = newSelected.map(selVal => {
                          const opt = options.find(o => String(o.value) === String(selVal));
                          const valueObj = safeValue.find(v => v && String(v.value) === String(selVal));

                          return opt || valueObj || { value: selVal, label: selVal };
                        });

                        onChange(newSelectedObjects);
                        const chipElement = e.target.closest('.MuiChip-root');
                        chipElement?.blur();
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
          {options?.map((option) => {
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