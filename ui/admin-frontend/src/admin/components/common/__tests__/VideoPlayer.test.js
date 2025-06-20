import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import VideoPlayer from '../VideoPlayer';
import { ThemeProvider, createTheme } from '@mui/material/styles';

// Create a mock theme to provide to the ThemeProvider
const mockTheme = createTheme({
  palette: {
    background: {
      paper: '#ffffff',
      buttonPrimaryDefault: '#6200ee',
      buttonPrimaryDefaultHover: '#3700b3',
    },
    primary: {
      main: '#6200ee',
    },
    custom: {
      white: '#ffffff',
      purpleExtraDark: '#3700b3',
    },
    text: {
      defaultSubdued: '#666666',
    },
  },
});

// Mock the PrimaryButton component
jest.mock('../../../styles/sharedStyles', () => ({
  PrimaryButton: ({ children, onClick, startIcon }) => (
    <button onClick={onClick} data-testid="play-button">
      {startIcon}
      {children}
    </button>
  ),
}));

const renderWithTheme = (ui) => {
  return render(
    <ThemeProvider theme={mockTheme}>
      {ui}
    </ThemeProvider>
  );
};

describe('VideoPlayer Component', () => {
  const defaultProps = {
    url: 'https://example.com/video',
    thumbnailUrl: 'https://example.com/thumbnail.jpg',
  };

  test('renders with required props', () => {
    renderWithTheme(<VideoPlayer {...defaultProps} />);
    
    // Check if thumbnail is displayed
    const thumbnailImg = screen.getByAltText('Video thumbnail');
    expect(thumbnailImg).toBeInTheDocument();
    expect(thumbnailImg).toHaveAttribute('src', defaultProps.thumbnailUrl);
    
    // Check if play button is displayed
    const playButton = screen.getByTestId('play-button');
    expect(playButton).toBeInTheDocument();
    expect(playButton).toHaveTextContent('play demo');
  });

  test('applies custom styles via sx prop', () => {
    const customSx = {
      backgroundColor: 'red',
      width: '500px',
    };
    
    renderWithTheme(<VideoPlayer {...defaultProps} sx={customSx} />);
    
    // We can't directly test the sx prop with Testing Library,
    // but we can verify the component renders with the custom props
    const playButton = screen.getByTestId('play-button');
    expect(playButton).toBeInTheDocument();
    
    // The main container should be present
    const thumbnailImg = screen.getByAltText('Video thumbnail');
    expect(thumbnailImg).toBeInTheDocument();
  });

  test('shows iframe when play button is clicked', () => {
    renderWithTheme(<VideoPlayer {...defaultProps} />);
    
    // Initially, the iframe should not be present
    expect(screen.queryByTitle('Video Player')).not.toBeInTheDocument();
    
    // Click the play button
    const playButton = screen.getByTestId('play-button');
    fireEvent.click(playButton);
    
    // After clicking, the iframe should be present and the thumbnail should be gone
    const iframe = screen.getByTitle('Video Player');
    expect(iframe).toBeInTheDocument();
    expect(iframe).toHaveAttribute('src', 'https://example.com/video?autoplay=1');
    expect(screen.queryByAltText('Video thumbnail')).not.toBeInTheDocument();
    expect(screen.queryByTestId('play-button')).not.toBeInTheDocument();
  });

  test('constructs autoplay URL correctly for URL without query parameters', () => {
    renderWithTheme(<VideoPlayer {...defaultProps} />);
    
    // Click the play button
    const playButton = screen.getByTestId('play-button');
    fireEvent.click(playButton);
    
    // Check if the iframe src has the autoplay parameter added
    const iframe = screen.getByTitle('Video Player');
    expect(iframe).toHaveAttribute('src', 'https://example.com/video?autoplay=1');
  });

  test('constructs autoplay URL correctly for URL with existing query parameters', () => {
    const propsWithQueryParams = {
      ...defaultProps,
      url: 'https://example.com/video?param=value',
    };
    
    renderWithTheme(<VideoPlayer {...propsWithQueryParams} />);
    
    // Click the play button
    const playButton = screen.getByTestId('play-button');
    fireEvent.click(playButton);
    
    // Check if the iframe src has the autoplay parameter appended
    const iframe = screen.getByTitle('Video Player');
    expect(iframe).toHaveAttribute('src', 'https://example.com/video?param=value&autoplay=1');
  });

  test('iframe has correct attributes', () => {
    renderWithTheme(<VideoPlayer {...defaultProps} />);
    
    // Click the play button
    const playButton = screen.getByTestId('play-button');
    fireEvent.click(playButton);
    
    // Check if the iframe has the correct attributes
    const iframe = screen.getByTitle('Video Player');
    expect(iframe).toHaveAttribute('allowFullScreen');
    expect(iframe).toHaveAttribute('allow', 'autoplay');
    expect(iframe).toHaveAttribute('title', 'Video Player');
  });

  test('handles missing thumbnailUrl gracefully', () => {
    // This test checks if the component handles missing thumbnailUrl without crashing
    const { url } = defaultProps;
    
    // We expect a console error, but the component should render
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    renderWithTheme(<VideoPlayer url={url} />);
    
    // The component should still render the play button
    const playButton = screen.getByTestId('play-button');
    expect(playButton).toBeInTheDocument();
    
    // Cleanup
    consoleErrorSpy.mockRestore();
  });

  test('handles missing url gracefully', () => {
    // This test checks if the component handles missing url without crashing
    const { thumbnailUrl } = defaultProps;
    
    // We expect a console error, but the component should render
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    
    renderWithTheme(<VideoPlayer thumbnailUrl={thumbnailUrl} />);
    
    // The component should still render the thumbnail
    const thumbnailImg = screen.getByAltText('Video thumbnail');
    expect(thumbnailImg).toBeInTheDocument();
    
    // Cleanup
    consoleErrorSpy.mockRestore();
  });
});