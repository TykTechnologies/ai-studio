import React from "react";
import { Droppable, Draggable } from "react-beautiful-dnd";
import { Box, Typography, IconButton } from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";

const FloatingSection = ({
  title,
  items,
  droppableId,
  onRemove,
  emptyText,
}) => {
  return (
    <Box sx={{ border: "1px solid #ccc", borderRadius: 2, p: 2 }}>
      <Typography variant="h6" gutterBottom>
        {title}
      </Typography>
      <Droppable droppableId={droppableId}>
        {(provided) => (
          <Box
            {...provided.droppableProps}
            ref={provided.innerRef}
            sx={{ minHeight: 100 }}
          >
            {items.length > 0 ? (
              items.map((item, index) => (
                <Draggable key={item.id} draggableId={item.id} index={index}>
                  {(provided) => (
                    <Box
                      ref={provided.innerRef}
                      {...provided.draggableProps}
                      {...provided.dragHandleProps}
                      sx={{
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "center",
                        bgcolor: "background.paper",
                        p: 1,
                        mb: 1,
                        borderRadius: 1,
                      }}
                    >
                      <Typography>{item.name}</Typography>
                      {onRemove && (
                        <IconButton
                          size="small"
                          onClick={() => onRemove(item.id)}
                        >
                          <CloseIcon fontSize="small" />
                        </IconButton>
                      )}
                    </Box>
                  )}
                </Draggable>
              ))
            ) : (
              <Typography color="text.secondary">
                {emptyText || "No items"}
              </Typography>
            )}
            {provided.placeholder}
          </Box>
        )}
      </Droppable>
    </Box>
  );
};

export default FloatingSection;
