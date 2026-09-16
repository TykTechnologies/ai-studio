import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import MCPServerRegister, { buildPayload } from "./MCPServerRegister";
import apiClient from "../utils/apiClient";

jest.mock("../utils/apiClient");

jest.mock("../styles/sharedStyles", () => ({
  TitleBox: ({ children }) => <div>{children}</div>,
  StyledPaper: ({ children, sx, ...props }) => <div {...props}>{children}</div>,
}));

const theme = createTheme();

const connections = [
  { id: 1, name: "Prod", status: "active", effective_mode: "full", capabilities: { rest_to_mcp_supported: { state: "ok" } } },
  { id: 2, name: "Catalogue only", status: "active", effective_mode: "catalogue", capabilities: {} },
];

const renderPage = () =>
  render(
    <ThemeProvider theme={theme}>
      <MemoryRouter initialEntries={["/admin/mcp-servers/register"]}>
        <Routes>
          <Route path="/admin/mcp-servers/register" element={<MCPServerRegister />} />
          <Route path="/admin/mcp-servers/:id" element={<div data-testid="detail-page" />} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

const selectOption = async (testId, label) => {
  const input = screen.getByTestId(testId);
  fireEvent.mouseDown(input.parentElement.querySelector('[role="combobox"]') || input.parentElement);
  fireEvent.click(await screen.findByRole("option", { name: label }));
};

describe("MCPServerRegister", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((path) => {
      if (path === "/tyk-mcp/status") return Promise.resolve({ data: { available: true, enabled: true } });
      if (path === "/tyk-connections") return Promise.resolve({ data: connections });
      if (path === "/tyk-connections/1/gateway-tags") return Promise.resolve({ data: [{ tag: "edge-eu", label: "EU edge", verified: false, sources: ["known"], data_planes: [] }] });
      if (path === "/tyk-connections/1/apis") return Promise.resolve({ data: [{ api_id: "api-orders", name: "Orders API", listen_path: "/orders/", active: true }] });
      if (path === "/tyk-connections/1/apis/api-orders/operations") return Promise.resolve({ data: [{ operation_id: "getOrder", method: "GET", path: "/orders/{id}", summary: "Get order" }] });
      return Promise.reject(new Error("unexpected " + path));
    });
  });

  it("walks a remote registration through dry run to create", async () => {
    apiClient.post.mockImplementation((path) => {
      if (path === "/mcp-servers/register?dry_run=1") {
        return Promise.resolve({ data: { definition: { openapi: "3.0.3" }, warnings: ["Tag edge-eu is not reported by any data plane right now."], endpoint_url: "https://gw.example.com/weather-mcp/mcp" } });
      }
      return Promise.resolve({ data: { server: { id: 42 }, warnings: [] } });
    });
    renderPage();

    // Only full-mode connections are offered.
    await screen.findByTestId("connection");
    await selectOption("connection", "Prod");
    expect(screen.queryByRole("option", { name: "Catalogue only" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("next"));

    fireEvent.change(await screen.findByTestId("name"), { target: { value: "Weather MCP" } });
    fireEvent.change(screen.getByTestId("listen-path"), { target: { value: "/weather-mcp/" } });
    fireEvent.click(screen.getByTestId("next"));
    expect(await screen.findByTestId("page-error")).toHaveTextContent("upstream MCP URL");
    fireEvent.change(screen.getByTestId("upstream-url"), { target: { value: "https://weather.example.com/mcp" } });
    fireEvent.change(screen.getByTestId("upstream-token"), { target: { value: "s3cr3t" } });
    fireEvent.click(screen.getByTestId("next"));

    // Deployment target control renders because the connection knows tags.
    await screen.findByTestId("gateway-tags");
    fireEvent.click(screen.getByTestId("next"));
    expect(await screen.findByTestId("page-error")).toHaveTextContent("deployment target");
    fireEvent.click(screen.getByTestId("confirm-no-tags"));
    fireEvent.change(screen.getByTestId("privacy-score"), { target: { value: "20" } });
    fireEvent.click(screen.getByTestId("publish"));
    fireEvent.click(screen.getByTestId("next"));

    await screen.findByTestId("preview");
    expect(apiClient.post).toHaveBeenCalledWith(
      "/mcp-servers/register?dry_run=1",
      expect.objectContaining({ connection_id: 1, kind: "remote", name: "Weather MCP", listen_path: "/weather-mcp/", upstream_url: "https://weather.example.com/mcp", upstream_auth_token: "s3cr3t", upstream_auth_header_name: "Authorization", confirm_no_gateway_tags: true, privacy_score: 20, publish: true })
    );
    expect(screen.getByText(/not reported by any data plane/)).toBeInTheDocument();
    expect(screen.getByTestId("preview")).toHaveTextContent("https://gw.example.com/weather-mcp/mcp");

    fireEvent.click(screen.getByTestId("create"));
    await waitFor(() => expect(apiClient.post).toHaveBeenCalledWith("/mcp-servers/register", expect.objectContaining({ name: "Weather MCP" })));
    expect(await screen.findByTestId("detail-page")).toBeInTheDocument();
  });

  it("builds a REST-to-MCP payload from picked operations", async () => {
    renderPage();
    await screen.findByTestId("connection");
    await selectOption("connection", "Prod");
    await selectOption("kind", /REST API to MCP/);
    fireEvent.click(screen.getByTestId("next"));

    fireEvent.change(await screen.findByTestId("name"), { target: { value: "Orders MCP" } });
    await selectOption("source-api", /Orders API/);
    await screen.findByTestId("operations");
    fireEvent.click(screen.getByTestId("op-0"));
    fireEvent.change(screen.getByTestId("op-name-0"), { target: { value: "get_order" } });

    // Surface the wizard state through buildPayload via the dry run.
    apiClient.post.mockResolvedValue({ data: { definition: {}, warnings: [], endpoint_url: "" } });
    fireEvent.click(screen.getByTestId("next"));
    await selectOption("consumer-auth", /OAuth 2.1/);
    fireEvent.change(screen.getByTestId("authorization-servers"), { target: { value: "https://auth.example.com" } });
    fireEvent.click(screen.getByTestId("confirm-no-tags"));
    fireEvent.click(screen.getByTestId("next"));
    await screen.findByTestId("preview");
    const body = apiClient.post.mock.calls[0][1];
    expect(body.kind).toBe("rest_to_mcp");
    expect(body.source_api_id).toBe("api-orders");
    expect(body.primitives).toEqual([{ operation_id: "getOrder", method: "GET", path: "/orders/{id}", name: "get_order", description: "Get order" }]);
    expect(body.authorization_servers).toEqual(["https://auth.example.com"]);
    expect(body.upstream_auth_token).toBeUndefined();
  });

  it("surfaces the Dashboard's rejection on the review step", async () => {
    apiClient.post.mockRejectedValue({ response: { data: { errors: [{ detail: "invalid input: the Tyk Dashboard rejected the request: listen path in use" }] } } });
    renderPage();
    await screen.findByTestId("connection");
    await selectOption("connection", "Prod");
    fireEvent.click(screen.getByTestId("next"));
    fireEvent.change(await screen.findByTestId("name"), { target: { value: "X" } });
    fireEvent.change(screen.getByTestId("upstream-url"), { target: { value: "https://x.example.com" } });
    fireEvent.click(screen.getByTestId("next"));
    await screen.findByTestId("gateway-tags");
    fireEvent.click(screen.getByTestId("confirm-no-tags"));
    fireEvent.click(screen.getByTestId("next"));
    expect(await screen.findByTestId("page-error")).toHaveTextContent("listen path in use");
    expect(screen.queryByTestId("preview")).not.toBeInTheDocument();
  });

  it("buildPayload omits upstream auth when no token is given and parses lists", () => {
    const body = buildPayload({
      connection_id: "1", kind: "remote", name: "A", listen_path: "", consumer_auth: "auth_token", gateway_tags: ["edge-eu"], confirm_no_gateway_tags: false,
      description: "", long_description: "", tags: "a, b", privacy_score: "", publish: false, upstream_url: "https://u", upstream_auth_header_name: "X", upstream_auth_token: "", allowed_tools: "t1,t2", primitives: [],
    });
    expect(body.upstream_auth_token).toBeUndefined();
    expect(body.upstream_auth_header_name).toBeUndefined();
    expect(body.allowed_tools).toEqual(["t1", "t2"]);
    expect(body.tags).toEqual(["a", "b"]);
    expect(body.privacy_score).toBeUndefined();
    expect(body.gateway_tags).toEqual(["edge-eu"]);
  });
});
