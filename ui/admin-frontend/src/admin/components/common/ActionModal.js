import React from "react";
import { Typography } from "@mui/material";
import { PrimaryButton, SecondaryOutlineButton } from "../../styles/sharedStyles";
import {
  StyledActionDialog,
  TitleBox,
  DialogDivider,
  StyledDialogContent,
  StyledDialogActions
} from "./styles";

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
  const responsiveWidths = {
    xs: '95%',  
    sm: '85%',  
    md: '80%',  
    lg: '60%',  
    xl: '50%',  
  };

  return (
    <StyledActionDialog open={open} onClose={onClose}>
      <TitleBox>
        <Typography variant="headingMedium" color="text.primary">
          {title}
        </Typography>
      </TitleBox>
      
      <DialogDivider />
      
      <StyledDialogContent>
        {children}
      </StyledDialogContent>
      
      <DialogDivider />
      
      <StyledDialogActions>
        <SecondaryOutlineButton onClick={onSecondaryAction || onClose}>
          {secondaryButtonLabel}
        </SecondaryOutlineButton>
        <PrimaryButton onClick={onPrimaryAction}>
          {primaryButtonLabel}
        </PrimaryButton>
      </StyledDialogActions>
    </StyledActionDialog>
  );
};

export default ActionModal;