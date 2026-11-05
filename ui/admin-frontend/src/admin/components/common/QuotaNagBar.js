import React, { useEffect, useState } from "react";
import { Box } from "@mui/material";
import { useLocation } from "react-router-dom";
import WarningBanner from "./WarningBanner";
import pubClient from "../../utils/pubClient";

// QuotaNagBar shows the near-limit notices the server sends for the signed-in
// user (licence limits for users with admin access, their own App quota for
// portal users) and dismisses them for good through the API, so a dismissal
// holds across reloads, sign-ins and browsers. Community Edition sends none.
const QuotaNagBar = () => {
  const [notices, setNotices] = useState([]);
  const { pathname } = useLocation();

  useEffect(() => {
    let cancelled = false;
    pubClient
      .get("/common/notices")
      .then((response) => {
        if (!cancelled) setNotices(response?.data?.data || []);
      })
      .catch(() => {
        if (!cancelled) setNotices([]);
      });
    return () => {
      cancelled = true;
    };
  }, [pathname]);

  const dismiss = async (key) => {
    setNotices((current) => current.filter((n) => n.key !== key));
    try {
      await pubClient.post(`/common/notices/${encodeURIComponent(key)}/dismiss`);
    } catch (err) {
      console.error("Failed to dismiss notice:", err);
    }
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
