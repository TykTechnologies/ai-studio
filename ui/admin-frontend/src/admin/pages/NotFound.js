import React from "react";
import { Link as RouterLink } from "react-router-dom";
import { Box, Button, Typography } from "@mui/material";

/**
 * Rendered by the admin catch-all route. Before it existed an unknown admin
 * URL (a typo, an old bookmark, a guessed /llms/:id/edit) rendered a blank
 * content area with no hint that nothing matched.
 */
const NotFound = () => (
  <Box sx={{ p: 4, textAlign: "center" }} data-testid="admin-not-found">
    <Typography variant="headingXLarge" component="h1" gutterBottom>
      Page not found
    </Typography>
    <Typography variant="bodyLargeDefault" color="text.defaultSubdued" paragraph>
      There is no admin page at this address. It may have moved, or the link may be out of date.
    </Typography>
    <Button component={RouterLink} to="/admin" variant="contained">
      Back to overview
    </Button>
  </Box>
);

export default NotFound;
