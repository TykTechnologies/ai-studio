import React from "react";
import { Alert, AlertTitle, Box } from "@mui/material";
import { BULK_ACTION_VERBS } from "./bulkActions";

/**
 * Lists, by name, the items a bulk action could not process. The snackbar
 * carries the one-line summary; this stays on the page until dismissed so
 * the admin can act on each failure.
 */
const BulkResultAlert = ({ action, failures, onClose }) => {
  if (!failures || failures.length === 0) return null;
  const verb = BULK_ACTION_VERBS[action]?.present || "process";
  return (
    <Alert severity="error" onClose={onClose} sx={{ mb: 2 }} data-testid="bulk-failures">
      <AlertTitle>
        Could not {verb} {failures.length} {failures.length === 1 ? "item" : "items"}
      </AlertTitle>
      <Box component="ul" sx={{ m: 0, pl: 2 }}>
        {failures.map((failure) => (
          <li key={failure.id}>
            {failure.name}: {failure.error}
          </li>
        ))}
      </Box>
    </Alert>
  );
};

export default BulkResultAlert;
