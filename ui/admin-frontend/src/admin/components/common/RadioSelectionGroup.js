import React from 'react';
import {
  Typography,
  Divider,
  RadioGroup,
  FormControlLabel,
  Radio,
  FormControl,
} from '@mui/material';

const RadioSelectionGroup = ({
  options,
  value,
  onChange,
  renderContent,
}) => {
  return (
    <FormControl component="fieldset" sx={{ width: '100%', mb: 2 }}>
      <RadioGroup
        name="selection"
        value={value}
        onChange={onChange}
      >
        {options.map((option, index) => (
          <React.Fragment key={option.value}>
            <FormControlLabel 
              value={option.value} 
              control={<Radio sx={{
                '&.Mui-checked': {
                  color: theme => theme.palette.background.buttonPrimaryDefault
                }
              }} />}
              label={
                <Typography variant="bodyLargeBold" color="text.primary">
                  {option.label}
                </Typography>
              } 
            />
            
            {value === option.value && renderContent && renderContent(option)}
            
            {index < options.length - 1 && (
              <Divider sx={{ borderColor: 'border.neutralDefault', my: 2 }} />
            )}
          </React.Fragment>
        ))}
      </RadioGroup>
    </FormControl>
  );
};

export default RadioSelectionGroup;