import React, { useState, useEffect, useRef } from "react";
import { Box, Typography, Checkbox, Collapse } from "@mui/material";
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';

const FloatingSection = ({ title, items, onRemove, onAdd, emptyText, messages, roomName }) => {
  const [isCollapsed, setIsCollapsed] = useState(messages?.length > 0);
  const hasUserInteracted = useRef(false);
  const prevMessagesLength = useRef(messages?.length || 0);

  useEffect(() => {
    const currentMessagesLength = messages?.length || 0;

    if (!hasUserInteracted.current &&
      prevMessagesLength.current === 0 &&
      currentMessagesLength > 0) {
      setIsCollapsed(true);
    }

    prevMessagesLength.current = currentMessagesLength;
  }, [messages]);

  const handleToggle = () => {
    hasUserInteracted.current = true;
    setIsCollapsed(!isCollapsed);
  };

  return (
    <Box
      sx={{
        border: "1px solid #ccc",
        borderRadius: 2,
      }}
    >
      <Box
        onClick={handleToggle}
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          p: 2,
          cursor: 'pointer',
        }}
      >
        <Box>
          <Typography variant="headingMedium">
            {title}
          </Typography>
          {roomName && (
            <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
              Room: {roomName}
            </Typography>
          )}
        </Box>
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
                onClick={() => {
                  if (item.isSelected) {
                    onRemove(item);
                  } else {
                    onAdd(item);
                  }
                }}
                sx={{
                  display: "flex",
                  alignItems: "flex-start",
                  bgcolor: "background.paper",
                  p: 1,
                  mb: 1,
                  borderRadius: 1,
                  gap: 1,
                  cursor: "pointer",
                }}
              >
                <Checkbox
                  size="small"
                  checked={item.isSelected || false}
                  onChange={(e) => {
                    e.stopPropagation();
                    if (e.target.checked) {
                      onAdd(item);
                    } else {
                      onRemove(item);
                    }
                  }}
                  onClick={(e) => e.stopPropagation()}
                  sx={{ p: 0 }}
                />
                <Box sx={{ display: "flex", flexDirection: "column", flexGrow: 1 }}>
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
