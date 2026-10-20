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
  const [state, setState] = useState({ detail: null, spans: [], resources: [], previews: null, loadingSpans: true, error: null });

  useEffect(() => {
    let cancelled = false;
    setState({ detail: null, spans: [], resources: [], previews: null, loadingSpans: true, error: null });
    (async () => {
      try {
        const detail = await agentTracingService.getSession(sessionId);
        if (cancelled) return;
        setState((s) => ({ ...s, detail }));
        let cursor;
        const seen = new Map();
        // Inline previews are asked for only by content readers; the server
        // decides per page whether they are served (and audits them).
        const previews = detail?.viewer?.can_read_content ? { include: "previews" } : {};
        do {
          const page = await agentTracingService.listSpans(sessionId, { limit: SPAN_PAGE, ...previews, ...(cursor ? { cursor } : {}) });
          if (cancelled) return;
          (page?.resources || []).forEach((r) => seen.set(`${r.type}:${r.id}`, r));
          const data = page?.data || [];
          setState((s) => ({
            ...s,
            spans: [...s.spans, ...data],
            resources: [...seen.values()],
            // The weakest state of any page wins: one denied page means
            // some rows have no preview.
            previews: s.previews && s.previews !== "included" ? s.previews : page?.previews || s.previews,
          }));
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
