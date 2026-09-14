import {
  avatarColor,
  buildAppPath,
  detailPath,
  hashString,
  initialsFor,
  kindFacetLabel,
  kindLabel,
  openAICompatibleBaseUrl,
} from "./catalog";

let mockConfig = {};
jest.mock("../../config", () => ({
  getConfig: () => mockConfig,
}));

const item = (type, id, attributes = {}) => ({ type, id: String(id), attributes });

const items = [
  item("llm", 1, { name: "Acme OpenAI", kind: "openai", privacy_score: 40, created_at: "2026-09-01T00:00:00Z", default_model: "gpt-4o", catalogs: [{ id: "1", name: "Default" }] }),
  item("llm", 2, { name: "Bedrock Claude", kind: "bedrock", privacy_score: 80, created_at: "2026-09-10T00:00:00Z", catalogs: [{ id: "2", name: "Platform" }] }),
  item("datasource", 3, { name: "Docs index", kind: "pgvector", privacy_score: 20, created_at: "2026-09-05T00:00:00Z", community_submitted: true, tags: ["docs"], catalogs: [{ id: "1", name: "Default" }] }),
  item("tool", 4, { name: "Weather API", kind: "rest", privacy_score: 10, created_at: "2026-08-01T00:00:00Z", operations: ["getForecast"], catalogs: [] }),
];

describe("catalog utils", () => {
  it("derives stable initials and colours from a name", () => {
    expect(initialsFor("Acme OpenAI")).toBe("AO");
    expect(initialsFor("gpt4")).toBe("GP");
    expect(initialsFor("weather-api")).toBe("WA");
    expect(initialsFor("")).toBe("?");
    expect(hashString("llm:1")).toBe(hashString("llm:1"));
    expect(avatarColor("llm:1")).toEqual(avatarColor("llm:1"));
    expect(avatarColor("llm:1").solid).toMatch(/^hsl\(/);
  });

  it("names kinds per type", () => {
    expect(kindLabel(items[0])).toBe("OpenAI");
    expect(kindLabel(items[2])).toBe("pgvector");
    expect(kindLabel(items[3])).toBe("REST");
    expect(kindLabel(item("plugin_resource", "x", { kind: "7:agent", kind_label: "Agent" }))).toBe("Agent");
  });

  it("routes to the detail page and the app builder per type", () => {
    expect(detailPath(items[0])).toBe("/portal/catalog/llms/1");
    expect(detailPath(items[2])).toBe("/portal/catalog/datasources/3");
    expect(detailPath(items[3])).toBe("/portal/catalog/tools/4");
    const resource = item("plugin_resource", "asset:9", { resource_type: { plugin_id: 7, slug: "agent", name: "Agent" } });
    expect(detailPath(resource)).toBe("/portal/catalog/resources/7/agent/asset%3A9");
    expect(buildAppPath(items[0])).toBe("/portal/app/new?llm=1");
    expect(buildAppPath(items[2])).toBe("/portal/app/new?datasource=3");
    expect(buildAppPath(resource)).toBe("/portal/app/new?plugin_resource=7%3Aagent%3Aasset%3A9");
  });

  it("labels kind facets from the server label or the UI's vendor map", () => {
    expect(kindFacetLabel({ type: "llm", kind: "bedrock", label: "AWS Bedrock" })).toBe("AWS Bedrock");
    expect(kindFacetLabel({ type: "llm", kind: "openai" })).toBe("OpenAI");
    expect(kindFacetLabel({ type: "datasource", kind: "pgvector" })).toBe("pgvector");
    expect(kindFacetLabel({ type: "tool", kind: "rest" })).toBe("REST");
    expect(kindFacetLabel({ type: "plugin_resource", kind: "7:agent", label: "Agent" })).toBe("Agent");
  });

  it("builds the OpenAI-compatible base URL from the proxy URL and the slug", () => {
    mockConfig = { proxyURL: "http://gw.example.com/" };
    expect(openAICompatibleBaseUrl("Acme OpenAI")).toBe("http://gw.example.com/ai/acme-openai/v1");
    mockConfig = {};
    expect(openAICompatibleBaseUrl("Acme OpenAI")).toMatch(/\/\/localhost:9090\/ai\/acme-openai\/v1$/);
  });
});

describe("formatPerMillion", () => {
  const { formatPerMillion } = require("./catalog");
  it("prints dollar prices plainly and keeps sub-dollar precision", () => {
    expect(formatPerMillion(2.5)).toBe("$2.50");
    expect(formatPerMillion(10)).toBe("$10.00");
    expect(formatPerMillion(0.15)).toBe("$0.15");
    expect(formatPerMillion(0.0006)).toBe("$0.0006");
    expect(formatPerMillion(3, "EUR")).toBe("3.00 EUR");
    expect(formatPerMillion(undefined)).toBe("—");
  });
});
