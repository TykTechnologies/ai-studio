import React, { useEffect, useMemo, useRef } from 'react';
import { Box, Typography } from '@mui/material';
import { styled } from '@mui/material/styles';
import { useAui } from '@assistant-ui/react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { renderGenerativeUI, defaultGenerativeUILibrary } from '@assistant-ui/react-generative-ui';
import { toGenerativeTree } from './generativeTree';
import { safeBackground, safeImageSrc } from './generativeSafety';

/**
 * Generative UI: the "present" client tool. The model composes a tree of
 * components from a fixed vocabulary (cards, facts, tables, charts, forms,
 * buttons, ...) and the chat draws it. The call needs no user input, so it
 * resolves itself as soon as the arguments have fully arrived; interactive
 * elements (`$action`) send their payload back to the model as the next
 * user message.
 *
 * The vocabulary comes from @assistant-ui/react-generative-ui and renders
 * semantic HTML tagged with `data-aui` attributes, styled below with the
 * platform theme. Its JSON schema is embedded in the backend
 * (models/generative_ui_present_schema.json) so the model sees exactly what
 * this renderer understands.
 */

const Markdown = ({ value }) => (
  <Box data-aui="markdown" sx={{ '& p': { m: 0, mb: 1 }, '& p:last-child': { mb: 0 } }}>
    <ReactMarkdown remarkPlugins={[remarkGfm]}>{typeof value === 'string' ? value : ''}</ReactMarkdown>
  </Box>
);

/**
 * The default vocabulary with a real markdown renderer (react-markdown
 * without raw HTML, so model text cannot inject markup), a Button that also
 * accepts `text` (the flat schema tempts models into using Header's prop
 * name for the label), and guards on the props that reach attributes or
 * inline styles: image sources are limited to http(s), same-origin paths
 * and inline images; backgrounds to colours and gradients.
 */
export const studioGenerativeLibrary = {
  ...defaultGenerativeUILibrary,
  Markdown: { ...defaultGenerativeUILibrary.Markdown, render: ({ value }) => <Markdown value={value} /> },
  Button: {
    ...defaultGenerativeUILibrary.Button,
    render: ({ label, text, value, ...rest }) => defaultGenerativeUILibrary.Button.render({ label: label ?? text ?? value, ...rest }),
  },
  Image: {
    ...defaultGenerativeUILibrary.Image,
    render: ({ src, alt, ...rest }) => {
      const safe = safeImageSrc(src);
      if (!safe) return <span data-aui="image-blocked">{typeof alt === 'string' ? alt : ''}</span>;
      return defaultGenerativeUILibrary.Image.render({ src: safe, alt, ...rest });
    },
  },
  Card: {
    ...defaultGenerativeUILibrary.Card,
    render: ({ background, ...rest }) => defaultGenerativeUILibrary.Card.render({ background: safeBackground(background), ...rest }),
  },
  Box: {
    ...defaultGenerativeUILibrary.Box,
    render: ({ background, ...rest }) => defaultGenerativeUILibrary.Box.render({ background: safeBackground(background), ...rest }),
  },
};

/**
 * The arguments are final once the part stops running. A parked client
 * tool sits at `requires-action`; a call replayed from history is
 * `complete`; a cancelled run is `incomplete`.
 */
const argsReady = (status) => Boolean(status) && status.type !== 'running';

const gapRules = () => {
  const rules = {};
  for (let i = 0; i <= 8; i += 1) {
    rules[`& [data-aui-gap="${i}"]`] = { gap: `${i * 4}px` };
    rules[`& [data-aui="card"][data-aui-padding="${i}"]`] = { padding: `${i * 4}px` };
  }
  return rules;
};

