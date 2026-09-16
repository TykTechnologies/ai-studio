export const PROVIDER_TYPES = {
  TYK_DASHBOARD: "tyk",
  DIRECT_IMPORT: "direct",
};

export const STEPS = {
  SELECT_PROVIDER: "SELECT_PROVIDER",
  // Tyk Dashboard steps: pick (or add) a saved Tyk connection, then an API
  SELECT_CONNECTION: "SELECT_CONNECTION",
  SELECT_API: "SELECT_API",
  // Direct Import steps
  DIRECT_IMPORT: "DIRECT_IMPORT",
  // Final step for both flows
  CONFIGURE_TOOL: "CONFIGURE_TOOL",
};

// Step sequences for each provider type
export const STEP_SEQUENCES = {
  [PROVIDER_TYPES.TYK_DASHBOARD]: [
    STEPS.SELECT_PROVIDER,
    STEPS.SELECT_CONNECTION,
    STEPS.SELECT_API,
    STEPS.CONFIGURE_TOOL,
  ],
  [PROVIDER_TYPES.DIRECT_IMPORT]: [
    STEPS.SELECT_PROVIDER,
    STEPS.DIRECT_IMPORT,
    STEPS.CONFIGURE_TOOL,
  ],
};

export const STEP_LABELS = {
  [STEPS.SELECT_PROVIDER]: "Select Provider",
  [STEPS.SELECT_CONNECTION]: "Choose Connection",
  [STEPS.SELECT_API]: "Select API",
  [STEPS.DIRECT_IMPORT]: "Import Specification",
  [STEPS.CONFIGURE_TOOL]: "Configure Tool",
};

// The import methods are fixed: the wizard has no server-side registry.
export const IMPORT_METHODS = [
  {
    id: "direct",
    type: PROVIDER_TYPES.DIRECT_IMPORT,
    name: "Direct Import",
    description: "Import from URL or upload OpenAPI specification file",
  },
  {
    id: "tyk",
    type: PROVIDER_TYPES.TYK_DASHBOARD,
    name: "Tyk Dashboard",
    description: "Import an API from a connected Tyk Dashboard (Enterprise)",
  },
];
