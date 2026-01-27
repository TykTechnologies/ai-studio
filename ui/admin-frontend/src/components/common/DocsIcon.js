import React from 'react';
import { IconButton, Tooltip } from '@mui/material';
import MenuBookIcon from '@mui/icons-material/MenuBook';

const DocsIcon = ({ docsUrl }) => {
  const handleClick = () => {
    window.open(docsUrl, '_blank', 'noopener,noreferrer');
  };

  return (
    <Tooltip title="Documentation">
      <IconButton onClick={handleClick} sx={{ color: 'white' }}>
        <MenuBookIcon />
      </IconButton>
    </Tooltip>
  );
};

export default DocsIcon;
