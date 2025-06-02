import React from "react";
import DataTable from "../../../components/common/DataTable";
import { Typography } from "@mui/material";
import CatalogueBadges from "./CatalogueBadges";
import { getFeatureFlags } from "../../../utils/featureUtils";

const GroupsTable = ({
  groups,
  page,
  pageSize,
  totalPages,
  handlePageChange,
  handlePageSizeChange,
  handleSearch,
  sortConfig,
  handleSortChange,
  handleGroupClick,
  handleEdit,
  handleDelete,
  handleManageMembers,
  handleManageCatalogs,
  features = {},
}) => {
  const { isGatewayOnly } = getFeatureFlags(features);

  const baseColumns = [
    { field: "id", headerName: "ID", sortable: true },
    {
      field: "attributes.name",
      headerName: "Name",
      sortable: true,
      renderCell: (item) => item.attributes.name
    },
    {
      field: "attributes.user_count",
      headerName: "Members",
      renderCell: (item) => (
        <Typography variant="bodyMediumDefault">
          {item.attributes.user_count || 0}
        </Typography>
      )
    }
  ];

  const cataloguesColumn = {
    field: "attributes.catalogues",
    headerName: "Catalogues",
    renderCell: (item) => (
      <CatalogueBadges
        catalogues={item.attributes.catalogue_names || []}
        dataCatalogues={item.attributes.data_catalogue_names || []}
        toolCatalogues={item.attributes.tool_catalogue_names || []}
        features={features}
      />
    ),
    sx: { width: '35%' }
  };

  const columns = isGatewayOnly ? baseColumns : [...baseColumns, cataloguesColumn];

  const baseActions = [
    {
      label: "Edit team",
      onClick: handleEdit
    },
    {
      label: "Manage team members",
      onClick: handleManageMembers
    }
  ];

  const catalogsAction = {
    label: "Manage catalogs",
    onClick: handleManageCatalogs
  };

  const deleteAction = {
    label: "Delete team",
    onClick: handleDelete
  };

  const actions = isGatewayOnly 
    ? [...baseActions, deleteAction]
    : [...baseActions, catalogsAction, deleteAction];

  return (
    <DataTable
      columns={columns}
      data={groups}
      pagination={{
        page,
        pageSize,
        totalPages,
        onPageChange: handlePageChange,
        onPageSizeChange: handlePageSizeChange,
      }}
      onRowClick={handleGroupClick}
      enableSearch={true}
      onSearch={handleSearch}
      searchPlaceholder="Search by name"
      sortConfig={sortConfig}
      onSortChange={handleSortChange}
      actions={actions}
    />
  );
};

export default GroupsTable;