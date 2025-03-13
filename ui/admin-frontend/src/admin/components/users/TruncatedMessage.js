import React, { useState } from 'react';
import { Typography, Button, Box } from '@mui/material';

const TruncatedMessage = ({ message, maxLength = 150 }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  
  // Parse the message if it's a string, or use it directly if it's already an object
  let parsedMessage;
  let messageString;
  
  try {
    if (typeof message === 'string') {
      // Try to parse as JSON if it's a string
      parsedMessage = JSON.parse(message);
      messageString = JSON.stringify(parsedMessage, null, 2);
    } else {
      // If it's already an object, use it directly
      parsedMessage = message;
      messageString = JSON.stringify(parsedMessage, null, 2);
    }
  } catch (error) {
    // If parsing fails, use the raw message
    console.warn('Error parsing message:', error);
    messageString = typeof message === 'string' ? message : String(message);
  }
  
  const shouldTruncate = messageString.length > maxLength;
  const toggleExpand = () => setIsExpanded(!isExpanded);

  const formattedMessage = isExpanded || !shouldTruncate
    ? messageString
    : messageString.substring(0, maxLength) + '...';

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
