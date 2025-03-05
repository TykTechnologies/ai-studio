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
