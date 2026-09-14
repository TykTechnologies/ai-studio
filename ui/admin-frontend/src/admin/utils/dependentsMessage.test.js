import { buildDependentsMessage, fetchDependents } from "./dependentsMessage";
import apiClient from "./apiClient";

jest.mock("./apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));

const empty = {
  apps: [],
  catalogues: [],
  llms: [],
  tools: [],
  datasources: [],
  agents: [],
  model_routers: [],
  total: 0,
};

describe("buildDependentsMessage", () => {
  it("names the dependents and states the consequence", () => {
    const message = buildDependentsMessage(
      {
        ...empty,
        apps: [
          { id: 1, name: "Billing Copilot" },
          { id: 2, name: "Quickstart App" },
        ],
        catalogues: [{ id: 3, name: "Platform LLMs" }],
        total: 3,
      },
      "LLM",
      "Deleting it removes it from all of them; apps that only have this LLM will stop working.",
    );
    expect(message).toBe(
      "Used by 2 apps (Billing Copilot, Quickstart App) and 1 catalog (Platform LLMs). Deleting it removes it from all of them; apps that only have this LLM will stop working.",
    );
  });

  it("says nothing references the object when the total is zero", () => {
    expect(buildDependentsMessage(empty, "tool")).toBe("Nothing references this tool.");
  });

  it("lists three groups with commas and a final 'and'", () => {
    const message = buildDependentsMessage(
      {
        ...empty,
        apps: [{ id: 1, name: "A" }],
        llms: [{ id: 2, name: "B" }],
        agents: [{ id: 3, name: "C" }],
        total: 3,
      },
      "filter",
    );
    expect(message).toBe(
      "Used by 1 app (A), 1 LLM (B) and 1 agent (C). Deleting it removes it from all of them.",
    );
  });

  it("truncates long name lists", () => {
    const apps = Array.from({ length: 7 }, (_, i) => ({ id: i, name: `App ${i}` }));
    const message = buildDependentsMessage({ ...empty, apps, total: 7 }, "secret");
    expect(message).toContain("7 apps (App 0, App 1, App 2, App 3, App 4, +2 more)");
  });

  it("falls back to a generic sentence when dependents are unknown", () => {
    expect(buildDependentsMessage(null, "data source")).toBe(
      "Deleting this data source removes it from everything that references it.",
    );
  });
});

describe("fetchDependents", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("returns the attributes block from the dependents endpoint", async () => {
    apiClient.get.mockResolvedValue({
      data: { data: { type: "dependents", id: "3", attributes: { ...empty, total: 0 } } },
    });
    const result = await fetchDependents("model-routers", 3);
    expect(apiClient.get).toHaveBeenCalledWith("/model-routers/3/dependents");
    expect(result).toEqual({ ...empty, total: 0 });
  });

  it("returns null when the request fails", async () => {
    jest.spyOn(console, "error").mockImplementation(() => {});
    apiClient.get.mockRejectedValue(new Error("404"));
    expect(await fetchDependents("llms", 1)).toBeNull();
    console.error.mockRestore();
  });
});
