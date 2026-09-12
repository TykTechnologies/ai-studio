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
