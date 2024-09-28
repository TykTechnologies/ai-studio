import React from "react";
import { Box, Typography, IconButton } from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import AddIcon from "@mui/icons-material/Add";
const FloatingSection = ({ title, items, onRemove, onAdd, emptyText }) => {
  return (
    <Box
      sx={{
        border: "1px solid #ccc",
        borderRadius: 2,
        p: 2,
      }}
    >
      <Typography variant="h6" gutterBottom>
        {title}
      </Typography>
      <Box sx={{ minHeight: 100 }}>
        {items.length > 0 ? (
          items.map((item) => (
            <Box
              key={`${title}-${item.uniqueId || item.id}`}
              sx={{
                display: "flex",
                justifyContent: "flex-start",
                alignItems: "center",
                bgcolor: "background.paper",
                p: 1,
                mb: 1,
                borderRadius: 1,
              }}
            >
              {onAdd && (
                <IconButton
                  size="small"
                  onClick={() => onAdd(item)}
                  sx={{ mr: 1 }}
                >
                  <AddIcon fontSize="small" />
                </IconButton>
              )}
              <Typography sx={{ flexGrow: 1 }}>{item.name}</Typography>
              {onRemove && (
                <IconButton size="small" onClick={() => onRemove(item)}>
                  <CloseIcon fontSize="small" />
                </IconButton>
              )}
            </Box>
          ))
        ) : (
          <Typography color="text.secondary">
            {emptyText || "No items"}
          </Typography>
        )}
      </Box>
    </Box>
  );
};

export default FloatingSection;
