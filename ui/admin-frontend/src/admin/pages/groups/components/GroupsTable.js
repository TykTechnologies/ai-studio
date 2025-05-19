import React from "react";
import DataTable from "../../../components/common/DataTable";
import { Typography } from "@mui/material";
import CatalogueBadges from "./CatalogueBadges";

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
}) => {
  const columns = [
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
    },
    {
      field: "attributes.catalogues",
      headerName: "Catalogues",
      renderCell: (item) => (
        <CatalogueBadges
          catalogues={item.attributes.catalogue_names || []}
          dataCatalogues={item.attributes.data_catalogue_names || []}
          toolCatalogues={item.attributes.tool_catalogue_names || []}
        />
      ),
      sx: { width: '35%' }
    }
  ];

  const actions = [
    {
      label: "Manage catalogues",
    },
    {
      label: "Manage team members",
    },
    {
      label: "Edit team",
      onClick: handleEdit
    },
    {
      label: "Delete team",
      onClick: handleDelete
    }
  ];

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