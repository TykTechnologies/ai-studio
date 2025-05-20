import { renderHook, act, waitFor } from "@testing-library/react";
import { useParams } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";
import useGroupDetail from "./useGroupDetail";

jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useParams: jest.fn(),
}));

jest.mock("../../../services/teamsService");

describe("useGroupDetail", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("should initialize with default values", () => {
    useParams.mockReturnValue({ id: null });
    const { result } = renderHook(() => useGroupDetail());

    expect(result.current.group).toBeNull();
    expect(result.current.users).toEqual([]);
    expect(result.current.catalogues).toEqual([]);
    expect(result.current.dataCatalogues).toEqual([]);
    expect(result.current.toolCatalogues).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("should not fetch group details if id is not present", async () => {
    useParams.mockReturnValue({ id: null });
    const { result } = renderHook(() => useGroupDetail());

    // No fetch should be called
    expect(teamsService.getTeam).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);
  });

  it("should fetch group details and update state on successful API call", async () => {
    const mockGroupId = "123";
    const mockGroupData = {
      data: {
        id: mockGroupId,
        attributes: {
          name: "Test Group",
          users: [{ id: "user1", name: "User One" }],
          catalogues: [{ id: "cat1", name: "Catalogue One" }],
          data_catalogues: [{ id: "data_cat1", name: "Data Catalogue One" }],
          tool_catalogues: [{ id: "tool_cat1", name: "Tool Catalogue One" }],
        },
      },
    };
    useParams.mockReturnValue({ id: mockGroupId });
    teamsService.getTeam.mockResolvedValue(mockGroupData);

    const { result } = renderHook(() => useGroupDetail());

    expect(result.current.loading).toBe(true);

    await waitFor(() => expect(result.current.loading).toBe(false));
    
    expect(teamsService.getTeam).toHaveBeenCalledWith(mockGroupId);
    expect(result.current.group).toEqual(mockGroupData.data);
    expect(result.current.users).toEqual(mockGroupData.data.attributes.users);
    expect(result.current.catalogues).toEqual(mockGroupData.data.attributes.catalogues);
    expect(result.current.dataCatalogues).toEqual(mockGroupData.data.attributes.data_catalogues);
    expect(result.current.toolCatalogues).toEqual(mockGroupData.data.attributes.tool_catalogues);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("should set error state on API call failure", async () => {
    const mockGroupId = "123";
    useParams.mockReturnValue({ id: mockGroupId });
    teamsService.getTeam.mockRejectedValue(new Error("API Error"));

    const { result } = renderHook(() => useGroupDetail());

    expect(result.current.loading).toBe(true);

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(teamsService.getTeam).toHaveBeenCalledWith(mockGroupId);
    expect(result.current.group).toBeNull();
    expect(result.current.users).toEqual([]);
    expect(result.current.catalogues).toEqual([]);
    expect(result.current.dataCatalogues).toEqual([]);
    expect(result.current.toolCatalogues).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBe("Failed to load group details");
  });

   it("should handle missing attributes in API response gracefully", async () => {
    const mockGroupId = "456";
    const mockGroupData = {
      data: {
        id: mockGroupId,
        attributes: {
          name: "Test Group Missing Attributes",
          // users, catalogues, data_catalogues, tool_catalogues are intentionally missing
        },
      },
    };
    useParams.mockReturnValue({ id: mockGroupId });
    teamsService.getTeam.mockResolvedValue(mockGroupData);

    const { result } = renderHook(() => useGroupDetail());

    expect(result.current.loading).toBe(true);
    
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(teamsService.getTeam).toHaveBeenCalledWith(mockGroupId);
    expect(result.current.group).toEqual(mockGroupData.data);
    expect(result.current.users).toEqual([]);
    expect(result.current.catalogues).toEqual([]);
    expect(result.current.dataCatalogues).toEqual([]);
    expect(result.current.toolCatalogues).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });
}); 