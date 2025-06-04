import React from "react";
import { TooltipIcon, StyledTooltip } from "./styles";
import { Box } from "@mui/material";

const InfoTooltip = ({ title }) => {
  const [open, setOpen] = React.useState(false);

  const handleTooltipClose = () => {
    setOpen(false);
  };

  const handleTooltipOpen = () => {
    setOpen(true);
  };

  return (
    <StyledTooltip
      title={title}
      placement="right"
      open={open}
      onClose={handleTooltipClose}
      onOpen={handleTooltipOpen}
    >
      <Box component="span" display="inline-flex" alignItems="center">
        <TooltipIcon name="circle-question" />
      </Box>
    </StyledTooltip>
  );
};

export default InfoTooltip;
