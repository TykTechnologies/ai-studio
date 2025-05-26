import { renderHook, act, waitFor } from "@testing-library/react";
import { useCatalogsModal } from "./useCatalogsModal";
import { useCatalogsSelection } from "../../../components/groups/hooks/useCatalogsSelection";
import { teamsService } from "../../../services/teamsService";

jest.mock("../../../components/groups/hooks/useCatalogsSelection");
jest.mock("../../../services/teamsService");

describe("useCatalogsModal", () => {
  const groupId = "123";
  
  const mockCatalogs = [
    { value: "1", label: "Catalog 1" },
    { value: "2", label: "Catalog 2" }
  ];
  
  const mockDataCatalogs = [
    { value: "3", label: "Data Catalog 1" },
    { value: "4", label: "Data Catalog 2" }
  ];
  
  const mockToolCatalogs = [
    { value: "5", label: "Tool Catalog 1" },
    { value: "6", label: "Tool Catalog 2" }
  ];
  
  const mockSetSelectedCatalogs = jest.fn();
  const mockSetSelectedDataCatalogs = jest.fn();
  const mockSetSelectedToolCatalogs = jest.fn();
  const mockFetchCatalogs = jest.fn();
  
  const mockGroupResponse = {
    data: {
      id: groupId,
      attributes: {
        name: "Test Group",
        catalogues: [
          { id: "1", attributes: { name: "Catalog 1" } }
        ],
        data_catalogues: [
          { id: "3", attributes: { name: "Data Catalog 1" } }
        ],
        tool_catalogues: [
          { id: "5", attributes: { name: "Tool Catalog 1" } }
        ]
      }
    }
  };

  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(console, "error").mockImplementation(() => {});
    
    useCatalogsSelection.mockReturnValue({
      catalogs: mockCatalogs,
      selectedCatalogs: [],
      setSelectedCatalogs: mockSetSelectedCatalogs,
      dataCatalogs: mockDataCatalogs,
      selectedDataCatalogs: [],
      setSelectedDataCatalogs: mockSetSelectedDataCatalogs,
      toolCatalogs: mockToolCatalogs,
      selectedToolCatalogs: [],
      setSelectedToolCatalogs: mockSetSelectedToolCatalogs,
      loading: false,
      error: null,
      fetchCatalogs: mockFetchCatalogs
    });
    
    teamsService.getTeam.mockResolvedValue(mockGroupResponse);
  });

  afterEach(() => {
    console.error.mockRestore();
  });

  it("initializes with loading state", () => {
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    expect(result.current.loading).toBe(true);
  });

  it("fetches group catalogs when groupId is provided", async () => {
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeam).toHaveBeenCalledWith(groupId);
    
    expect(mockSetSelectedCatalogs).toHaveBeenCalledWith([
      { value: "1", label: "Catalog 1" }
    ]);
    
    expect(mockSetSelectedDataCatalogs).toHaveBeenCalledWith([
      { value: "3", label: "Data Catalog 1" }
    ]);
    
    expect(mockSetSelectedToolCatalogs).toHaveBeenCalledWith([
      { value: "5", label: "Tool Catalog 1" }
    ]);
  });

  it("does not fetch group catalogs when groupId is not provided", async () => {
    const { result } = renderHook(() => useCatalogsModal(null));
    
    expect(result.current.loading).toBe(false);
    expect(teamsService.getTeam).not.toHaveBeenCalled();
    expect(mockSetSelectedCatalogs).not.toHaveBeenCalled();
    expect(mockSetSelectedDataCatalogs).not.toHaveBeenCalled();
    expect(mockSetSelectedToolCatalogs).not.toHaveBeenCalled();
  });

  it("refetches group catalogs when groupId changes", async () => {
    const { result, rerender } = renderHook((props) => useCatalogsModal(props), {
      initialProps: groupId
    });
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeam).toHaveBeenCalledWith(groupId);
    teamsService.getTeam.mockClear();
    
    const newGroupId = "456";
    rerender(newGroupId);
    
    expect(result.current.loading).toBe(true);
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeam).toHaveBeenCalledWith(newGroupId);
  });

  it("handles empty catalog arrays in group response", async () => {
    teamsService.getTeam.mockResolvedValueOnce({
      data: {
        id: groupId,
        attributes: {
          name: "Test Group"
        }
      }
    });
    
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(mockSetSelectedCatalogs).not.toHaveBeenCalled();
    expect(mockSetSelectedDataCatalogs).not.toHaveBeenCalled();
    expect(mockSetSelectedToolCatalogs).not.toHaveBeenCalled();
  });

  it("handles API errors when fetching group catalogs", async () => {
    const mockError = new Error("API error");
    teamsService.getTeam.mockRejectedValueOnce(mockError);
    
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(console.error).toHaveBeenCalledWith(
      expect.stringContaining("Error fetching group catalogs:"),
      mockError
    );
  });

  it("exposes fetchGroupCatalogs function that can be called directly", async () => {
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    teamsService.getTeam.mockClear();
    mockSetSelectedCatalogs.mockClear();
    mockSetSelectedDataCatalogs.mockClear();
    mockSetSelectedToolCatalogs.mockClear();
    
    act(() => {
      result.current.fetchGroupCatalogs();
    });
    
    expect(result.current.loading).toBe(true);
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(teamsService.getTeam).toHaveBeenCalledWith(groupId);
    expect(mockSetSelectedCatalogs).toHaveBeenCalled();
    expect(mockSetSelectedDataCatalogs).toHaveBeenCalled();
    expect(mockSetSelectedToolCatalogs).toHaveBeenCalled();
  });

  it("exposes all catalog selection values and functions", async () => {
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    
    expect(result.current.catalogs).toEqual(mockCatalogs);
    expect(result.current.dataCatalogs).toEqual(mockDataCatalogs);
    expect(result.current.toolCatalogs).toEqual(mockToolCatalogs);
    expect(result.current.fetchCatalogs).toBe(mockFetchCatalogs);
  });

  it("combines loading states from internal state and useCatalogsSelection", async () => {
    useCatalogsSelection.mockReturnValueOnce({
      catalogs: mockCatalogs,
      selectedCatalogs: [],
      setSelectedCatalogs: mockSetSelectedCatalogs,
      dataCatalogs: mockDataCatalogs,
      selectedDataCatalogs: [],
      setSelectedDataCatalogs: mockSetSelectedDataCatalogs,
      toolCatalogs: mockToolCatalogs,
      selectedToolCatalogs: [],
      setSelectedToolCatalogs: mockSetSelectedToolCatalogs,
      loading: true,
      error: null,
      fetchCatalogs: mockFetchCatalogs
    });
    
    const { result } = renderHook(() => useCatalogsModal(groupId));
    
    expect(result.current.loading).toBe(true);
    
    await waitFor(() => {
      expect(teamsService.getTeam).toHaveBeenCalled();
    });
    
    expect(result.current.loading).toBe(true);
  });
});