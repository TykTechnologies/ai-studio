import React from "react";
import { Box, Typography, TextField } from "@mui/material";

const DateRangePicker = ({
  startDate,
  endDate,
  onStartDateChange,
  onEndDateChange,
  label = "Date range:",
}) => {
  return (
    <Box display="flex" justifyContent="flex-end" alignItems="center">
      <Typography
        variant="body2"
        sx={{
          mr: 2,
          fontWeight: "medium",
          fontSize: "0.875rem",
          color: "text.secondary",
        }}
      >
        {label}
      </Typography>
      <TextField
        label="Start Date"
        type="date"
        value={startDate}
        onChange={(e) => onStartDateChange(e.target.value)}
        InputLabelProps={{ shrink: true }}
        size="small"
        sx={{ mr: 2, width: "140px" }}
      />
      <TextField
        label="End Date"
        type="date"
        value={endDate}
        onChange={(e) => onEndDateChange(e.target.value)}
        InputLabelProps={{ shrink: true }}
        size="small"
        sx={{ width: "140px" }}
      />
    </Box>
  );
};

export default DateRangePicker;