const Surface = styled(Box)(({ theme }) => {
  const t = theme.palette;
  const border = `1px solid ${t.divider}`;
  const control = {
    font: 'inherit',
    fontSize: '0.9rem',
    padding: '6px 10px',
    borderRadius: 6,
    border,
    backgroundColor: t.background.paper,
    color: t.text.primary,
  };
  return {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(1.5),
    fontSize: '0.95rem',
    lineHeight: 1.5,
    color: t.text.primary,
    ...gapRules(),

    '& [data-aui="header"]': { margin: 0, fontWeight: 600, lineHeight: 1.3 },
    '& [data-aui="header"][data-aui-size="xs"]': { fontSize: '0.8rem' },
    '& [data-aui="header"][data-aui-size="sm"]': { fontSize: '0.95rem' },
    '& [data-aui="header"][data-aui-size="md"]': { fontSize: '1.05rem' },
    '& [data-aui="header"][data-aui-size="lg"]': { fontSize: '1.25rem' },
    '& [data-aui="header"][data-aui-size="xl"]': { fontSize: '1.5rem' },
    '& [data-aui="text"]': { display: 'inline' },
    '& [data-aui="text"][data-aui-size="xs"]': { fontSize: '0.75rem' },
    '& [data-aui="text"][data-aui-size="sm"]': { fontSize: '0.85rem' },
    '& [data-aui="text"][data-aui-size="lg"]': { fontSize: '1.15rem' },
    '& [data-aui="text"][data-aui-size="xl"]': { fontSize: '1.4rem' },
    '& [data-aui="text"][data-aui-weight="medium"]': { fontWeight: 500 },
    '& [data-aui="text"][data-aui-weight="semibold"]': { fontWeight: 600 },
    '& [data-aui="text"][data-aui-weight="bold"]': { fontWeight: 700 },
    '& [data-aui="text"][data-aui-color="emphasis"]': { color: t.primary.main },
    '& [data-aui="text"][data-aui-color="secondary"]': { color: t.text.secondary },
    '& [data-aui="text"][data-aui-color="alpha-70"]': { opacity: 0.7 },
    '& [data-aui="text"][data-aui-color="white"]': { color: '#fff' },
    '& [data-aui="text"][data-aui-color="white-70"]': { color: 'rgba(255,255,255,0.7)' },
    '& [data-aui="text"][data-aui-color="white-50"]': { color: 'rgba(255,255,255,0.5)' },
    '& [data-aui="caption"]': { margin: 0, fontSize: '0.8rem', color: t.text.secondary },

    '& [data-aui="card"]': { display: 'flex', flexDirection: 'column', gap: theme.spacing(1), minWidth: 0 },
    '& [data-aui="card"][data-aui-surface]': {
      border,
      borderRadius: 10,
      padding: theme.spacing(2),
      backgroundColor: t.background.paper,
    },
    '& [data-aui="card"][data-aui-background]': { border: 'none' },
    '& [data-aui="card-title"]': { fontWeight: 600, fontSize: '1.05rem' },
    '& [data-aui="card-footer"]': { display: 'flex', gap: theme.spacing(1), marginTop: theme.spacing(1) },
    '& [data-aui="col"]': { display: 'flex', flexDirection: 'column', minWidth: 0 },
    '& [data-aui="row"]': { display: 'flex', flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', minWidth: 0 },
    '& [data-aui-align="start"]': { alignItems: 'flex-start' },
    '& [data-aui-align="center"]': { alignItems: 'center' },
    '& [data-aui-align="end"]': { alignItems: 'flex-end' },
    '& [data-aui-align="stretch"]': { alignItems: 'stretch' },
    '& [data-aui-justify="start"]': { justifyContent: 'flex-start' },
    '& [data-aui-justify="center"]': { justifyContent: 'center' },
    '& [data-aui-justify="end"]': { justifyContent: 'flex-end' },
    '& [data-aui-justify="between"]': { justifyContent: 'space-between' },
    '& [data-aui-justify="around"]': { justifyContent: 'space-around' },
    '& [data-aui="spacer"]': { flex: 1 },
    '& [data-aui="box"]': { minWidth: 0 },
    '& [data-aui="divider"]': { border: 'none', borderTop: border, margin: `${theme.spacing(1)} 0`, width: '100%' },
    '& [data-aui="badge"]': {
      display: 'inline-flex',
      alignItems: 'center',
      padding: '1px 8px',
      borderRadius: 999,
      fontSize: '0.75rem',
      fontWeight: 500,
      backgroundColor: t.primary.main,
      color: t.primary.contrastText,
    },
    '& [data-aui="badge"][data-aui-variant="secondary"], & [data-aui="badge"][data-aui-variant="outline"]': {
      backgroundColor: 'transparent',
      color: t.text.primary,
      border,
    },
    '& [data-aui="badge"][data-aui-variant="success"]': { backgroundColor: t.success.main, color: t.success.contrastText },
    '& [data-aui="badge"][data-aui-variant="warning"]': { backgroundColor: t.warning.main, color: t.warning.contrastText },
    '& [data-aui="badge"][data-aui-variant="danger"], & [data-aui="badge"][data-aui-variant="error"]': {
      backgroundColor: t.error.main,
      color: t.error.contrastText,
    },

    '& [data-aui="fact"]': { margin: 0, display: 'flex', flexDirection: 'column', gap: 2, minWidth: 96 },
    '& [data-aui="fact-label"]': { fontSize: '0.75rem', color: t.text.secondary, textTransform: 'uppercase', letterSpacing: 0.4 },
    '& [data-aui="fact-value"]': { margin: 0, fontSize: '1.15rem', fontWeight: 600 },

    '& [data-aui="table"]': { borderCollapse: 'collapse', width: '100%', fontSize: '0.875rem' },
    '& [data-aui="table"] th, & [data-aui="table"] td': { padding: '6px 10px', borderBottom: border, textAlign: 'left' },
    '& [data-aui="table"] th': { fontWeight: 600, color: t.text.secondary, fontSize: '0.75rem', textTransform: 'uppercase' },

    '& [data-aui="alert"]': { padding: theme.spacing(1.5), borderRadius: 8, borderLeft: '4px solid', backgroundColor: t.background.paper, border: border },
    '& [data-aui="alert"][data-aui-tone="info"]': { borderLeftColor: t.info.main, backgroundColor: 'rgba(2,136,209,0.06)' },
    '& [data-aui="alert"][data-aui-tone="success"]': { borderLeftColor: t.success.main, backgroundColor: 'rgba(46,125,50,0.06)' },
    '& [data-aui="alert"][data-aui-tone="warning"]': { borderLeftColor: t.warning.main, backgroundColor: 'rgba(237,108,2,0.08)' },
    '& [data-aui="alert"][data-aui-tone="danger"], & [data-aui="alert"][data-aui-tone="error"]': { borderLeftColor: t.error.main, backgroundColor: 'rgba(211,47,47,0.06)' },
    '& [data-aui="alert-title"]': { fontWeight: 600, marginBottom: 2 },
    '& [data-aui="alert-desc"]': { margin: 0, fontSize: '0.9rem' },

    '& [data-aui="listview"]': { listStyle: 'none', margin: 0, padding: 0, border, borderRadius: 8, overflow: 'hidden' },
    '& [data-aui="listview-item"]': { padding: '8px 12px', borderBottom: border },
    '& [data-aui="listview-item"]:last-child': { borderBottom: 'none' },
    '& [data-aui="listview-item-trigger"]': { cursor: 'pointer', outline: 'none' },
    '& [data-aui="listview-item-trigger"]:hover': { backgroundColor: 'rgba(0,0,0,0.03)' },

    '& [data-aui="carousel"]': { display: 'flex', gap: theme.spacing(1.5), overflowX: 'auto', paddingBottom: theme.spacing(0.5) },
    '& [data-aui="carousel-slide"]': { flex: '0 0 260px' },
    '& [data-aui="carousel-slide"] [data-aui="card"]': { border, borderRadius: 10, padding: theme.spacing(2), height: '100%', backgroundColor: t.background.paper },

    '& [data-aui="image"]': { maxWidth: '100%', borderRadius: 8, display: 'block' },
    '& [data-aui="image"][data-aui-size="sm"]': { maxWidth: 120 },
    '& [data-aui="image"][data-aui-size="md"]': { maxWidth: 240 },
    '& [data-aui="image"][data-aui-size="lg"]': { maxWidth: 480 },
    '& [data-aui="image"][data-aui-round]': { borderRadius: '50%', objectFit: 'cover' },
    '& [data-aui="icon"]': { verticalAlign: 'middle' },

    '& [data-aui="button"]': {
      ...control,
      cursor: 'pointer',
      fontWeight: 500,
      backgroundColor: t.primary.main,
      color: t.primary.contrastText,
      border: 'none',
    },
    '& [data-aui="button"]:hover': { filter: 'brightness(0.95)' },
    '& [data-aui="button"][data-aui-style="secondary"], & [data-aui="button"][data-aui-style="outline"], & [data-aui="card-cancel"]': {
      backgroundColor: 'transparent',
      color: t.text.primary,
      border,
    },
    '& [data-aui="button"][data-aui-style="ghost"], & [data-aui="button"][data-aui-style="link"]': {
      backgroundColor: 'transparent',
      color: t.primary.main,
      border: 'none',
      padding: '2px 4px',
    },
    '& [data-aui="button"][data-aui-style="danger"], & [data-aui="button"][data-aui-style="destructive"]': { backgroundColor: t.error.main, color: t.error.contrastText },
    '& [data-aui="button"][data-aui-block]': { width: '100%' },
    '& [data-aui="card-confirm"]': { ...control, cursor: 'pointer', fontWeight: 500, backgroundColor: t.primary.main, color: t.primary.contrastText, border: 'none' },
    '& [data-aui="card-cancel"]': { ...control, cursor: 'pointer' },
    '& [data-aui="select"], & [data-aui="input"], & [data-aui="datepicker"]': { ...control, minWidth: 160 },
    '& textarea[data-aui="input"]': { minHeight: 72, resize: 'vertical', width: '100%' },
    '& [data-aui="checkbox"], & [data-aui="radiogroup-option"]': { display: 'inline-flex', alignItems: 'center', gap: 6, cursor: 'pointer' },
    '& [data-aui="radiogroup"]': { border: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 4 },
    '& [data-aui="form"]': { display: 'flex', flexDirection: 'column', gap: theme.spacing(1) },

    '& [data-aui="chart"]': { width: '100%', height: 120, color: t.primary.main, display: 'block' },
    '& [data-aui="chart"][data-aui-color="secondary"]': { color: t.secondary.main },
    '& [data-aui="chart-frame"]': {
      display: 'grid',
      gridTemplateColumns: 'auto 1fr',
      gridTemplateRows: 'auto auto auto',
      columnGap: 8,
      rowGap: 4,
      fontSize: '0.7rem',
      color: t.text.secondary,
    },
    '& [data-aui="chart-ticks"]': { display: 'flex', flexDirection: 'column', justifyContent: 'space-between', textAlign: 'right', gridRow: 1 },
    '& [data-aui="chart-frame"] [data-aui="chart"]': { gridRow: 1, gridColumn: 2 },
    '& [data-aui="chart-xlabels"]': { gridRow: 2, gridColumn: 2, display: 'flex', justifyContent: 'space-between' },
    '& [data-aui="chart-legend"]': { gridRow: 3, gridColumn: '1 / -1', display: 'flex', gap: 12, flexWrap: 'wrap' },
    '& [data-aui="chart-legend-item"]': { display: 'inline-flex', alignItems: 'center', gap: 4 },
    '& [data-aui="chart-legend-swatch"]': { width: 10, height: 10, borderRadius: 2, backgroundColor: 'currentColor' },
    '& [data-aui-series="0"]': { color: t.primary.main },
    '& [data-aui-series="1"]': { color: t.secondary.main },
    '& [data-aui-series="2"]': { color: t.success.main },
    '& [data-aui-series="3"]': { color: t.warning.main },
    '& [data-aui-series="4"]': { color: t.info.main },
  };
});

const summariseAction = (payload) => {
  const { type, $input, ...rest } = payload || {};
  const parts = [];
  if (type) parts.push(type);
  if ($input !== undefined) parts.push(typeof $input === 'string' ? $input : JSON.stringify($input));
  const others = Object.keys(rest).length ? JSON.stringify(rest) : '';
  return { parts, others };
};

/**
 * Turns an `$action` fired by an interactive element into the next user
 * message, so the model sees what the person chose.
 */
const useActionDispatch = () => {
  const aui = useAui();
  return useMemo(
    () => (payload) => {
      const { parts, others } = summariseAction(payload);
      const text = ['Action:', ...parts, others].filter(Boolean).join(' ');
      aui.thread().append({ role: 'user', content: [{ type: 'text', text }] });
    },
    [aui],
  );
};

/** Renders one `present` call. */
export const GenerativeUiCard = ({ args, result, status, addResult }) => {
  const dispatch = useActionDispatch();
  const done = argsReady(status) || result !== undefined;
  const answered = useRef(false);

  // Frontend tool: resolve as soon as the arguments have fully arrived so
  // the model can carry on. Only a parked call (requires-action) needs an
  // answer; replayed and cancelled calls are left alone.
  useEffect(() => {
    if (status?.type === 'requires-action' && result === undefined && !answered.current) {
      answered.current = true;
      addResult?.({});
    }
  }, [status?.type, result, addResult]);

  const tree = useMemo(() => {
    try {
      return renderGenerativeUI(toGenerativeTree(args), studioGenerativeLibrary, { status: done ? 'done' : 'streaming', dispatch });
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error('generative UI failed to render', e);
      return <Typography variant="bodySmallDefault" color="error">This content could not be displayed.</Typography>;
    }
  }, [args, done, dispatch]);

  if (status?.type === 'incomplete' && result === undefined) return null;
  return (
    <Surface data-aui="root" data-testid="generative-ui" sx={{ my: 1 }}>
      {tree}
    </Surface>
  );
};

/** Registry factory: one renderer per `present`-kind client tool. */
export const makePresentRenderer = (info) => {
  const Renderer = (props) => <GenerativeUiCard {...props} />;
  Renderer.displayName = `GenerativeUI(${info?.name || 'present'})`;
  return Renderer;
};

export default GenerativeUiCard;
