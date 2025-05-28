import { useCallback, useRef, useEffect } from 'react';
import { ScrollContainer } from './styles';

export default function InfiniteScrollContainer({
  children,
  onLoadMore,
  hasMore,
  isLoading,
  threshold = 50,
}) {
  const ref = useRef(null);

  const handleScroll = useCallback(() => {
    if (!ref.current || !hasMore || isLoading) return;
    const { scrollTop, scrollHeight, clientHeight } = ref.current;
    if (scrollHeight - scrollTop - clientHeight < threshold) {
      onLoadMore?.();
    }
  }, [hasMore, isLoading, onLoadMore, threshold]);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.addEventListener('scroll', handleScroll);
    return () => el.removeEventListener('scroll', handleScroll);
  }, [handleScroll]);

  return (
    <ScrollContainer ref={ref}>
      {children}
    </ScrollContainer>
  );
} 