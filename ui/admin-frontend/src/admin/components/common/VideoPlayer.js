import React, { useState } from 'react';
import { Box } from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { PrimaryButton } from '../../styles/sharedStyles';

const VideoPlayer = ({ url, thumbnailUrl, sx = {} }) => {
  const [isPlaying, setIsPlaying] = useState(false);
  
  const handlePlayClick = () => {
    setIsPlaying(true);
  };
  
  const autoplayUrl = url.includes('?')
    ? `${url}&autoplay=1`
    : `${url}?autoplay=1`;
  
  return (
    <Box sx={{
      position: 'relative',
      width: '100%',
      height: '100%',
      borderRadius: '8px',
      overflow: 'hidden',
      ...sx
    }}>
      {isPlaying ? (
        <Box
          component="iframe"
          src={autoplayUrl}
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            objectFit: 'cover',
            border: 'none',
            borderRadius: '8px',
            width: '100%',
            height: "100%"
          }}
          allowFullScreen
          allow="autoplay"
          title="Video Player"
        />
      ) : (
        <Box sx={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%' }}>
          <Box
            component="img"
            src={thumbnailUrl}
            alt="Video thumbnail"
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              objectFit: 'cover',
              borderRadius: '8px',
              width: '100%',
              height: "100%"
            }}
          />
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
        </Box>
      )}
    </Box>
  );
};

export default VideoPlayer;