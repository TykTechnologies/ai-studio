import React, { memo, useState } from 'react';
import { Box, IconButton, Typography } from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import CheckIcon from '@mui/icons-material/Check';
import { MarkdownTextPrimitive } from '@assistant-ui/react-markdown';
import remarkGfm from 'remark-gfm';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { a11yDark } from 'react-syntax-highlighter/dist/cjs/styles/prism';

const CodeHeader = ({ language, code }) => {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 800);
  };
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        px: 1.5,
        py: 0.25,
        bgcolor: '#1f2430',
        color: '#ddd',
        borderTopLeftRadius: 6,
        borderTopRightRadius: 6,
        fontFamily: 'monospace',
        fontSize: '0.75rem',
      }}
    >
      <span>{language || 'text'}</span>
      <IconButton size="small" onClick={copy} aria-label="Copy code" sx={{ color: copied ? '#0af20a' : '#ddd' }}>
        {copied ? <CheckIcon fontSize="inherit" /> : <ContentCopyIcon fontSize="inherit" />}
      </IconButton>
    </Box>
  );
};

const PrismHighlighter = ({ language, code }) => (
  <SyntaxHighlighter
    style={a11yDark}
    language={language || 'text'}
    PreTag="div"
    showLineNumbers={Boolean(language) && language !== 'text'}
    customStyle={{
      margin: 0,
      maxWidth: '100%',
      overflowX: 'auto',
      wordBreak: 'break-word',
      whiteSpace: 'pre-wrap',
      borderTopLeftRadius: 0,
      borderTopRightRadius: 0,
    }}
  >
    {code}
  </SyntaxHighlighter>
);

const Link = ({ node, children, ...props }) => (
  <a target="_blank" rel="noopener noreferrer" {...props}>
    {children}
  </a>
);

const markdownComponents = {
  SyntaxHighlighter: PrismHighlighter,
  CodeHeader,
  a: Link,
};

const remarkPlugins = [remarkGfm];

/**
 * Text part renderer: GitHub-flavoured markdown with highlighted, copyable
 * code fences, smooth token reveal while streaming, styled with MUI so the
 * branding typography applies.
 */
const MarkdownText = memo(() => (
  <Typography
    component="div"
    sx={{
      fontSize: '1rem',
      lineHeight: 1.6,
      overflowWrap: 'break-word',
      '& p': { my: 1 },
      '& p:first-of-type': { mt: 0 },
      '& p:last-of-type': { mb: 0 },
      '& ul, & ol': { pl: 3, my: 1 },
      '& li + li': { mt: 0.5 },
      '& h1, & h2, & h3, & h4': { mt: 2, mb: 1, lineHeight: 1.3 },
      '& blockquote': {
        borderLeft: '3px solid',
        borderColor: 'border.neutralDefault',
        pl: 2,
        ml: 0,
        color: 'text.secondary',
      },
      '& table': { borderCollapse: 'collapse', my: 1, display: 'block', overflowX: 'auto' },
      '& th, & td': { border: '1px solid', borderColor: 'border.neutralDefault', px: 1, py: 0.5 },
      '& th': { bgcolor: 'background.neutralDefault' },
      '& code:not(pre code)': {
        px: 0.5,
        py: 0.25,
        bgcolor: 'rgb(240, 240, 240)',
        borderRadius: '3px',
        fontFamily: 'monospace',
        fontSize: '0.95em',
      },
      '& pre': { my: 1, borderRadius: '6px', overflow: 'hidden' },
      '& hr': { border: 0, borderTop: '1px solid', borderColor: 'border.neutralDefault', my: 2 },
      '& img': { maxWidth: '100%' },
    }}
  >
    <MarkdownTextPrimitive remarkPlugins={remarkPlugins} components={markdownComponents} smooth />
  </Typography>
));

MarkdownText.displayName = 'MarkdownText';

export default MarkdownText;
