import React from "react";
import { Box, Chip, Grid, Typography } from "@mui/material";

// Renders a plugin submission's resource_payload as a label/value grid. The
// reviewer has no fixed field list to fall back on -- the plugin's submission
// schema is the only thing that knows what "max_tokens" or "system_prompt"
// mean -- so titles and descriptions come from the schema when there is one,
// schema properties render first in schema order, and anything the payload
// carries beyond the schema is listed after them so nothing is silently hidden.

const isEmpty = (value) =>
  value === undefined || value === null || value === "";

const renderValue = (value, property) => {
  if (isEmpty(value)) {
    return <Typography variant="body2">—</Typography>;
  }
  if (typeof value === "boolean") {
    return (
      <Chip
        label={value ? "Yes" : "No"}
        size="small"
        color={value ? "success" : "default"}
        variant="outlined"
      />
    );
  }
  if (typeof value === "object") {
    return (
      <Box
        component="pre"
        sx={{
          m: 0,
          p: 1,
          fontSize: "0.8rem",
          fontFamily: "monospace",
          bgcolor: "action.hover",
          borderRadius: 1,
          overflowX: "auto",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
        }}
      >
        {JSON.stringify(value, null, 2)}
      </Box>
    );
  }
  if (property?.format === "password" || property?.writeOnly) {
    return <Typography variant="body2">***</Typography>;
  }
  return (
    <Typography variant="body2" sx={{ whiteSpace: "pre-wrap" }}>
      {String(value)}
    </Typography>
  );
};

const PluginPayloadView = ({ payload, schema }) => {
  const data = payload || {};
  const properties = schema?.properties || {};
  const schemaKeys = Object.keys(properties);
  const extraKeys = Object.keys(data).filter((k) => !schemaKeys.includes(k));
  const keys = [...schemaKeys, ...extraKeys];

  if (keys.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        This submission carries no resource fields.
      </Typography>
    );
  }

  return (
    <Grid container spacing={1} data-testid="plugin-payload-view">
      {keys.map((key) => {
        const property = properties[key];
        return (
          <React.Fragment key={key}>
            <Grid item xs={4}>
              <Typography
                variant="body2"
                color="text.secondary"
                fontWeight="bold"
              >
                {property?.title || key}
              </Typography>
              {property?.description && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  display="block"
                >
                  {property.description}
                </Typography>
              )}
            </Grid>
            <Grid item xs={8}>
              {renderValue(data[key], property)}
            </Grid>
          </React.Fragment>
        );
      })}
    </Grid>
  );
};

export default PluginPayloadView;
