import React, { useCallback, useState } from "react";
import { Alert, Snackbar } from "@mui/material";

/**
 * The bottom-centre snackbar every list page used to hand-roll. Pages call
 * `notify(message, severity)` for in-page results (deletes, toggles, bulk
 * summaries); messages that follow a navigation go through RouteSnackbar.
 */
export const useFeedbackSnackbar = () => {
  const [state, setState] = useState({ open: false, message: "", severity: "success" });

  const notify = useCallback((message, severity = "success") => {
    setState({ open: true, message, severity });
  }, []);

  const handleClose = useCallback((event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setState((current) => ({ ...current, open: false }));
  }, []);

  return { notify, snackbarProps: { ...state, onClose: handleClose } };
};

const FeedbackSnackbar = ({ open, message, severity = "success", onClose }) => (
  <Snackbar
    open={open}
    autoHideDuration={6000}
    onClose={onClose}
    anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
  >
    <Alert onClose={onClose} severity={severity} sx={{ width: "100%" }}>
      {message}
    </Alert>
  </Snackbar>
);

export default FeedbackSnackbar;
