import { useEffect, useState } from "react";
import agentTracingService from "../../services/agentTracingService";
import { isWindowed, WINDOW_MAX_SPANS } from "./sessionWindow";

/** Spans per request: the API's page size (spec §6.3). */
export const SPAN_PAGE = 1000;
/**
 * Pages are buffered and handed to the views at most this often: each
 * hand-over re-indexes every span loaded so far, so one per page made a
 * large session quadratic and froze the page while it loaded.
 */
export const FLUSH_MS = 250;

const emptySpans = { spans: [], resources: [], previews: null, loadingSpans: true, truncated: false, error: null };

/**
 * Loads a session's detail and then its span skeleton page by page, so the
 * views can render the first page while the rest streams in. A large
 * session (sessionWindow.js) loads only the spans of window ({ from, to },
 * epoch ms), and nothing until there is one. No content is fetched here:
 * payloads load lazily in the span drawer.
 */
const useSessionData = (sessionId, window = null) => {
  const [detailState, setDetailState] = useState({ detail: null, error: null });
  const [spanState, setSpanState] = useState(emptySpans);
  const { detail } = detailState;
  const windowed = isWindowed(detail);
  const from = windowed ? window?.from : undefined;
  const to = windowed ? window?.to : undefined;

  useEffect(() => {
    let cancelled = false;
    setDetailState({ detail: null, error: null });
    agentTracingService.getSession(sessionId).then(
      (d) => !cancelled && setDetailState({ detail: d, error: null }),
      (error) => !cancelled && setDetailState({ detail: null, error }),
    );
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  useEffect(() => {
    if (!detail) return undefined;
    let cancelled = false;
    if (windowed && from == null) {
      setSpanState({ ...emptySpans, loadingSpans: false });
      return undefined;
    }
    setSpanState(emptySpans);
    (async () => {
      const spans = [];
      const seen = new Map();
      let previews = null;
      let flushedAt = 0;
      const flush = (extra = {}) => {
        flushedAt = Date.now();
        setSpanState((s) => ({ ...s, spans: spans.slice(), resources: [...seen.values()], previews, ...extra }));
      };
      try {
        let cursor;
        // Inline previews are asked for only by content readers; the server
        // decides per page whether they are served (and audits them).
        const params = detail?.viewer?.can_read_content ? { include: "previews" } : {};
        if (windowed) {
          params.from = new Date(from).toISOString();
          params.to = new Date(to).toISOString();
        }
        let truncated = false;
        do {
          const page = await agentTracingService.listSpans(sessionId, { limit: SPAN_PAGE, ...params, ...(cursor ? { cursor } : {}) });
          if (cancelled) return;
          (page?.resources || []).forEach((r) => seen.set(`${r.type}:${r.id}`, r));
          spans.push(...(page?.data || []));
          // The weakest state of any page wins: one denied page means
          // some rows have no preview.
          previews = previews && previews !== "included" ? previews : page?.previews || previews;
          cursor = page?.next_cursor || null;
          if (windowed && cursor && spans.length >= WINDOW_MAX_SPANS) {
            truncated = true;
            cursor = null;
          }
          if (cursor && Date.now() - flushedAt >= FLUSH_MS) flush();
        } while (cursor);
        flush({ loadingSpans: false, truncated });
      } catch (error) {
        if (!cancelled) flush({ loadingSpans: false, error });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [sessionId, detail, windowed, from, to]);

  return { ...spanState, detail, windowed, error: detailState.error || spanState.error };
};

export default useSessionData;
