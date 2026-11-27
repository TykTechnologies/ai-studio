import React, { useEffect, useRef, useState } from "react";
import { Box } from "@mui/material";
import { useLocation } from "react-router-dom";
import WarningBanner from "./WarningBanner";
import pubClient from "../../utils/pubClient";

// PENDING_KEY holds the dismissals this browser made that the API has not
// confirmed yet. Leaving the page right after the click (a link, a reload, a
// scripted page.goto) can abort the request before it is sent; the banner
// stays hidden and the dismissal is sent again on the next load. Signing out
// clears localStorage, so one user's pending dismissals never reach another.
export const PENDING_KEY = "quotaNagBar.pendingDismissals";

// The server refuses keys over 255 characters; a user has a handful of
// notices at most. Anything else in storage is ignored.
const MAX_KEY_LENGTH = 255;
const MAX_PENDING = 20;

const readPending = () => {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(PENDING_KEY) || "[]");
    return Array.isArray(parsed)
      ? parsed.filter((k) => typeof k === "string" && k.length > 0 && k.length <= MAX_KEY_LENGTH).slice(-MAX_PENDING)
      : [];
  } catch {
    return [];
  }
};

const writePending = (keys) => {
  try {
    if (keys.length === 0) window.localStorage.removeItem(PENDING_KEY);
    else window.localStorage.setItem(PENDING_KEY, JSON.stringify(keys));
  } catch {
    // Storage unavailable: the in-memory set still hides it on this page.
  }
};

const sendDismissal = async (key) => {
  try {
    await pubClient.post(`/common/notices/${encodeURIComponent(key)}/dismiss`);
  } catch (err) {
    // 400: the notice no longer applies, so there is nothing to retry.
    if (err?.response?.status !== 400) {
      console.error("Failed to dismiss notice:", err);
      return;
    }
  }
  writePending(readPending().filter((k) => k !== key));
};

// QuotaNagBar shows the near-limit notices the server sends for the signed-in
// user (licence limits, for users with admin access) and dismisses them for
// good through the API, so a dismissal holds across reloads, sign-ins and
// browsers. Community Edition sends none.
const QuotaNagBar = () => {
  const [notices, setNotices] = useState([]);
  const { pathname } = useLocation();
  // Keys dismissed in this page's lifetime: a notices request already in
  // flight when the user clicks must not bring the banner back.
  const dismissed = useRef(new Set());

  useEffect(() => {
    let cancelled = false;
    pubClient
      .get("/common/notices")
      .then((response) => {
        if (cancelled) return;
        const list = response?.data?.data || [];
        const pending = new Set(readPending());
        // A pending dismissal the server still shows did not land: send it
        // again, and keep the banner hidden meanwhile.
        list.filter((n) => pending.has(n.key)).forEach((n) => sendDismissal(n.key));
        setNotices(list.filter((n) => !pending.has(n.key) && !dismissed.current.has(n.key)));
      })
      .catch(() => {
        if (!cancelled) setNotices([]);
      });
    return () => {
      cancelled = true;
    };
  }, [pathname]);

  const dismiss = (key) => {
    dismissed.current.add(key);
    setNotices((current) => current.filter((n) => n.key !== key));
    writePending([...new Set([...readPending(), key])]);
    sendDismissal(key);
  };

  if (notices.length === 0) return null;
  return (
    <Box data-testid="quota-nag-bar" sx={{ display: "flex", flexDirection: "column", gap: 1, mb: 2 }}>
      {notices.map((notice) => (
        <WarningBanner
          key={notice.key}
          title={notice.label}
          message={notice.message}
          onClose={() => dismiss(notice.key)}
        />
      ))}
    </Box>
  );
};

export default QuotaNagBar;
