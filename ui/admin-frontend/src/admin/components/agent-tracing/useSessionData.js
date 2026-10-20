import { useEffect, useState } from "react";
import agentTracingService from "../../services/agentTracingService";

/** Spans per request: the API's page size (spec §6.3). */
export const SPAN_PAGE = 1000;

/**
 * Loads a session's detail and then its span skeleton page by page, so the
 * views can render the first page while the rest streams in. No content is
 * fetched here: payloads load lazily in the span drawer.
 */
const useSessionData = (sessionId) => {
  const [state, setState] = useState({ detail: null, spans: [], resources: [], loadingSpans: true, error: null });

  useEffect(() => {
    let cancelled = false;
    setState({ detail: null, spans: [], resources: [], loadingSpans: true, error: null });
    (async () => {
      try {
        const detail = await agentTracingService.getSession(sessionId);
        if (cancelled) return;
        setState((s) => ({ ...s, detail }));
        let cursor;
        const seen = new Map();
        do {
          const page = await agentTracingService.listSpans(sessionId, { limit: SPAN_PAGE, ...(cursor ? { cursor } : {}) });
          if (cancelled) return;
          (page?.resources || []).forEach((r) => seen.set(`${r.type}:${r.id}`, r));
          const data = page?.data || [];
          setState((s) => ({ ...s, spans: [...s.spans, ...data], resources: [...seen.values()] }));
          cursor = page?.next_cursor || null;
        } while (cursor);
        if (!cancelled) setState((s) => ({ ...s, loadingSpans: false }));
      } catch (error) {
        if (!cancelled) setState((s) => ({ ...s, loadingSpans: false, error }));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  return state;
};

export default useSessionData;
