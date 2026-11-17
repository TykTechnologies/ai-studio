import React from "react";
import { Typography, Box } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import RelationshipPicker from "../../common/relationship-picker";
import CustomNote from "../../common/CustomNote";
import { getFeatureFlags } from "../../../utils/featureUtils";
import OtherWorkspacesNote from "../../catalogues/OtherWorkspacesNote";
import { usePermissions } from "../../../context/PermissionsContext";
import { P } from "../../../rbac/permissions";

// Catalog options and selections are `{ value, label }` pairs (see
// useCatalogsSelection), hence idField="value" and the label accessor.
const catalogLabel = (catalog) => catalog?.label ?? "";

// Workspaces (Enterprise): the catalog lists and the team's links hold only
// the catalogs the caller can see; a save keeps the team's links to the
// others, so each picker says so to a caller without a global read.
const hiddenNote = (kind) => `${kind} in Workspaces you cannot see are not listed; saving keeps this team's links to them.`;

const GroupCatalogsSection = ({
  catalogs,
  selectedCatalogs,
  onCatalogsChange,
  
  dataCatalogs,
  selectedDataCatalogs,
  onDataCatalogsChange,
  
  toolCatalogs,
  selectedToolCatalogs,
  onToolCatalogsChange,
  
  loading,
  features
}) => {
  const { isPortalEnabled, isChatEnabled } = getFeatureFlags(features);
  const { rbacEnabled, can } = usePermissions();
  const mayHideSome =
    Boolean(rbacEnabled) &&
    ((isPortalEnabled && !can(P.CATALOGUES_READ)) ||
      !can(P.DATA_CATALOGUES_READ) ||
      (isChatEnabled && !can(P.TOOL_CATALOGUES_READ)));
  const hasNoCatalogs = (!catalogs || catalogs.length === 0) &&
                        (!dataCatalogs || dataCatalogs.length === 0) &&
                        (!toolCatalogs || toolCatalogs.length === 0);

  return (
    <CollapsibleSection title="Add catalogs" defaultExpanded={false}>
      {hasNoCatalogs ? (
        <CustomNote
          message={
            mayHideSome
              ? "There are no catalogs in your Workspaces. Catalogs in other Workspaces are not listed; saving keeps this team's links to them."
              : "Currently, there are no catalogs available. To create a new one, please go to the Catalogs."
          }
        />
      ) : (
        <>
        <Box sx={{ mt: -2, mb: 2 }}>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Select one or more catalogs to make available to this team
          </Typography>
        </Box>
        
        {isPortalEnabled && (
          <Box sx={{ mb: 3 }}>
            <RelationshipPicker
              label="Model provider catalogs"
              itemLabel="model provider catalog"
              idField="value"
              value={selectedCatalogs}
              onChange={onCatalogsChange}
              options={catalogs}
              getOptionLabel={catalogLabel}
              disabled={loading}
            />
            <OtherWorkspacesNote permission={P.CATALOGUES_READ} sx={{ mt: 1 }}>
              {hiddenNote("Model provider catalogs")}
            </OtherWorkspacesNote>
          </Box>
        )}
        
        <Box sx={{ mb: 3 }}>
          <RelationshipPicker
            label="Data sources catalogs"
            itemLabel="data catalog"
            idField="value"
            value={selectedDataCatalogs}
            onChange={onDataCatalogsChange}
            options={dataCatalogs}
            getOptionLabel={catalogLabel}
            disabled={loading}
          />
          <OtherWorkspacesNote permission={P.DATA_CATALOGUES_READ} sx={{ mt: 1 }}>
            {hiddenNote("Data catalogs")}
          </OtherWorkspacesNote>
        </Box>
        
        {isChatEnabled && (
          <Box sx={{ mb: 2 }}>
            <RelationshipPicker
              label="Tools catalogs"
              itemLabel="tool catalog"
              idField="value"
              value={selectedToolCatalogs}
              onChange={onToolCatalogsChange}
              options={toolCatalogs}
              getOptionLabel={catalogLabel}
              disabled={loading}
            />
            <OtherWorkspacesNote permission={P.TOOL_CATALOGUES_READ} sx={{ mt: 1 }}>
              {hiddenNote("Tool catalogs")}
            </OtherWorkspacesNote>
          </Box>
        )}
        </>
      )}
    </CollapsibleSection>
  );
};

export default GroupCatalogsSection;