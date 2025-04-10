import { styled } from "@mui/material/styles";
import { TextField, Typography, Link, Checkbox, FormControlLabel } from "@mui/material";
import React from "react";

export const StyledTextField = styled(TextField)(({ theme }) => ({
  width: '100%',
  height: '36px',
  backgroundColor: theme.palette.custom.white,
  borderRadius: '8px',
  '& .MuiOutlinedInput-root': {
    width: '100%',
    boxSizing: 'border-box',
    borderRadius: '8px',
  },
  '& .MuiInputBase-input': {
    height: '36px',
    width: '100%',
    boxSizing: 'border-box',
  }
}));

export const FormLabel = styled(Typography)(({ theme }) => ({
  display: 'block',
  ...theme.typography.bodyLargeBold,
  color: theme.palette.text.primary,
}));

export const FormLink = styled(Link)(({ theme }) => ({
  ...theme.typography.bodyMediumSemiBold,
  color: theme.palette.text.primary,
  textDecoration: 'underline',
}));

export const FormText = styled(Typography)(({ theme }) => ({
  ...theme.typography.bodyMediumDefault,
  color: theme.palette.text.primary,
}));

export const FormWrapper = styled('div')(({ theme }) => ({
  width: '100%',
  position: 'relative',
  padding: theme.spacing(9),
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'stretch',
  
  '&::before': {
    content: '""',
    position: 'absolute',
    inset: '2px',
    borderRadius: '6px',
    background: 'linear-gradient(110.34deg, rgba(255, 255, 255, 0.53) 47%, rgba(255, 255, 255, 0.23) 100%)',
    backdropFilter: 'blur(43.46px)',
    WebkitBackdropFilter: 'blur(43.46px)',
    boxShadow: '0px 8.69px 43.46px 0px rgba(0, 0, 0, 0.25)',
    zIndex: -1,
  },
  
  '&::after': {
    content: '""',
    position: 'absolute',
    inset: 0,
    borderRadius: '8px',
    padding: '2px',
    background: `linear-gradient(163.33deg, ${theme.palette.primary.main} 46.22%, ${theme.palette.custom.purpleExtraDark} 161.35%)`,
    WebkitMask: 
      'linear-gradient(#fff 0 0) content-box, ' +
      'linear-gradient(#fff 0 0)',
    WebkitMaskComposite: 'xor',
    maskComposite: 'exclude',
    zIndex: -2,
  },
  
  '& > *': {
    position: 'relative',
    zIndex: 1,
  },
  
  [theme.breakpoints.down('sm')]: {
    padding: theme.spacing(2),
  },
}));

export const ContentContainer = styled('div')(({ theme }) => ({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  width: "100%",
  height: "100%",
  [theme.breakpoints.up('sm')]: {
    width: "643px",
  },
  zIndex: 3,
}));

export const Logo = styled('img')(({ theme }) => ({
  width: "100%",
  marginBottom: theme.spacing(1),
  [theme.breakpoints.up('sm')]: {
    width: "397px",
  },
}));

const CustomCheckbox = styled(Checkbox)(({ theme }) => ({
  '& .MuiSvgIcon-root': {
    fill: theme.palette.background.buttonPrimaryDefault,
  }
}));

const StyledLabel = styled(FormControlLabel)(({ theme }) => ({
  '& .MuiFormControlLabel-label': {
    ...theme.typography.bodyLargeBold,
    color: theme.palette.text.primary,
  }
}));

export const StyledCheckbox = ({ checked, onChange, label, ...props }) => {
  return (
    <StyledLabel
      control={
        <CustomCheckbox 
          checked={checked} 
          onChange={(e) => onChange(e.target.checked)} 
          {...props} 
        />
      }
      label={label}
    />
  );
};