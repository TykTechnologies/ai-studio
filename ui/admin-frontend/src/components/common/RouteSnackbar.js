import React, { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Alert, Snackbar } from "@mui/material";

/**
 * Shows a snackbar handed over through router state, once.
 *
 * Forms navigate away immediately after a successful save with
 * `navigate(path, { state: { snackbar: { message, severity } } })`; this
 * component (rendered once in the layout) picks the message up on the
 * destination and clears it from history state so a refresh or back
 * navigation does not replay it.
 */
const RouteSnackbar = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [snackbar, setSnackbar] = useState(null);

  const routeSnackbar = location.state?.snackbar;

  useEffect(() => {
    if (routeSnackbar?.message) {
      setSnackbar(routeSnackbar);
      // Strip the message from history state, keeping everything else.
      const { snackbar: _consumed, ...rest } = location.state || {};
      navigate(location.pathname + location.search + location.hash, {
        replace: true,
        state: rest,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [routeSnackbar]);

  const handleClose = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar(null);
  };

  if (!snackbar) {
    return null;
  }

  return (
    <Snackbar
      open
      autoHideDuration={6000}
      onClose={handleClose}
      anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
    >
      <Alert
        onClose={handleClose}
        severity={snackbar.severity || "success"}
        sx={{ width: "100%" }}
      >
        {snackbar.message}
      </Alert>
    </Snackbar>
  );
};

export default RouteSnackbar;
