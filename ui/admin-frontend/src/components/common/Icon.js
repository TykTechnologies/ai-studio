import React, { lazy, Suspense } from 'react';
import { SvgIcon } from '@mui/material';
import PropTypes from 'prop-types';

const iconComponents = {
  'monitor-waveform': lazy(() => import('../../common/fontawesome/monitor-waveform.svg').then(module => ({ default: module.ReactComponent }))),
  'microchip-ai': lazy(() => import('../../common/fontawesome/microchip-ai.svg').then(module => ({ default: module.ReactComponent }))),
  'layer-group': lazy(() => import('../../common/fontawesome/layer-group.svg').then(module => ({ default: module.ReactComponent }))),
  'shield': lazy(() => import('../../common/fontawesome/shield.svg').then(module => ({ default: module.ReactComponent }))),
  'display': lazy(() => import('../../common/fontawesome/display.svg').then(module => ({ default: module.ReactComponent }))),
  'rectangle-history': lazy(() => import('../../common/fontawesome/rectangle-history.svg').then(module => ({ default: module.ReactComponent }))),
  'message-lines': lazy(() => import('../../common/fontawesome/message-lines.svg').then(module => ({ default: module.ReactComponent }))),
  'grid-2-plus': lazy(() => import('../../common/fontawesome/grid-2-plus.svg').then(module => ({ default: module.ReactComponent }))),
  'house': lazy(() => import('../../common/fontawesome/house.svg').then(module => ({ default: module.ReactComponent }))),
  'users': lazy(() => import('../../common/fontawesome/users.svg').then(module => ({ default: module.ReactComponent }))),
  'gear': lazy(() => import('../../common/fontawesome/gear.svg').then(module => ({ default: module.ReactComponent }))),
  'book-sparkles': lazy(() => import('../../common/fontawesome/book-sparkles.svg').then(module => ({ default: module.ReactComponent }))),
  'screwdriver-wrench': lazy(() => import('../../common/fontawesome/screwdriver-wrench.svg').then(module => ({ default: module.ReactComponent }))),
  'hexagon-exclamation': lazy(() => import('../../common/fontawesome/hexagon-exclamation.svg').then(module => ({ default: module.ReactComponent }))),
  'hexagon-check': lazy(() => import('../../common/fontawesome/hexagon-check.svg').then(module => ({ default: module.ReactComponent }))),
  'triangle-exclamation': lazy(() => import('../../common/fontawesome/triangle-exclamation.svg').then(module => ({ default: module.ReactComponent }))),
  'lock': lazy(() => import('../../common/fontawesome/lock.svg').then(module => ({ default: module.ReactComponent }))),
  'unlock': lazy(() => import('../../common/fontawesome/unlock.svg').then(module => ({ default: module.ReactComponent }))),
  'lock-keyhole': lazy(() => import('../../common/fontawesome/lock-keyhole.svg').then(module => ({ default: module.ReactComponent }))),
  'shield-keyhole': lazy(() => import('../../common/fontawesome/shield-keyhole.svg').then(module => ({ default: module.ReactComponent }))),
  'circle-info': lazy(() => import('../../common/fontawesome/circle-info.svg').then(module => ({ default: module.ReactComponent }))),
  'circle-exclamation': lazy(() => import('../../common/fontawesome/circle-exclamation.svg').then(module => ({ default: module.ReactComponent }))),
  'circle-check': lazy(() => import('../../common/fontawesome/circle-check.svg').then(module => ({ default: module.ReactComponent }))),
};

// Plugins are represented by a puzzle piece (the drawer, the portal drawer and
// the permission matrix all ask for "puzzle-piece"). The FontAwesome set above
// never included one, so the entries rendered without an icon and React logged
// an invalid-prop warning on every admin page. Material's "Extension" glyph is
// the same shape; inlining its path keeps the component's lazy-SVG contract.
const PuzzlePieceSvg = React.forwardRef((props, ref) => (
  <svg viewBox="0 0 24 24" ref={ref} {...props}>
    <path d="M20.5 11H19V7c0-1.1-.9-2-2-2h-4V3.5C13 2.12 11.88 1 10.5 1S8 2.12 8 3.5V5H4c-1.1 0-1.99.9-1.99 2v3.8H3.5c1.49 0 2.7 1.21 2.7 2.7s-1.21 2.7-2.7 2.7H2V20c0 1.1.9 2 2 2h3.8v-1.5c0-1.49 1.21-2.7 2.7-2.7s2.7 1.21 2.7 2.7V22H17c1.1 0 2-.9 2-2v-4h1.5c1.38 0 2.5-1.12 2.5-2.5S21.88 11 20.5 11" />
  </svg>
));
PuzzlePieceSvg.displayName = 'PuzzlePieceSvg';
iconComponents['puzzle-piece'] = PuzzlePieceSvg;

