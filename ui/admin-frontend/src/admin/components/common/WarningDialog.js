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
import { DangerButton } from "../../styles/sharedStyles";
import Icon from '../../../components/common/Icon';
import CloseIcon from "@mui/icons-material/Close";

/**
 * A reusable warning dialog component for confirmation of destructive actions.
 * 
 * @param {Object} props - Component props
 * @param {string} props.title - The title of the warning dialog
 * @param {string} props.message - The message to display in the dialog
 * @param {string} props.buttonLabel - The label for the action button
 * @param {boolean} props.open - Whether the dialog is open
 * @param {Function} props.onConfirm - Callback when the action is confirmed
 * @param {Function} props.onCancel - Callback when the action is canceled
 * @param {Function} props.onClose - Optional callback when the dialog is closed (defaults to onCancel)
 */
const WarningDialog = ({
  title,
  message,
  buttonLabel,
  open,
  onConfirm,
  onCancel,
  onClose,
}) => {
  return (
    <Dialog
      open={open}
      onClose={onClose || onCancel}
      PaperProps={{
        sx: {
          bgcolor: "background.surfaceCriticalDefault",
          border: "2px solid",
          borderColor: "border.criticalDefaultSubdue",
          borderRadius: 2,
          maxWidth: 620,
        },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1, p: 2 }}>
        <Icon name="hexagon-exclamation" color="error" sx={{ width: 16, height: 16, mt: 0.3 }}/>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 1, px: 1, py: 0 }}>
            <Typography variant="headingMedium" color="text.criticalDefault">
            {title}
            </Typography>
            <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
                {message}
            </Typography>
            <Typography sx={{mt: 2}} variant="bodyMediumDefault" color="text.defaultSubdued">
                Are you sure?
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
            <Button onClick={onCancel}>Cancel</Button>
            <DangerButton onClick={onConfirm}>
            {buttonLabel}
            </DangerButton>
        </DialogActions>
      </Box>
    </Dialog>
  );
};

export default WarningDialog;