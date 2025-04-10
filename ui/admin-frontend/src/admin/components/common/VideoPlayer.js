import React, { useState, useRef } from 'react';
import { Box } from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { PrimaryButton } from '../../styles/sharedStyles';

const VideoPlayer = ({ url, sx = {} }) => {
  const [showPlayButton, setShowPlayButton] = useState(true);
  const iframeRef = useRef(null);

  const handlePlayClick = () => {
    setShowPlayButton(false);
    
    // Get the iframe element
    const iframe = iframeRef.current;
    if (iframe) {
      // Add autoplay parameter to the URL
      const autoplayUrl = url.includes('?')
        ? `${url}&autoplay=1`
        : `${url}?autoplay=1`;
      
      // Update the iframe src to include autoplay
      iframe.src = autoplayUrl;
    }
  };

  return (
    <Box sx={{ position: 'relative', width: '100%', height: '100%', ...sx }}>
      <Box
        component="iframe"
        ref={iframeRef}
        src={url}
        width="100%"
        height="100%"
        sx={{
          border: 'none',
          borderRadius: '8px',
          minHeight: '250px',
          boxShadow: '0px 4px 10px rgba(0, 0, 0, 0.1)',
        }}
        allowFullScreen
        allow="autoplay"
        title="Video Player"
      />
      
      {showPlayButton && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            backgroundColor: 'rgba(0, 0, 0, 0.1)',
            borderRadius: '8px',
            zIndex: 1,
          }}
        >
          <PrimaryButton
            variant="contained"
            onClick={handlePlayClick}
            startIcon={<PlayArrowIcon />}
          >
            play demo
          </PrimaryButton>
        </Box>
      )}
    </Box>
  );
};

export default VideoPlayer;