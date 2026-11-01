import React from "react";
import { Box, Typography } from "@mui/material";
import { MARKER_KINDS, markerCounts, tokenize, utf8Length } from "./promptTemplateText";

// The exact bytes of a template constant, with every character a reviewer
// could miss made visible: invisible and non-printing characters are
// replaced by a labelled marker (the character itself is never rendered,
// so a bidi control cannot reorder the text around it), and any other
// character outside ASCII is shown with its code point next to it.

const markerSx = (kind) => ({
  display: "inline-block",
  fontSize: "0.75em",
  lineHeight: 1.4,
  px: 0.5,
  mx: 0.25,
  borderRadius: 0.5,
  border: "1px solid",
  verticalAlign: "baseline",
  whiteSpace: "nowrap",
  ...(kind === "lookalike"
    ? { borderColor: "warning.main", color: "text.primary", bgcolor: "rgba(237, 108, 2, 0.08)" }
    : { borderColor: "error.main", color: "error.main", bgcolor: "rgba(211, 47, 47, 0.08)", fontWeight: 600 }),
});

const isMark = (ch) => /\p{M}/u.test(ch);

/** Marker is one character made visible. */
export const Marker = ({ token }) => {
  const kindLabel = MARKER_KINDS[token.kind];
  // A combining mark would join the marker's border: show its code only.
  const glyph = token.kind === "lookalike" && !isMark(token.char) ? token.char : null;
  return (
    <Box
      component="span"
      data-testid="char-marker"
      data-kind={token.kind}
      title={`${kindLabel}: ${token.label}`}
      aria-label={`${kindLabel} ${token.label}`}
      sx={markerSx(token.kind)}
    >
      {glyph ? `${glyph} ${token.label}` : `${kindLabel === "non-ASCII" ? "combining" : kindLabel} ${token.label}`}
    </Box>
  );
};

const RETURN_SYMBOL = String.fromCharCode(0x21b5);
const MIDDLE_DOT = String.fromCharCode(0xb7);
const faintSx = { color: "text.disabled", userSelect: "none" };

/**
 * PlainRun is ASCII text with its line ends shown (a faint return symbol)
 * and spaces at a line end shown as faint dots, since a trailing space is
 * part of the exact bytes too.
 */
const PlainRun = ({ text, atEnd }) => {
  const out = [];
  const lines = text.split("\n");
  lines.forEach((line, li) => {
    const lastLine = li === lines.length - 1;
    const trailing = lastLine && !atEnd ? null : line.match(/ +$/);
    const body = trailing ? line.slice(0, line.length - trailing[0].length) : line;
    if (body) out.push(body);
    if (trailing) {
      out.push(
        <Box component="span" key={`s${li}`} sx={faintSx} title="trailing spaces" data-testid="trailing-space">
          {MIDDLE_DOT.repeat(trailing[0].length)}
        </Box>
      );
    }
    if (!lastLine) {
      out.push(
        <Box component="span" key={`n${li}`} sx={faintSx} aria-hidden="true">
          {RETURN_SYMBOL}
        </Box>
      );
      out.push("\n");
    }
  });
  return out.map((o, i) => <React.Fragment key={i}>{o}</React.Fragment>);
};

/**
 * VisibleRun renders text inline with its markers. atEnd says the text
 * ends the constant, so its trailing spaces are marked.
 */
export const VisibleRun = ({ text, atEnd = true }) => {
  const tokens = tokenize(text);
  return tokens.map((t, i) =>
    t.type === "text" ? (
      <PlainRun key={i} text={t.text} atEnd={atEnd && i === tokens.length - 1} />
    ) : (
      <Marker key={i} token={t} />
    )
  );
};

export const monoBlockSx = {
  fontFamily: "monospace",
  fontSize: "0.85rem",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  m: 0,
  p: 1.5,
  borderRadius: 1,
  border: "1px solid",
  borderColor: "divider",
  bgcolor: "rgba(0, 0, 0, 0.03)",
  maxHeight: 360,
  overflow: "auto",
};

/** MarkerSummary says what a constant holds beyond plain ASCII. */
export const MarkerSummary = ({ text }) => {
  const counts = markerCounts(text);
  const kinds = Object.keys(counts);
  const bytes = utf8Length(text);
  if (kinds.length === 0) {
    return (
      <Typography variant="caption" color="text.secondary" data-testid="marker-summary">
        {bytes} bytes, plain ASCII.
      </Typography>
    );
  }
  const parts = kinds.map((k) => `${counts[k]} ${MARKER_KINDS[k]}`);
  return (
    <Typography variant="caption" color="warning.main" data-testid="marker-summary">
      {bytes} bytes. Marked: {parts.join(", ")}.
    </Typography>
  );
};

/** VisibleText is a constant's exact bytes in a monospace block. */
const VisibleText = ({ text, label }) => (
  <Box>
    <Box component="pre" sx={monoBlockSx} aria-label={label} data-testid="visible-text">
      <VisibleRun text={text} />
    </Box>
    <MarkerSummary text={text} />
  </Box>
);

export default VisibleText;
