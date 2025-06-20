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
