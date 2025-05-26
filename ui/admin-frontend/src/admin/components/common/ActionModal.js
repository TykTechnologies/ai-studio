import React from "react";
import { Dialog, DialogContent, DialogActions, Typography, Box } from "@mui/material";
import { PrimaryButton, SecondaryOutlineButton } from "../../styles/sharedStyles";

const ActionModal = ({
  open,
  title,
  children,
  primaryButtonLabel = "Save",
  secondaryButtonLabel = "Cancel",
  onClose,
  onPrimaryAction,
  onSecondaryAction,
}) => {
  return (
    <Dialog
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "border.neutralDefault",
          borderRadius: 2,
          maxWidth: {
            xs: '95%',  
            sm: '85%',  
            md: '80%',  
            lg: '60%',  
            xl: '50%',  
          },
        },
      }}
    >
      <Box sx={{ 
        p: 2
      }}>
        <Typography variant="headingMedium" color="text.primary">
          {title}
        </Typography>
      </Box>
      
      <Box sx={{ 
        mx: 2,
        borderBottom: "1px solid",
        borderColor: "border.neutralDefault",
      }} />
      
      <DialogContent sx={{ px: 2 }}>
        {children}
      </DialogContent>
      
      <Box sx={{ 
        mx: 2,
        borderTop: "1px solid", 
        borderColor: "border.neutralDefault",
      }} />
      
      <DialogActions sx={{ 
        justifyContent: "flex-end",
        p: 2,
        gap: 1,
      }}>
        <SecondaryOutlineButton onClick={onSecondaryAction || onClose}>
          {secondaryButtonLabel}
        </SecondaryOutlineButton>
        <PrimaryButton onClick={onPrimaryAction}>
          {primaryButtonLabel}
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

export default ActionModal;