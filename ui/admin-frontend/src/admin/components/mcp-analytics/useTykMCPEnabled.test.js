import { renderHook, waitFor } from "@testing-library/react";
import apiClient from "../../utils/apiClient";
import useTykMCPEnabled from "./useTykMCPEnabled";

jest.mock("../../utils/apiClient", () => ({ __esModule: true, default: { get: jest.fn() } }));

describe("useTykMCPEnabled", () => {
  beforeEach(() => jest.clearAllMocks());

  it("is on when Tyk MCP is available and enabled", async () => {
    apiClient.get.mockResolvedValue({ data: { available: true, enabled: true } });
    const { result } = renderHook(() => useTykMCPEnabled());
    await waitFor(() => expect(result.current).toBe(true));
    expect(apiClient.get).toHaveBeenCalledWith("/tyk-mcp/status");
  });

  it.each([
    ["CE", () => apiClient.get.mockResolvedValue({ data: { available: false } })],
    ["disabled", () => apiClient.get.mockResolvedValue({ data: { available: true, enabled: false } })],
    ["an error", () => apiClient.get.mockRejectedValue(new Error("403"))],
  ])("is off on %s", async (_, setup) => {
    setup();
    const { result } = renderHook(() => useTykMCPEnabled());
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 0));
    expect(result.current).toBe(false);
  });
});
