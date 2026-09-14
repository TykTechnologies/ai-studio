import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useNavigate } from "react-router-dom";
import UnsavedChangesDialog from "./UnsavedChangesDialog";

/**
 * Central registry of dirty forms plus the navigation guard that consults it.
 *
 * Forms declare their dirty state with `useUnsavedChangesGuard(isDirty)`;
 * while any registered form is dirty the provider intercepts:
 *
 *   - same-origin anchor clicks (sidebar, back arrows, notification links)
 *   - the top tabs and any other caller of `confirmNavigation`
 *   - browser back/forward (popstate)
 *   - reload / tab close (beforeunload, browser-native prompt)
 *
 * and asks before the change is discarded. The app is a plain BrowserRouter
 * (no data router), so react-router's `useBlocker` is not available; this
 * component covers the same ground at the DOM level.
 *
 * Without a provider mounted (unit tests render forms on their own) every hook
 * degrades to a no-op and `confirmNavigation` runs its callback immediately.
 */

const noop = () => {};

const defaultContext = {
  hasProvider: false,
  isDirty: false,
  register: noop,
  unregister: noop,
  clear: noop,
  confirmNavigation: (next) => {
    if (typeof next === "function") next();
  },
};

const UnsavedChangesContext = createContext(defaultContext);

// Anchor clicks that must be left alone: new tabs, downloads, modifier clicks,
// other origins, and anchors that opt out explicitly.
const shouldInterceptAnchorClick = (event, anchor) => {
  if (event.button !== 0) return false;
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return false;
  if (anchor.target && anchor.target !== "_self") return false;
  if (anchor.hasAttribute("download")) return false;
  if (anchor.hasAttribute("data-unsaved-changes-ignore")) return false;
  const href = anchor.getAttribute("href");
  if (!href || href.startsWith("#")) return false;
  // Non-http schemes (mailto:, javascript:) parse to an opaque origin and
  // fail the same-origin check below.
  let url;
  try {
    url = new URL(anchor.href, window.location.href);
  } catch (err) {
    return false;
  }
  if (url.origin !== window.location.origin) return false;
  // A link back to the page we are already on is not a navigation away.
  if (url.pathname === window.location.pathname && url.search === window.location.search) return false;
  return true;
};

const historyIndex = () => {
  const state = typeof window !== "undefined" ? window.history.state : null;
  return state && typeof state.idx === "number" ? state.idx : null;
};

// popstate must reach the guard BEFORE react-router's own listener: the
// router re-renders the other page in the microtask checkpoint between two
// window listeners, and by then the dirty form has unmounted. Browsers run
// same-target listeners in registration order (Chromium does not honour
// capture-before-bubble at the target for this), and BrowserRouter registers
// in a layout effect on mount, so the one listener the guard uses is bound
// here, at module evaluation, and providers plug their handler into it.
const popstateHandlers = new Set();
if (typeof window !== "undefined") {
  window.addEventListener(
    "popstate",
    (event) => {
      popstateHandlers.forEach((handler) => handler(event));
    },
    true
  );
}

