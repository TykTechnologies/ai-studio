import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import testTheme from "../utils/testTheme";
import TykConnectionForm, { formToInput, emptyForm, connectionToForm } from "./TykConnectionForm";
import { toKeyAccess, keyAccessErrors, toolNamesHelp } from "./MCPKeyAccessEditor";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

jest.mock("../context/PermissionsContext", () => ({
  usePermissions: () => ({
    can: () => true,
    canAny: () => true,
    canAll: () => true,
    isFullAdmin: true,
    hasAdminAccess: true,
    rbacEnabled: false,
  }),
}));

jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => jest.fn(),
}));

const renderForm = (path) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/admin/tyk-connections/new" element={<TykConnectionForm />} />
          <Route path="/admin/tyk-connections/edit/:id" element={<TykConnectionForm />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>,
  );

const gatewayConn = {
  id: 9,
  kind: "gateway",
  name: "OSS gateways",
  dashboard_url: "http://tyk-gateway:8080",
  gateway_base_url: "https://mcp.example.com",
  declared_mode: "full",
  effective_mode: "full",
  status: "active",
  capabilities: { cluster_shared_redis: { state: "ok" }, gateway_nodes: { state: "ok", detail: "2 of 2 node(s) answered" } },
  gateway_discovery: "static",
  gateway_node_urls: ["http://tyk-gateway-2:8080"],
  gateway_shared_storage: false,
  gateway_api_id_prefix: "studio-abc-",
  key_defaults: { alias_prefix: "studio:", expires_in_seconds: 0 },
  lock_version: 2,
  last_sync_at: "2026-10-06T00:00:00Z",
};

const nodes = [
  { id: 1, address: "http://tyk-gateway:8080", source: "url", state: "in_sync", reachable: true, version: "5.15.1", studio_count: 1, expected_count: 1, mcp_count: 3 },
  { id: 2, address: "http://tyk-gateway-2:8080", source: "static", state: "pending", reachable: true, version: "5.15.1", studio_count: 0, expected_count: 1, mcp_count: 2, last_error: "write failed" },
];

describe("gateway connection payloads", () => {
  it("sends the discovery settings and blanks the Dashboard-only fields", () => {
    const input = formToInput(
      {
        ...emptyForm,
        kind: "gateway",
        name: "g",
        dashboard_url: "http://gw:8080",
        dashboard_access_token: "secret",
        template_id: "left over",
        mdcb_url: "http://mdcb",
        mdcb_access_token: "mdcb",
        known_gateway_tags: "eu",
        gateway_discovery: "static",
        gateway_node_urls: "http://gw2:8080\n http://gw3:8080 ,",
        gateway_shared_storage: true,
      },
      false,
    );
    expect(input.kind).toBe("gateway");
    expect(input.dashboard_access_token).toBe("secret");
    expect(input.template_id).toBe("");
    expect(input.mdcb_url).toBe("");
    expect(input).not.toHaveProperty("mdcb_access_token");
    expect(input.known_gateway_tags).toEqual([]);
    expect(input.gateway_base_urls).toEqual({});
    expect(input.gateway_node_urls).toEqual(["http://gw2:8080", "http://gw3:8080"]);
    expect(input.gateway_shared_storage).toBe(true);
  });

  it("drops the node list unless discovery is static", () => {
    const input = formToInput({ ...emptyForm, kind: "gateway", gateway_discovery: "dns", gateway_node_urls: "http://gw2:8080" }, false);
    expect(input.gateway_node_urls).toEqual([]);
  });

  it("sends nothing gateway-specific for a Dashboard", () => {
    const input = formToInput({ ...emptyForm, name: "d" }, false);
    expect(input).not.toHaveProperty("kind");
    expect(input).not.toHaveProperty("gateway_discovery");
  });

  it("round-trips a stored gateway connection", () => {
    const form = connectionToForm(gatewayConn);
    expect(form.kind).toBe("gateway");
    expect(form.gateway_discovery).toBe("static");
    expect(form.gateway_node_urls).toBe("http://tyk-gateway-2:8080");
  });

  it("builds the key access body with empty meaning no limit", () => {
    expect(toKeyAccess({ allowed_tools: ["a"], rate: "", per: "60", quota_max: "", quota_renewal_rate: "3600" })).toEqual({
      allowed_tools: ["a"],
      rate: 0,
      per: 0,
      quota_max: 0,
      quota_renewal_rate: 0,
    });
    expect(toKeyAccess({ allowed_tools: [], rate: "10", per: "60", quota_max: "1000", quota_renewal_rate: "3600" })).toEqual({
      allowed_tools: [],
      rate: 10,
      per: 60,
      quota_max: 1000,
      quota_renewal_rate: 3600,
    });
  });
});