// MCP servers use "server" (portal catalog types). Material's "Dns" glyph,
// inlined for the same reason as the puzzle piece.
const ServerSvg = React.forwardRef((props, ref) => (
  <svg viewBox="0 0 24 24" ref={ref} {...props}>
    <path d="M20 13H4c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h16c.55 0 1-.45 1-1v-6c0-.55-.45-1-1-1M7 19c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2M20 3H4c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h16c.55 0 1-.45 1-1V4c0-.55-.45-1-1-1M7 9c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2" />
  </svg>
));
ServerSvg.displayName = 'ServerSvg';
iconComponents['server'] = ServerSvg;

// Model routers use "route" (portal catalog types). Material's "AltRoute"
// glyph, inlined for the same reason as the puzzle piece.
const RouteSvg = React.forwardRef((props, ref) => (
  <svg viewBox="0 0 24 24" ref={ref} {...props}>
    <path d="m9.78 11.16-1.42 1.42c-.68-.69-1.34-1.58-1.79-2.94l1.94-.49c.32.89.77 1.5 1.27 2.01M11 6 7 2 3 6h3.02c.02.81.08 1.54.19 2.17l1.94-.49C8.08 7.2 8.03 6.63 8.02 6zm10 0-4-4-4 4h2.99c-.1 3.68-1.28 4.75-2.54 5.88-.5.44-1.01.92-1.45 1.55-.34-.49-.73-.88-1.13-1.24L9.46 13.6c.93.85 1.54 1.54 1.54 3.4v5h2v-5c0-2.02.71-2.66 1.79-3.63 1.38-1.24 3.08-2.78 3.2-7.37z" />
  </svg>
));
RouteSvg.displayName = 'RouteSvg';
iconComponents['route'] = RouteSvg;

// Semantic routers use "psychology" (portal catalog types): they route by
// what the prompt is about. Material's "Psychology" glyph, inlined for the
// same reason as the puzzle piece.
const PsychologySvg = React.forwardRef((props, ref) => (
  <svg viewBox="0 0 24 24" ref={ref} {...props}>
    <path d="M13 8.57c-.79 0-1.43.64-1.43 1.43s.64 1.43 1.43 1.43 1.43-.64 1.43-1.43-.64-1.43-1.43-1.43" />
    <path d="M13 3C9.25 3 6.2 5.94 6.02 9.64L4.1 12.2c-.25.33-.01.8.4.8H6v3c0 1.1.9 2 2 2h1v3h7v-4.68c2.36-1.12 4-3.53 4-6.32 0-3.87-3.13-7-7-7m3 7c0 .13-.01.26-.02.39l.83.66c.08.06.1.16.05.25l-.8 1.39c-.05.09-.16.12-.24.09l-.99-.4c-.21.16-.43.29-.67.39L14 13.83c-.01.1-.1.17-.2.17h-1.6c-.1 0-.18-.07-.2-.17l-.15-1.06c-.25-.1-.47-.23-.68-.39l-.99.4c-.09.03-.2 0-.25-.09l-.8-1.39c-.05-.08-.03-.19.05-.25l.84-.66c-.01-.13-.02-.26-.02-.39s.02-.27.04-.39l-.85-.66c-.08-.06-.1-.16-.05-.26l.8-1.38c.05-.09.15-.12.24-.09l1 .4c.2-.15.43-.29.67-.39L12 6.17c.02-.1.1-.17.2-.17h1.6c.1 0 .18.07.2.17l.15 1.06c.24.1.46.23.67.39l1-.4c.09-.03.2 0 .24.09l.8 1.38c.05.09.03.2-.05.26l-.85.66c.03.12.04.25.04.39" />
  </svg>
));
PsychologySvg.displayName = 'PsychologySvg';
iconComponents['psychology'] = PsychologySvg;

const Icon = ({ name, ...svgProps }) => {
  const IconComponent = iconComponents[name];
  
  if (!IconComponent) {
    console.warn(`Icon "${name}" not found`);
    return null;
  }
  
  return (
    <Suspense fallback={null}>
      <SvgIcon component={IconComponent} inheritViewBox {...svgProps} />
    </Suspense>
  );
};

Icon.propTypes = {
  name: PropTypes.oneOf(Object.keys(iconComponents)).isRequired,
};

export default Icon;
