import { calculateGroupCatalogPayload } from './teamsServiceUtils';

describe('teamsServiceUtils', () => {
  describe('calculateGroupCatalogPayload', () => {
    test('should create payload with all catalog types', () => {
      const selectedCatalogs = [
        { value: '1', label: 'Catalog 1' },
        { value: '2', label: 'Catalog 2' }
      ];
      const selectedDataCatalogs = [
        { value: '3', label: 'Data Catalog 1' },
        { value: '4', label: 'Data Catalog 2' }
      ];
      const selectedToolCatalogs = [
        { value: '5', label: 'Tool Catalog 1' },
        { value: '6', label: 'Tool Catalog 2' }
      ];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );

      expect(result).toEqual({
        data: {
          type: "Group",
          attributes: {
            catalogues: [1, 2],
            data_catalogues: [3, 4],
            tool_catalogues: [5, 6]
          }
        }
      });
    });

    test('should handle empty arrays for all catalog types', () => {
      const selectedCatalogs = [];
      const selectedDataCatalogs = [];
      const selectedToolCatalogs = [];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );

      expect(result).toEqual({
        data: {
          type: "Group",
          attributes: {
            catalogues: [],
            data_catalogues: [],
            tool_catalogues: []
          }
        }
      });
    });

    test('should handle a mix of empty and non-empty catalog arrays', () => {
      const selectedCatalogs = [
        { value: '1', label: 'Catalog 1' }
      ];
      const selectedDataCatalogs = [];
      const selectedToolCatalogs = [
        { value: '5', label: 'Tool Catalog 1' }
      ];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );

      expect(result).toEqual({
        data: {
          type: "Group",
          attributes: {
            catalogues: [1],
            data_catalogues: [],
            tool_catalogues: [5]
          }
        }
      });
    });

    test('should parse string values to integers', () => {
      const selectedCatalogs = [
        { value: '1.5', label: 'Catalog 1' },
        { value: '2.7', label: 'Catalog 2' }
      ];
      const selectedDataCatalogs = [
        { value: '3.0', label: 'Data Catalog 1' }
      ];
      const selectedToolCatalogs = [
        { value: '5abc', label: 'Tool Catalog 1' }
      ];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );

      expect(result).toEqual({
        data: {
          type: "Group",
          attributes: {
            catalogues: [1, 2],
            data_catalogues: [3],
            tool_catalogues: [5]
          }
        }
      });
    });

    test('should handle empty arrays for data and tool catalogs', () => {
      const selectedCatalogs = [
        { value: '1', label: 'Catalog 1' }
      ];
      const selectedDataCatalogs = [];
      const selectedToolCatalogs = [];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        selectedDataCatalogs,
        selectedToolCatalogs
      );

      expect(result).toEqual({
        data: {
          type: "Group",
          attributes: {
            catalogues: [1],
            data_catalogues: [],
            tool_catalogues: []
          }
        }
      });
    });

    test('should handle non-parseable values as NaN', () => {
      const selectedCatalogs = [
        { value: 'abc', label: 'Invalid Catalog' }
      ];

      const result = calculateGroupCatalogPayload(
        selectedCatalogs,
        [],
        []
      );

      expect(result.data.attributes.catalogues[0]).toBeNaN();
    });
  });
});