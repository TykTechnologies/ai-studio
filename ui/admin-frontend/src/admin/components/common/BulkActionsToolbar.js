import React from "react";
import { Box, Button, Tooltip, Typography } from "@mui/material";

/**
 * The strip DataTable shows above its rows while something is selected:
 * "N selected", one button per bulk action, and Clear.
 *
 * actions: [{ key, label, onClick(selectedItems), disabled | disabled(selectedItems),
 *   disabledReason | disabledReason(selectedItems), danger }]
 */
const BulkActionsToolbar = ({ count, selectedItems = [], actions = [], onClear }) => (
  <Box
    role="toolbar"
    aria-label="Bulk actions"
    data-testid="bulk-actions-toolbar"
    sx={{
      display: "flex",
      alignItems: "center",
      flexWrap: "wrap",
      gap: 1,
      px: 2,
      py: 1,
      borderBottom: (theme) => `1px solid ${theme.palette.border.neutralDefault}`,
      backgroundColor: (theme) => theme.palette.background.neutralDefault,
    }}
  >
    <Typography variant="bodyMediumDefault" sx={{ fontWeight: 600, mr: 1 }} data-testid="bulk-selected-count">
      {count} selected
    </Typography>
    {actions.map((action) => {
      const disabled =
        typeof action.disabled === "function" ? action.disabled(selectedItems) : Boolean(action.disabled);
      const reason =
        typeof action.disabledReason === "function"
          ? action.disabledReason(selectedItems)
          : action.disabledReason;
      const button = (
        <Button
          key={action.key || action.label}
          size="small"
          variant="outlined"
          color={action.danger ? "error" : "primary"}
          disabled={disabled}
          onClick={() => action.onClick?.(selectedItems)}
          data-testid={`bulk-action-${action.key || action.label}`}
        >
          {action.label}
        </Button>
      );
      if (disabled && reason) {
        return (
          <Tooltip key={action.key || action.label} title={reason}>
            <span>{button}</span>
          </Tooltip>
        );
      }
      return button;
    })}
    <Box sx={{ flex: 1 }} />
    <Button size="small" onClick={onClear} data-testid="bulk-clear">
      Clear
    </Button>
  </Box>
);

export default BulkActionsToolbar;
