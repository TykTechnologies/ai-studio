import React from "react";
import { Box, Typography, TextField, Button } from "@mui/material";
import { StyledButton } from "../../styles/sharedStyles";

const DateRangePicker = ({
  startDate,
  endDate,
  onStartDateChange,
  onEndDateChange,
  onUpdate,
  updateMode = "immediate",
  label = "Date range:",
}) => {
  return (
    <Box display="flex" justifyContent="flex-end" alignItems="center" gap={2}>
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
        onChange={(e) => {
          onStartDateChange(e.target.value);
          if (updateMode === "immediate") {
            onUpdate?.();
          }
        }}
        InputLabelProps={{ shrink: true }}
        size="small"
        sx={{ mr: 2, width: "140px" }}
      />
      <TextField
        label="End Date"
        type="date"
        value={endDate}
        onChange={(e) => {
          onEndDateChange(e.target.value);
          if (updateMode === "immediate") {
            onUpdate?.();
          }
        }}
        InputLabelProps={{ shrink: true }}
        size="small"
        sx={{ width: "140px" }}
      />
      {updateMode === "manual" && (
        <StyledButton
          variant="contained"
          onClick={onUpdate}
          size="small"
        >
          Update
        </StyledButton>
      )}
    </Box>
  );
};

export default DateRangePicker;
