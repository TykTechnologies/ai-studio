import React from "react";
import {
  Dialog,
  DialogContent,
  DialogActions,
  Typography,
  Box,
  Button,
  IconButton,
} from "@mui/material";
import { DangerButton, SecondaryOutlineButton } from "../../styles/sharedStyles";
import Icon from '../../../components/common/Icon';
import CloseIcon from "@mui/icons-material/Close";

const ConfirmationDialog = ({
  title,
  message,
  confirmText = "Are you sure?",
  buttonLabel,
  open,
  onConfirm,
  onCancel,
  onClose,
  iconName,
  iconColor,
  titleColor,
  backgroundColor,
  borderColor,
  primaryButtonComponent = "primary",
}) => {
  return (
    <Dialog
      open={open}
      onClose={onClose || onCancel}
      PaperProps={{
        sx: {
          bgcolor: backgroundColor,
          border: "2px solid",
          borderColor: borderColor,
          borderRadius: 2,
          maxWidth: 620,
        },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1, p: 2 }}>
        <Icon name={iconName} sx={{ width: 16, height: 16, mt: 0.3, color: iconColor }}/>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 1, px: 1, py: 0 }}>
            <Typography variant="headingMedium" color={titleColor}>
            {title}
            </Typography>
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
                {message}
            </Typography>
            <Typography sx={{mt: 2}} variant="bodyMediumDefault" color="text.defaultSubdued">
                {confirmText}
            </Typography>
        </DialogContent>
        <IconButton onClick={onClose || onCancel} size="small" sx={{ p: 0 }}>
          <CloseIcon />
        </IconButton>
      </Box>
      <Box sx={{ px: 2 }}>
        <DialogActions sx={{ 
            borderTop: "1px solid", 
            borderColor: "border.neutralDefault",
            justifyContent: "flex-end",
            py: 1,
            px: 0,
            gap: 1,
            mt: 2,
            mb: 1,
        }}>
            <SecondaryOutlineButton onClick={onCancel}>Cancel</SecondaryOutlineButton>
            {primaryButtonComponent === "danger" ? (
              <DangerButton onClick={onConfirm}>
                {buttonLabel}
              </DangerButton>
            ) : (
              <Button onClick={onConfirm} variant="contained">
                {buttonLabel}
              </Button>
            )}
        </DialogActions>
      </Box>
    </Dialog>
  );
};

export default ConfirmationDialog;