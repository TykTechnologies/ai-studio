export const CATALOG_ROWS = [
    {
      label: "LLM providers",
      itemsKey: "catalogues",
    },
    {
      label: "Data sources",
      itemsKey: "dataCatalogues",
    },
    {
      label: "Tools",
      itemsKey: "toolCatalogues",
    },
  ];

export const borderStyle = {
  borderBottom: "2px solid",
  borderColor: "border.neutralDefaultSubdued",
  padding: "16px 0",
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
};

export const lastRowStyle = {
  padding: "16px 0",
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
};

export const TEAM_MEMBERS_COLUMNS = [
  { field: "name", headerName: "Name", width: "30%" },
  { field: "email", headerName: "Email", width: "40%" },
  { field: "role", headerName: "Role", width: "30%" },
];