describe("keyAccessErrors", () => {
  const form = { allowed_tools: [], rate: "", per: "", quota_max: "", quota_renewal_rate: "" };
  it("accepts empty limits", () => {
    expect(keyAccessErrors(form)).toEqual({});
  });
  it("needs a period for a rate and a renewal for a quota", () => {
    expect(keyAccessErrors({ ...form, rate: "10" }).per).toBeTruthy();
    expect(keyAccessErrors({ ...form, rate: "10", per: "60" })).toEqual({});
    expect(keyAccessErrors({ ...form, quota_max: "100" }).quota_renewal_rate).toBeTruthy();
    expect(keyAccessErrors({ ...form, quota_max: "100", quota_renewal_rate: "3600" })).toEqual({});
  });
  it("refuses negative limits", () => {
    expect(keyAccessErrors({ ...form, rate: "-1" }).negative).toBeTruthy();
  });
});

describe("toolNamesHelp", () => {
  it("explains the field when nothing is listed", () => {
    expect(toolNamesHelp([], ["get-weather"]).warning).toBe(false);
  });
  it("is quiet for known tools", () => {
    expect(toolNamesHelp(["get-weather"], ["get-weather", "get-forecast"]).warning).toBe(false);
  });
  it("names tools the proxy does not have", () => {
    const help = toolNamesHelp(["get-wether", "get-weather"], ["get-weather"]);
    expect(help.warning).toBe(true);
    expect(help.text).toContain("get-wether");
    expect(help.text).not.toContain("get-weather,");
  });
  it("warns that names cannot be checked when the definition lists no tools", () => {
    const help = toolNamesHelp(["anything"], []);
    expect(help.warning).toBe(true);
    expect(help.text).toContain("tools/list");
  });
});

describe("TykConnectionForm for a gateway connection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections/9") return Promise.resolve({ data: gatewayConn });
      if (path === "/tyk-connections/9/nodes") return Promise.resolve({ data: nodes });
      return Promise.reject(new Error("unexpected " + path));
    });
  });

  it("shows the nodes and the discovery settings, and hides the Dashboard-only sections", async () => {
    renderForm("/admin/tyk-connections/edit/9");
    expect(await screen.findByTestId("kind-chip")).toHaveTextContent("Tyk Gateway (open source)");
    const table = await screen.findByTestId("gateway-nodes");
    expect(within(table).getByText("http://tyk-gateway-2:8080")).toBeInTheDocument();
    expect(within(table).getByText("Pending")).toBeInTheDocument();
    expect(within(table).getByText("0/1")).toBeInTheDocument();
    expect(screen.getByTestId("gateway-discovery-section")).toBeInTheDocument();
    expect(screen.getByLabelText(/Gateway API URL/)).toHaveValue("http://tyk-gateway:8080");
    expect(screen.getByLabelText(/Gateway secret/)).toBeInTheDocument();
    expect(screen.queryByTestId("template-id-input")).not.toBeInTheDocument();
    expect(screen.queryByText("Gateway segmentation")).not.toBeInTheDocument();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/tyk-connections/9/nodes"));
  });
});
