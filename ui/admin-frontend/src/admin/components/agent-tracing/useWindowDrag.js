import { useCallback, useEffect, useRef } from "react";

// A drag that keeps tracking once started: moves and the release are heard
// on the window, so leaving the element (or releasing outside it) neither
// cancels nor loses the drag.
const useWindowDrag = () => {
  const stop = useRef(null);
  useEffect(() => () => stop.current?.(), []);
  return useCallback((onMove, onEnd) => {
    stop.current?.();
    const move = (e) => onMove(e);
    const up = (e) => {
      cleanup();
      onEnd(e);
    };
    const cleanup = () => {
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
      stop.current = null;
    };
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
    stop.current = cleanup;
  }, []);
};

export default useWindowDrag;
