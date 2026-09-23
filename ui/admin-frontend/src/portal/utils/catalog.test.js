import {
  avatarColor,
  buildAppPath,
  builtInDetailPath,
  detailPath,
  hashString,
  initialsFor,
  isAppGranted,
  kindFacetLabel,
  kindLabel,
  openAICompatibleBaseUrl,
  opensInPlugin,
  secondaryActionLabel,
  secondaryActionPath,
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

  // Building an app only makes sense when an app credential is what grants
  // access; items the providing plugin gates itself link there instead.
  it("tells app-granted items from plugin-gated ones and where to send the latter", () => {
    expect(isAppGranted(items[0])).toBe(true);
    expect(isAppGranted(item("plugin_resource", "x", { resource_type: { plugin_id: 7, slug: "mcp", name: "MCP Servers" } }))).toBe(true);
    const gated = item("plugin_resource", "ast_9", {
      access_granted_via_app: false,
      portal_detail_url: "/portal/plugins/asset-catalog#/assets/ast_9",
      resource_type: { plugin_id: 7, slug: "agent", name: "Agent" },
    });
    expect(isAppGranted(gated)).toBe(false);
    expect(secondaryActionPath(gated)).toBe("/portal/plugins/asset-catalog#/assets/ast_9");
    expect(secondaryActionLabel(gated)).toBe("View in Agent");
    const nowhere = item("plugin_resource", "ast_10", { access_granted_via_app: false });
    expect(secondaryActionPath(nowhere)).toBeNull();
    expect(secondaryActionLabel(nowhere)).toBe("View in plugin");
  });

  // The built-in page has nothing to show for a resource whose access the
  // plugin manages, so the card goes straight to the plugin's page. An
  // app-granted resource keeps the built-in page: "Build app" lives there.
  it("makes the plugin's page the detail view only for plugin-gated items that declare one", () => {
    const rt = { plugin_id: 7, slug: "prompt", name: "Prompt" };
    const url = "/portal/plugins/asset-catalog#/assets/ast_9";
    const gated = item("plugin_resource", "ast_9", { access_granted_via_app: false, portal_detail_url: url, resource_type: rt });
    expect(opensInPlugin(gated)).toBe(true);
    expect(detailPath(gated)).toBe(url);
    expect(builtInDetailPath(gated)).toBe("/portal/catalog/resources/7/prompt/ast_9");

    const granted = item("plugin_resource", "ast_9", { access_granted_via_app: true, portal_detail_url: url, resource_type: rt });
    expect(opensInPlugin(granted)).toBe(false);
    expect(detailPath(granted)).toBe("/portal/catalog/resources/7/prompt/ast_9");
    expect(secondaryActionPath(granted)).toBe(url);

    const nowhere = item("plugin_resource", "ast_9", { access_granted_via_app: false, resource_type: rt });
    expect(opensInPlugin(nowhere)).toBe(false);
    expect(detailPath(nowhere)).toBe("/portal/catalog/resources/7/prompt/ast_9");

    expect(opensInPlugin(items[0])).toBe(false);
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

describe("model router catalog type", () => {
  const {
    CATALOG_TYPES,
    browsePath,
    buildActionLabel,
    itemTypeLabel,
    modelRouterDetailApiPath,
    typeForSlug,
    typeIcon,
    typeLabel,
    unifiedIngressBaseUrl,
  } = require("./catalog");
  const router = item("model_router", 5, { name: "Prod", kind: "openai", kind_label: "Model Router", access_granted_via_app: true });

  it("registers the type with its labels, route slug and icon", () => {
    expect(CATALOG_TYPES.MODEL_ROUTER).toBe("model_router");
    expect(typeLabel("model_router")).toBe("Model router");
    expect(typeLabel("model_router", { plural: true })).toBe("Model routers");
    expect(itemTypeLabel(router)).toBe("Model router");
    expect(typeIcon("model_router")).toBe("route");
    expect(typeForSlug("model-routers")).toBe("model_router");
    expect(browsePath("model_router")).toBe("/portal/catalog/model-routers");
    expect(kindLabel(router)).toBe("Model Router");
  });

  it("routes to its detail page, its detail endpoint and the app builder", () => {
    expect(detailPath(router)).toBe("/portal/catalog/model-routers/5");
    expect(modelRouterDetailApiPath(5)).toBe("/common/catalog/model-routers/5");
    expect(isAppGranted(router)).toBe(true);
    expect(buildAppPath(router)).toBe("/portal/app/new?model_router=5");
    expect(buildActionLabel(router)).toBe("Build app");
  });

  it("builds the unified ingress URL only when the gateway serves it", () => {
    mockConfig = { proxyURL: "http://gw.example.com/", unifiedRouterPath: "/v1/" };
    expect(unifiedIngressBaseUrl()).toBe("http://gw.example.com/v1");
    mockConfig = { proxyURL: "http://gw.example.com" };
    expect(unifiedIngressBaseUrl()).toBeNull();
  });
});

describe("semantic router catalog type", () => {
  const {
    CATALOG_TYPES,
    browsePath,
    buildActionLabel,
    isRouterType,
    itemTypeLabel,
    semanticRouterDetailApiPath,
    typeForSlug,
    typeIcon,
    typeLabel,
  } = require("./catalog");
  const router = item("semantic_router", 7, { name: "Smart", kind: "openai", kind_label: "Semantic Router", access_granted_via_app: true });

  it("registers the type with its labels, route slug and icon", () => {
    expect(CATALOG_TYPES.SEMANTIC_ROUTER).toBe("semantic_router");
    expect(typeLabel("semantic_router")).toBe("Semantic router");
    expect(typeLabel("semantic_router", { plural: true })).toBe("Semantic routers");
    expect(itemTypeLabel(router)).toBe("Semantic router");
    expect(typeIcon("semantic_router")).toBe("psychology");
    expect(typeForSlug("semantic-routers")).toBe("semantic_router");
    expect(browsePath("semantic_router")).toBe("/portal/catalog/semantic-routers");
    expect(kindLabel(router)).toBe("Semantic Router");
    expect(isRouterType("semantic_router")).toBe(true);
    expect(isRouterType("model_router")).toBe(true);
    expect(isRouterType("llm")).toBe(false);
  });

  it("routes to its detail page, its detail endpoint and the app builder", () => {
    expect(detailPath(router)).toBe("/portal/catalog/semantic-routers/7");
    expect(builtInDetailPath(router)).toBe("/portal/catalog/semantic-routers/7");
    expect(semanticRouterDetailApiPath(7)).toBe("/common/catalog/semantic-routers/7");
    expect(buildAppPath(router)).toBe("/portal/app/new?semantic_router=7");
    expect(buildActionLabel(router)).toBe("Build app");
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
