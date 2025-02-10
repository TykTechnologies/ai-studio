export const PROVIDER_TYPES = {
  TYK_DASHBOARD: "tyk",
  DIRECT_IMPORT: "direct",
};

export const STEPS = {
  SELECT_PROVIDER: "SELECT_PROVIDER",
  // Tyk Dashboard steps
  CONFIGURE_PROVIDER: "CONFIGURE_PROVIDER",
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
    STEPS.CONFIGURE_PROVIDER,
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
  [STEPS.CONFIGURE_PROVIDER]: "Configure Provider",
  [STEPS.SELECT_API]: "Select API",
  [STEPS.DIRECT_IMPORT]: "Import Specification",
  [STEPS.CONFIGURE_TOOL]: "Configure Tool",
};
