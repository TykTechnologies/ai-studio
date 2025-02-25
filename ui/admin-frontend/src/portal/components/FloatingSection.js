import React, { useState, useEffect } from "react";
import { Box, Typography, Checkbox, Collapse } from "@mui/material";
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';

const FloatingSection = ({ title, items, onRemove, onAdd, emptyText, messages }) => {
  const [isCollapsed, setIsCollapsed] = useState(messages?.length > 0);

  useEffect(() => {
    setIsCollapsed(messages?.length > 0);
  }, [messages]);

  return (
    <Box
      sx={{
        border: "1px solid #ccc",
        borderRadius: 2,
      }}
    >
      <Box
        onClick={() => setIsCollapsed(!isCollapsed)}
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          p: 2,
          cursor: 'pointer',
        }}
      >
        <Typography variant="headingMedium">
          {title}
        </Typography>
        <KeyboardArrowDownIcon 
          sx={{
            transform: isCollapsed ? 'none' : 'rotate(180deg)',
            transition: 'transform 0.2s'
          }}
        />
      </Box>

      <Collapse in={isCollapsed}>
        <Box sx={{ p: 2, pt: 0 }}>
          {items.length > 0 ? (
            items.map((item) => (
              <Box
                key={`${title}-${item.uniqueId || item.id}`}
                sx={{
                  display: "flex",
                  alignItems: "flex-start",
                  bgcolor: "background.paper",
                  p: 1,
                  mb: 1,
                  borderRadius: 1,
                  gap: 1,
                }}
              >
                <Checkbox
                  size="small"
                  checked={item.isSelected || false}
                  onChange={(e) => {
                    if (e.target.checked) {
                      onAdd(item);
                    } else {
                      onRemove(item);
                    }
                  }}
                  sx={{ p: 0 }}
                />
                <Box sx={{ display: "flex", flexDirection: "column" }}>
                  <Typography variant="bodyLargeMedium" sx={{ flexGrow: 1 }}>
                    {item.name}
                  </Typography>
                  <Typography 
                    variant="bodySmallDefault" 
                    color="text.defaultSubdued"
                    sx={{
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      display: '-webkit-box',
                      WebkitLineClamp: 2,
                      WebkitBoxOrient: 'vertical',
                      lineHeight: '1.2em',
                      maxHeight: '2.4em'
                    }}
                  >
                    {item.description}
                  </Typography>
                </Box>
              </Box>
            ))
          ) : (
            <Typography color="text.secondary">
              {emptyText || "No items"}
            </Typography>
          )}
        </Box>
      </Collapse>
    </Box>
  );
};

export default FloatingSection;
