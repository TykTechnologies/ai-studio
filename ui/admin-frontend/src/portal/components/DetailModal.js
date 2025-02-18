import React from 'react';
import {
  Modal,
  Box,
  Typography,
} from '@mui/material';

const DetailModal = ({ open, handleClose, title, children }) => {
  return (
    <Modal
      open={open}
      onClose={handleClose}
      aria-labelledby="detail-modal-title"
      aria-describedby="detail-modal-description"
    >
      <Box
        sx={{
          position: "absolute",
          top: "50%",
          left: "50%",
          transform: "translate(-50%, -50%)",
          width: 400,
          bgcolor: "background.paper",
          boxShadow: 24,
          borderRadius: "16px",
          overflow: "hidden",
          maxHeight: "80vh",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <Box
          sx={{
            bgcolor: "background.paper",
            color: "text.primary",
            p: 2,
          }}
        >
          <Typography
            id="detail-modal-title"
            variant="h6"
            component="h2"
          >
            {title}
          </Typography>
        </Box>
        <Box sx={{ p: 3, overflowY: "auto" }}>
          {children}
        </Box>
      </Box>
    </Modal>
  );
};

export default DetailModal;
