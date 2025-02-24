import React from 'react';
import {
  Code,
  Apps,
} from '@mui/icons-material';
import { SvgIcon } from '@mui/material';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import { ReactComponent as HouseIcon } from '../../../common/fontawesome/house.svg';

const PortalDrawer = () => {
  const { features, loading: featuresLoading } = useSystemFeatures();
  const { 
    userEntitlements, 
    uiOptions, 
    loading: entitlementsLoading 
  } = useUserEntitlements();

  if (featuresLoading || entitlementsLoading) {
    return null;
  }

  const showPortalFeatures = features.feature_portal || features.feature_gateway;

  const getMenuItems = () => {
    if (!showPortalFeatures || !uiOptions?.show_portal) {
      return [];
    }

    return [
      {
        id: 'dashboard',
        text: 'Overview',
        icon: <SvgIcon component={HouseIcon} inheritViewBox />,
        path: '/portal/dashboard'
      },
      {
        id: 'my-apps',
        text: 'Apps',
        icon: <Apps />,
        path: '/portal/apps'
      },
      {
        id: 'llms',
        text: 'LLM providers catalogs',
        icon: <Code />,
        subItems: userEntitlements?.catalogues?.map(catalogue => ({
          id: `llm-${catalogue.id}`,
          text: catalogue.attributes.name,
          path: `/portal/llms/${catalogue.id}`
        })) || []
      },
      {
        id: 'databases',
        text: 'Data sources catalogs',
        subItems: userEntitlements?.data_catalogues?.map(catalogue => ({
          id: `db-${catalogue.id}`,
          text: catalogue.attributes.name,
          path: `/portal/databases/${catalogue.id}`
        })) || []
      },
    ];
  };

  return (
    <BaseDrawer
      id="portal"
      menuItems={getMenuItems()}
      showToolbar={false}
      customStyles={{
        marginTop: '64px'
      }}
      defaultExpandedItems={{
        'resources': true,
        'llms': false,
        'databases': false
      }}
    />
  );
};

export default PortalDrawer;
