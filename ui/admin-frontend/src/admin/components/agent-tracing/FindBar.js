import React, { useEffect, useRef, useState } from "react";
import { Box, IconButton, Stack, TextField, Tooltip, Typography } from "@mui/material";
import KeyboardArrowUpIcon from "@mui/icons-material/KeyboardArrowUp";
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";
import CloseIcon from "@mui/icons-material/Close";
import { useSessionDetail } from "./SessionDetailContext";
import agentTracingService from "../../services/agentTracingService";

// In-session find (spec §6.2): searched on the server over span metadata
// (and, for content readers, permitted content). The term lives in this
// component's state only and goes to the API in a POST body, never into a
// URL (security review, jyx.29).

export const MAX_TERM = 256;

/**
 * Metadata matches, then content matches (content readers only), one per
 * span, in that order.
 */
export const allMatches = (result) => {
  const seen = new Set();
  return [...(result?.matches || []), ...(result?.content_matches || [])].filter((m) => {
    if (seen.has(m.span_id)) return false;
    seen.add(m.span_id);
    return true;
  });
};

const FindBar = () => {
  const { detail, findOpen, dispatch } = useSessionDetail();
  const [term, setTerm] = useState("");
  const [result, setResult] = useState(null);
  const [pos, setPos] = useState(0);
  const [error, setError] = useState(null);
  const input = useRef(null);

  useEffect(() => {
    if (findOpen) input.current?.focus();
  }, [findOpen]);

  if (!findOpen) return null;

  const go = (matches, i) => {
    if (!matches.length) return;
    const at = (i + matches.length) % matches.length;
    setPos(at);
    dispatch({ type: "select", id: matches[at].span_id });
  };

  const run = async () => {
    const q = term.trim();
    if (!q) return;
    setError(null);
    try {
      const res = await agentTracingService.search(detail.session.id, q);
      setResult({ ...res, term: q });
      go(allMatches(res), 0);
    } catch (e) {
      setResult(null);
      setError("Find failed");
    }
  };

  const matches = allMatches(result);
  const close = () => dispatch({ type: "find", open: false });

  return (
    <Box data-testid="find-bar" sx={{ mb: 1 }}>
      <Stack direction="row" spacing={1} alignItems="center">
        <TextField
          inputRef={input}
          size="small"
          label="Find in session"
          value={term}
          inputProps={{ maxLength: MAX_TERM }}
          onChange={(e) => setTerm(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              // The same term steps through its matches; a new term searches.
              if (result && result.term === term.trim() && matches.length) go(matches, pos + (e.shiftKey ? -1 : 1));
              else run();
            } else if (e.key === "Escape") {
              close();
            }
          }}
        />
        <Typography variant="body2" aria-live="polite" data-testid="find-count">
          {error ||
            (result
              ? matches.length
                ? `${pos + 1} of ${matches.length}${result.truncated ? "+" : ""}`
                : "No matches"
              : "")}
        </Typography>
        <Tooltip title="Previous match (Shift+Enter)">
          <span>
            <IconButton size="small" aria-label="Previous match" disabled={!matches.length} onClick={() => go(matches, pos - 1)}>
              <KeyboardArrowUpIcon />
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title="Next match (Enter)">
          <span>
            <IconButton size="small" aria-label="Next match" disabled={!matches.length} onClick={() => go(matches, pos + 1)}>
              <KeyboardArrowDownIcon />
            </IconButton>
          </span>
        </Tooltip>
        <IconButton size="small" aria-label="Close find" onClick={close}>
          <CloseIcon />
        </IconButton>
      </Stack>
    </Box>
  );
};

export default FindBar;
