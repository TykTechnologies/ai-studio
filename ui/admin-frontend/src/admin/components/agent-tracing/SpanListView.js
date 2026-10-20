import React, { useEffect, useRef } from "react";
import { FixedSizeList } from "react-window";
import { Box, Chip, Stack, Typography } from "@mui/material";
import { useSessionDetail } from "./SessionDetailContext";
import { KIND_LABELS, durationMs, isProblem, ms } from "./spanModel";
import { formatDuration } from "./sessionFilters";

// A plain chronological span list on the shared selection model: the main
// pane until the Transcript (jyx.30) and Timeline (jyx.31) views replace it.
// Virtualised, so the DOM holds only the visible rows however many spans
// the session has.

export const ROW_HEIGHT = 36;

const SpanListView = ({ height = 480 }) => {
  const { index, selectedId, detail, dispatch } = useSessionDetail();
  const list = useRef(null);
  const t0 = ms(detail.session.started_at);

  useEffect(() => {
    const at = selectedId ? index.byId.get(selectedId)?.index : undefined;
    if (at !== undefined) list.current?.scrollToItem(at, "smart");
  }, [selectedId, index]);

  const Row = ({ index: i, style }) => {
    const s = index.ordered[i];
    const selected = s.span_id === selectedId;
    return (
      <Box
        style={style}
        role="row"
        aria-selected={selected}
        data-testid="span-row"
        data-span-id={s.span_id}
        onClick={() => dispatch({ type: "open", id: s.span_id })}
        sx={{
          display: "flex",
          alignItems: "center",
          px: 1,
          cursor: "pointer",
          borderBottom: 1,
          borderColor: "divider",
          bgcolor: selected ? "action.selected" : "transparent",
          "&:hover": { bgcolor: "action.hover" },
        }}
      >
        <Typography variant="caption" color="text.secondary" sx={{ width: 72, flexShrink: 0 }}>
          +{formatDuration(Math.max(0, ms(s.start_time) - t0))}
        </Typography>
        <Stack direction="row" spacing={1} alignItems="center" sx={{ flexGrow: 1, minWidth: 0 }}>
          <Chip size="small" variant="outlined" label={KIND_LABELS[s.kind] || s.kind} />
          <Typography variant="body2" noWrap color={isProblem(s) ? "error" : "text.primary"}>
            {s.name}
          </Typography>
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ width: 80, textAlign: "right", flexShrink: 0 }}>
          {s.end_time ? formatDuration(durationMs(s)) : "…"}
        </Typography>
      </Box>
    );
  };

  if (!index.ordered.length) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
        No spans in this range.
      </Typography>
    );
  }
  return (
    <Box
      role="grid"
      aria-label="Session spans"
      aria-rowcount={index.ordered.length}
      tabIndex={0}
      data-span-view="true"
      sx={{ "&:focus-visible": { outline: 2, outlineColor: "primary.main", outlineStyle: "solid" } }}
    >
      <FixedSizeList ref={list} height={height} itemCount={index.ordered.length} itemSize={ROW_HEIGHT} width="100%">
        {Row}
      </FixedSizeList>
    </Box>
  );
};

export default SpanListView;
