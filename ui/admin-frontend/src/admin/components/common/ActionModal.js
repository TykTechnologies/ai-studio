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
  // Disables the primary action (e.g. while saving or loading). Callers were
  // already passing this; it was previously ignored.
  disabled = false,
}) => {
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
        <PrimaryButton onClick={onPrimaryAction} disabled={disabled}>
          {primaryButtonLabel}
        </PrimaryButton>
      </StyledDialogActions>
    </StyledActionDialog>
  );
};

export default ActionModal;