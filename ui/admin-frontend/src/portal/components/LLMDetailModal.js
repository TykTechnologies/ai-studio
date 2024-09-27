import React from "react";
import { Modal, Box, Typography, Button } from "@mui/material";

const style = {
  position: "absolute",
  top: "50%",
  left: "50%",
  transform: "translate(-50%, -50%)",
  width: 400,
  bgcolor: "background.paper",
  border: "2px solid #000",
  boxShadow: 24,
  p: 4,
};

const LLMDetailModal = ({ llm, open, handleClose }) => {
  return (
    <Modal
      open={open}
      onClose={handleClose}
      aria-labelledby="modal-modal-title"
      aria-describedby="modal-modal-description"
    >
      <Box sx={style}>
        <Typography id="modal-modal-title" variant="h6" component="h2">
          {llm.attributes.name}
        </Typography>
        <Typography id="modal-modal-description" sx={{ mt: 2 }}>
          Vendor: {llm.attributes.vendor}
        </Typography>
        <Typography sx={{ mt: 2 }}>
          {llm.attributes.long_description}
        </Typography>
        <Button onClick={handleClose} sx={{ mt: 2 }}>
          Close
        </Button>
      </Box>
    </Modal>
  );
};

export default LLMDetailModal;
