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
          maxWidth: 800,
        },
      }}
    >
      <Box sx={{ 
        p: 3, 
        borderBottom: "2px solid",
        borderColor: "border.neutralDefault",
      }}>
        <Typography variant="headingMedium" color="text.primary">
          {title}
        </Typography>
      </Box>
      
      <DialogContent>
        {children}
      </DialogContent>
      
      <DialogActions sx={{ 
        borderTop: "2px solid", 
        borderColor: "border.neutralDefault",
        justifyContent: "flex-end",
        p: 2,
        gap: 2,
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