export const UnsavedChangesProvider = ({ children }) => {
  const navigate = useNavigate();
  const registryRef = useRef(new Map());
  const [isDirty, setIsDirty] = useState(false);
  const [pending, setPending] = useState(null); // { next: () => void } while the dialog is open

  // Live read for DOM listeners, which are registered once.
  const anyDirty = useCallback(() => {
    for (const value of registryRef.current.values()) {
      if (value) return true;
    }
    return false;
  }, []);

  const recompute = useCallback(() => {
    setIsDirty(anyDirty());
  }, [anyDirty]);

  const register = useCallback(
    (id, dirty) => {
      registryRef.current.set(id, Boolean(dirty));
      recompute();
    },
    [recompute]
  );

  const unregister = useCallback(
    (id) => {
      registryRef.current.delete(id);
      recompute();
    },
    [recompute]
  );

  const clear = useCallback(() => {
    registryRef.current.clear();
    setIsDirty(false);
  }, []);

  const confirmNavigation = useCallback(
    (next) => {
      const proceed = typeof next === "function" ? next : noop;
      if (!anyDirty()) {
        proceed();
        return;
      }
      setPending({ next: proceed });
    },
    [anyDirty]
  );

  const handleStay = useCallback(() => {
    setPending(null);
  }, []);

  const handleLeave = useCallback(() => {
    const current = pending;
    setPending(null);
    // Clear first so nothing re-prompts during the navigation that follows;
    // the leaving form's own unregister runs on unmount.
    clear();
    if (current) current.next();
  }, [pending, clear]);

  // (a) Internal link clicks. Capture phase on `document` runs before React's
  // root listener, so stopping propagation here keeps <Link> from navigating.
  useEffect(() => {
    const onClick = (event) => {
      if (!anyDirty()) return;
      const target = event.target;
      if (!target || typeof target.closest !== "function") return;
      const anchor = target.closest("a[href]");
      if (!anchor || !shouldInterceptAnchorClick(event, anchor)) return;
      event.preventDefault();
      event.stopPropagation();
      const url = new URL(anchor.href, window.location.href);
      const to = `${url.pathname}${url.search}${url.hash}`;
      setPending({ next: () => navigate(to) });
    };
    document.addEventListener("click", onClick, true);
    return () => document.removeEventListener("click", onClick, true);
  }, [anyDirty, navigate]);

  // Remember which history entry the dirty form lives on, so a popstate can
  // tell how far the user moved.
  const guardIndexRef = useRef(null);
  useEffect(() => {
    guardIndexRef.current = isDirty ? historyIndex() : null;
  }, [isDirty]);

  // (c) Browser back/forward. The URL has already changed by the time
  // popstate fires. The module-level listener above runs before
  // react-router's, so stopping propagation keeps the router from rendering
  // the other page; the history is then stepped back to the form and the
  // user is asked. "Leave" replays the original step.
  const restoringRef = useRef(false);
  useEffect(() => {
    const onPopState = (event) => {
      if (restoringRef.current) {
        // This is the go(-delta) that put the form's entry back.
        restoringRef.current = false;
        event.stopImmediatePropagation();
        return;
      }
      if (!anyDirty()) return;
      const from = historyIndex();
      // Entries without a router index (or a same-entry hash change) cannot
      // be measured; assume the common case, a single step back.
      const delta = from === null || guardIndexRef.current === null ? -1 : from - guardIndexRef.current;
      if (delta === 0) return;
      event.stopImmediatePropagation();
      restoringRef.current = true;
      window.history.go(-delta);
      setPending({ next: () => window.history.go(delta) });
    };
    popstateHandlers.add(onPopState);
    return () => popstateHandlers.delete(onPopState);
  }, [anyDirty]);

  // (d) Reload / close: only registered while dirty, so a clean page never
  // shows the browser's prompt.
  useEffect(() => {
    if (!isDirty) return undefined;
    const onBeforeUnload = (event) => {
      event.preventDefault();
      // Legacy browsers read returnValue; modern ones only need preventDefault.
      event.returnValue = "";
      return "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [isDirty]);

  const value = useMemo(
    () => ({
      hasProvider: true,
      isDirty,
      register,
      unregister,
      clear,
      confirmNavigation,
    }),
    [isDirty, register, unregister, clear, confirmNavigation]
  );

  return (
    <UnsavedChangesContext.Provider value={value}>
      {children}
      <UnsavedChangesDialog open={pending !== null} onStay={handleStay} onLeave={handleLeave} />
    </UnsavedChangesContext.Provider>
  );
};

/** Full context: `{ isDirty, register, unregister, clear, confirmNavigation, hasProvider }`. */
export const useUnsavedChanges = () => useContext(UnsavedChangesContext);

/**
 * `confirmNavigation(next)`: runs `next` at once when nothing is dirty,
 * otherwise opens the prompt and runs it only on "Leave without saving".
 */
export const useConfirmNavigation = () => useContext(UnsavedChangesContext).confirmNavigation;

let guardSeq = 0;

/**
 * Declares a form's dirty state to the provider under a stable id. Re-registers
 * whenever `isDirty` changes and unregisters on unmount.
 */
export const useUnsavedChangesGuard = (isDirty) => {
  const { register, unregister } = useContext(UnsavedChangesContext);
  const idRef = useRef(null);
  if (idRef.current === null) {
    guardSeq += 1;
    idRef.current = `form-${guardSeq}`;
  }

  useEffect(() => {
    register(idRef.current, Boolean(isDirty));
  }, [isDirty, register]);

  useEffect(() => {
    const id = idRef.current;
    return () => unregister(id);
  }, [unregister]);
};

export default UnsavedChangesProvider;
