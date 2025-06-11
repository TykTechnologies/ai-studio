import React from 'react';
import BaseDrawer from './base-drawer';
import useSystemFeatures from '../../hooks/useSystemFeatures';
import useUserEntitlements from '../../hooks/useUserEntitlements';
import Icon from '../../../components/common/Icon';

const PortalDrawer = ({ catalogues, dataCatalogues, toolCatalogues, open }) => {
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
        icon: <Icon name="house" />,
        path: '/portal/dashboard'
      },
      {
        id: 'my-apps',
        text: 'Apps',
        icon: <Icon name="grid-2-plus" />,
        path: '/portal/apps'
      },
      {
        id: 'catalogs',
        text: 'Catalogs',
        icon: <Icon name="rectangle-history" />,
        subItems: [
          {
            id: 'llms',
            text: 'LLM providers',
            subItems: userEntitlements?.catalogues?.map(catalogue => ({
              id: `llm-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/llms/${catalogue.id}`
            })) || []
          },
          {
            id: 'data-sources',
            text: 'Data sources',
            subItems: userEntitlements?.data_catalogues?.map(catalogue => ({
              id: `db-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/databases/${catalogue.id}`
            })) || []
          },
          {
            id: 'tools',
            text: 'Tools',
            subItems: userEntitlements?.tool_catalogues?.map(catalogue => ({
              id: `tool-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/tools/${catalogue.id}`
            })) || []
          }
        ]
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
