import React, { createContext, useContext, useMemo, useReducer } from "react";
import { indexSpans } from "./spanModel";

// One selection model and one time-range filter shared by every view of a
// session (spec §6.2): selecting a span in one view selects it in all, and
// a range dragged on the timeline strip filters them all.

const SessionDetailContext = createContext(null);

const initial = {
  view: "transcript",
  selectedId: null,
  drawerSpanId: null,
  range: null, // { from, to } in epoch ms
  viewport: null, // the Timeline view's visible window while it is open, for the strip's cursor
  expandAll: false,
  findOpen: false,
};

export const reducer = (state, action) => {
  switch (action.type) {
    case "view":
      return { ...state, view: action.view };
    case "select":
      return { ...state, selectedId: action.id };
    case "open":
      return { ...state, selectedId: action.id ?? state.selectedId, drawerSpanId: action.id ?? state.selectedId };
    case "close":
      return { ...state, drawerSpanId: null };
    case "range":
      return { ...state, range: action.range };
    case "viewport":
      return { ...state, viewport: action.viewport };
    case "toggleExpandAll":
      return { ...state, expandAll: !state.expandAll };
    case "find":
      return { ...state, findOpen: action.open };
    default:
      return state;
  }
};

export const SessionDetailProvider = ({ detail, spans, resources, previews = null, children, initialState }) => {
  const [state, dispatch] = useReducer(reducer, { ...initial, ...initialState });
  const index = useMemo(() => indexSpans(spans, state.range), [spans, state.range]);
  const value = useMemo(
    () => ({
      ...state,
      dispatch,
      detail,
      spans,
      resources,
      previews,
      index,
      appId: detail?.session?.app_id,
      viewer: detail?.viewer || {},
    }),
    [state, detail, spans, resources, previews, index],
  );
  return <SessionDetailContext.Provider value={value}>{children}</SessionDetailContext.Provider>;
};

export const useSessionDetail = () => {
  const ctx = useContext(SessionDetailContext);
  if (!ctx) throw new Error("useSessionDetail outside SessionDetailProvider");
  return ctx;
};
