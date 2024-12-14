import React from "react";
import { Tooltip, IconButton } from "@mui/material";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";

const InfoTooltip = ({ title }) => (
  <Tooltip
    title={title}
    arrow
    placement="left"
    componentsProps={{
      tooltip: {
        sx: {
          backgroundColor: "white",
          color: "rgba(0, 0, 0, 0.87)",
          fontSize: "1rem",
          padding: "10px 15px",
          boxShadow: "0px 2px 10px rgba(0, 0, 0, 0.1)",
          "& .MuiTooltip-arrow": {
            color: "white",
          },
        },
      },
    }}
  >
    <IconButton sx={{ padding: 0 }}>
      <InfoOutlinedIcon
        sx={{
          cursor: "help",
          fontSize: "1.5em",
          color: "black",
          mr: 1,
          paddingBottom: "2px",
        }}
      />
    </IconButton>
  </Tooltip>
);

export default InfoTooltip;
