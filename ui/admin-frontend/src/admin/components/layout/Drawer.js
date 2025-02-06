import React from "react";
import {
  Dashboard,
  Person,
  Group,
  People,
  SmartToy,
  Settings,
  AttachMoney,
  Storage,
  Build,
  DataObject,
  FolderOpen,
  FilterList,
  SettingsInputComponent,
  Apps,
  Web,
  Chat,
  VpnKey,
} from "@mui/icons-material";
import BaseDrawer from "./BaseDrawer";
import useSystemFeatures from "../../hooks/useSystemFeatures";

const Drawer = () => {
  const { features, loading } = useSystemFeatures();

  if (loading) {
    return null;
  }

  const getMenuItems = () => [
    { 
      id: "dashboard",
      text: "Dashboard", 
      icon: <Dashboard />, 
      path: "/admin/dash" 
    },
    {
      id: "team",
      text: "Team",
      icon: <People />,
      subItems: [
        { 
          id: "users",
          text: "Users", 
          icon: <Person />, 
          path: "/admin/users" 
        },
        ...(!features.feature_gateway ||
        features.feature_portal ||
        features.feature_chat
          ? [{ 
              id: "groups",
              text: "Groups", 
              icon: <Group />, 
              path: "/admin/groups" 
            }]
          : []),
      ],
    },
    {
      text: "AI",
      icon: <SmartToy />,
      subItems: [
        { text: "LLMs", icon: <SmartToy />, path: "/admin/llms" },
        ...(features.feature_chat
          ? [
              {
                text: "Call Settings",
                icon: <Settings />,
                path: "/admin/llm-settings",
              },
            ]
          : []),
        {
          text: "Model Prices",
          icon: <AttachMoney />,
          path: "/admin/model-prices",
        },
      ],
    },
    {
      text: "Data",
      icon: <DataObject />,
      subItems: [
        {
          text: "Vector Sources",
          icon: <Storage />,
          path: "/admin/datasources",
        },
        ...(features.feature_chat
          ? [{ text: "Tools", icon: <Build />, path: "/admin/tools" }]
          : []),
      ],
    },
    ...(features.feature_gateway
      ? [
          {
            text: "Gateway",
            icon: <SettingsInputComponent />,
            subItems: [
              {
                text: "Filters",
                icon: <FilterList />,
                path: "/admin/filters",
              },
              { text: "Secrets", icon: <VpnKey />, path: "/admin/secrets" },
            ],
          },
        ]
      : []),
    ...(features.feature_gateway &&
    !features.feature_portal &&
    !features.feature_chat
      ? [
          {
            text: "Apps and Credentials",
            icon: <Web />,
            subItems: [
              { text: "Apps", icon: <Apps />, path: "/admin/apps" },
            ],
          },
        ]
      : [
          {
            text: "Portal",
            icon: <Web />,
            subItems: [
              ...(features.feature_portal || features.feature_gateway
                ? [{ text: "Apps", icon: <Apps />, path: "/admin/apps" }]
                : []),
              ...(features.feature_chat
                ? [
                    {
                      text: "Chat Rooms",
                      icon: <Chat />,
                      path: "/admin/chats",
                    },
                  ]
                : []),
              ...(features.feature_portal || features.feature_chat
                ? [
                    {
                      text: "Catalogs",
                      icon: <FolderOpen />,
                      subItems: [
                        ...(features.feature_portal
                          ? [
                              {
                                text: "LLMs",
                                icon: <SmartToy />,
                                path: "/admin/catalogs/llms",
                              },
                            ]
                          : []),
                        {
                          text: "Data",
                          icon: <DataObject />,
                          path: "/admin/catalogs/data",
                        },
                        ...(features.feature_chat
                          ? [
                              {
                                text: "Tools",
                                icon: <Build />,
                                path: "/admin/catalogs/tools",
                              },
                            ]
                          : []),
                      ],
                    },
                  ]
                : []),
            ],
          },
        ]),
  ];

  return (
    <BaseDrawer
      menuItems={getMenuItems()}
      drawerWidth={240}
      minimizedWidth={60}
      isCollapsible={true}
      showToolbar={true}
    />
  );
};

export default Drawer;
