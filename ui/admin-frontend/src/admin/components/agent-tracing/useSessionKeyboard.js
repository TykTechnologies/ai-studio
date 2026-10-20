import { useEffect } from "react";
import { useSessionDetail } from "./SessionDetailContext";
import { nextInList } from "./spanModel";

// The session detail's keys (spec §6.2 "Navigation and search"):
//   1 / 2      Transcript / Timeline (3, Tree, comes in Phase 2)
//   j / k      next / previous step (LLM call)
//   n / N      next / previous tool call
//   e / E      next / previous error or gateway denial
//   g          next gateway event
//   / , Ctrl/Cmd-F   find in session
//   w          expand / collapse all
//   Enter      open the selected span
//   Esc        close the drawer (or the find bar)
// Keys typed into a field are left alone. Moving while the drawer is open
// moves the drawer with the selection.

export const VIEW_KEYS = { 1: "transcript", 2: "timeline" };

// Enter opens the selected span only from the span views (or with nothing
// focused), so it never doubles up with a focused button or link.
const inSpanView = (el) => !el || el === document.body || Boolean(el.closest?.('[data-span-view="true"]'));

const isTyping = (el) => {
  if (!el) return false;
  const tag = el.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || el.isContentEditable;
};

const useSessionKeyboard = () => {
  const ctx = useSessionDetail();
  const { index, selectedId, drawerSpanId, findOpen, dispatch } = ctx;

  useEffect(() => {
    const move = (list, dir) => {
      const id = nextInList(index, list, selectedId, dir);
      if (!id) return;
      dispatch({ type: drawerSpanId ? "open" : "select", id });
    };
    const onKey = (e) => {
      if (e.key === "Escape") {
        if (findOpen) dispatch({ type: "find", open: false });
        else if (drawerSpanId) dispatch({ type: "close" });
        return;
      }
      if ((e.ctrlKey || e.metaKey) && (e.key === "f" || e.key === "F")) {
        e.preventDefault();
        dispatch({ type: "find", open: true });
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey || isTyping(e.target)) return;
      switch (e.key) {
        case "1":
        case "2":
          dispatch({ type: "view", view: VIEW_KEYS[e.key] });
          break;
        case "j":
          move(index.steps, 1);
          break;
        case "k":
          move(index.steps, -1);
          break;
        case "n":
          move(index.toolCalls, 1);
          break;
        case "N":
          move(index.toolCalls, -1);
          break;
        case "e":
          move(index.problems, 1);
          break;
        case "E":
          move(index.problems, -1);
          break;
        case "g":
          move(index.gatewayEvents, 1);
          break;
        case "w":
          dispatch({ type: "toggleExpandAll" });
          break;
        case "/":
          e.preventDefault();
          dispatch({ type: "find", open: true });
          break;
        case "Enter":
          if (!inSpanView(e.target)) return;
          if (selectedId && !drawerSpanId) dispatch({ type: "open", id: selectedId });
          break;
        default:
          return;
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [index, selectedId, drawerSpanId, findOpen, dispatch]);
};

export default useSessionKeyboard;
