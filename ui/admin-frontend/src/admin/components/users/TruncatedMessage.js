import React, { useState } from 'react';
import { Typography, Button, Box } from '@mui/material';

const TruncatedMessage = ({ message, maxLength = 150 }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const shouldTruncate = message.length > maxLength;

  const toggleExpand = () => setIsExpanded(!isExpanded);

  const formattedMessage = isExpanded || !shouldTruncate
    ? JSON.stringify(JSON.parse(message), null, 2)
    : JSON.stringify(JSON.parse(message), null, 2).substring(0, maxLength) + '...';

  return (
    <Box>
      <Typography
        component="pre"
        sx={{
          fontFamily: 'monospace',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}
      >
        {formattedMessage}
      </Typography>
      {shouldTruncate && (
        <Button onClick={toggleExpand} size="small">
          {isExpanded ? 'Collapse' : 'Expand'}
        </Button>
      )}
    </Box>
  );
};

export default TruncatedMessage;
