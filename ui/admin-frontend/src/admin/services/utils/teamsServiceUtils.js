export const calculateGroupCatalogPayload = (
  selectedCatalogs,
  selectedDataCatalogs,
  selectedToolCatalogs
) => {
  return {
    data: {
      type: "Group",
      attributes: {
        catalogues: selectedCatalogs.map(cat => parseInt(cat.value, 10)),
        data_catalogues: selectedDataCatalogs.map(cat => parseInt(cat.value, 10)),
        tool_catalogues: selectedToolCatalogs.map(cat => parseInt(cat.value, 10))
      }
    }
  };
}; 