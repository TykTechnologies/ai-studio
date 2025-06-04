import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';

import InfiniteScrollContainer from '../InfiniteScrollContainer';

jest.mock('../styles', () => ({
  ScrollContainer: function MockScrollContainer(props) {
    return (
      <div 
        data-testid="scroll-container"
        {...props}
      >
        {props.children}
      </div>
    );
  }
}));

describe('InfiniteScrollContainer Component', () => {
  const mockOnLoadMore = jest.fn();
  
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders with children', () => {
    render(
      <InfiniteScrollContainer>
        <p>Test Content</p>
      </InfiniteScrollContainer>
    );
    
    expect(screen.getByTestId('scroll-container')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders with hasMore prop set to true', () => {
    render(
      <InfiniteScrollContainer
        hasMore={true}
        onLoadMore={mockOnLoadMore}
      >
        <p>Test Content</p>
      </InfiniteScrollContainer>
    );
    
    expect(screen.getByTestId('scroll-container')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders with hasMore prop set to false', () => {
    render(
      <InfiniteScrollContainer
        hasMore={false}
        onLoadMore={mockOnLoadMore}
      >
        <p>Test Content</p>
      </InfiniteScrollContainer>
    );
    
    expect(screen.getByTestId('scroll-container')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders with isLoading prop set to true', () => {
    render(
      <InfiniteScrollContainer
        isLoading={true}
        onLoadMore={mockOnLoadMore}
      >
        <p>Test Content</p>
      </InfiniteScrollContainer>
    );
    
    expect(screen.getByTestId('scroll-container')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });

  test('renders with custom threshold value', () => {
    render(
      <InfiniteScrollContainer
        threshold={100}
        onLoadMore={mockOnLoadMore}
      >
        <p>Test Content</p>
      </InfiniteScrollContainer>
    );
    
    expect(screen.getByTestId('scroll-container')).toBeInTheDocument();
    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });
});