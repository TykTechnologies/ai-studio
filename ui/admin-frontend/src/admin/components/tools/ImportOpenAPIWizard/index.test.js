import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../../utils/testTheme";
import apiClient from "../../../utils/apiClient";
import ImportOpenAPIWizard from "./index";

jest.mock("../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

jest.mock("../../../context/PermissionsContext", () => ({
  usePermissions: () => ({ can: () => true }),
}));

const definition = {
  openapi: "3.0.3",
  info: { title: "Orders (title)", description: "Order lookups" },
  paths: { "/orders/{id}": { get: { operationId: "getOrder" } } },
  components: { securitySchemes: { orderKey: { type: "apiKey", in: "header", name: "X-Key" } } },
  security: [{ orderKey: [] }],
  "x-tyk-api-gateway": { upstream: { url: "https://orders.internal", authentication: { basic: { password: "***" } } } },
};

const byUrl = {
  "/tyk-mcp/status": { available: true, enabled: true },
  "/tools/import/tyk/connections": [
    { id: 1, name: "Prod", dashboard_url: "https://dash.example.com", status: "active", degraded: false, apis_read: "ok" },
  ],
  "/tools/import/tyk/connections/1/apis": [
    { api_id: "api-orders", name: "Orders API", listen_path: "/orders/", active: true },
    { api_id: "api-old", name: "Old API", listen_path: "/old/", active: false },
  ],
  "/tools/import/tyk/connections/1/apis/api-orders": { api_id: "api-orders", name: "Orders API", listen_path: "/orders/", active: true, definition },
};

describe("ImportOpenAPIWizard (Tyk Dashboard path)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    apiClient.get.mockImplementation((url) => {
      if (url in byUrl) return Promise.resolve({ data: byUrl[url] });
      return Promise.reject(new Error(`unexpected GET ${url}`));
    });
    apiClient.post.mockResolvedValue({ data: { data: { id: 42, attributes: { name: "Orders API" } } } });
  });

  it("walks from a saved connection to a created tool", async () => {
    const onImport = jest.fn();
    const onClose = jest.fn();
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <ImportOpenAPIWizard open onClose={onClose} onImport={onImport} />
        </MemoryRouter>
      </ThemeProvider>,
    );

    fireEvent.click(screen.getByDisplayValue("tyk"));
    fireEvent.click(screen.getByTestId("wizard-next"));

    await screen.findByText("Prod");
    expect(apiClient.get).toHaveBeenCalledWith("/tyk-mcp/status");
    fireEvent.click(screen.getByDisplayValue("1"));
    fireEvent.click(screen.getByTestId("wizard-next"));

    await screen.findByTestId("api-api-orders");
    expect(screen.getByText("inactive")).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("api-search"), { target: { value: "orders" } });
    expect(screen.queryByTestId("api-api-old")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("api-api-orders"));
    fireEvent.click(screen.getByTestId("wizard-next"));

    await screen.findByDisplayValue("Orders API");
    expect(screen.getByDisplayValue("Order lookups")).toBeInTheDocument();
    expect(screen.getByDisplayValue("orderKey")).toBeInTheDocument();
    expect(screen.getByText("getOrder")).toBeInTheDocument();
    expect(screen.getByText(/Type: apiKey/)).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("wizard-next"));
    await waitFor(() => expect(onImport).toHaveBeenCalledWith({ id: 42, attributes: { name: "Orders API" } }));
    const [url, body] = apiClient.post.mock.calls[0];
    expect(url).toBe("/tools");
    const attrs = body.data.attributes;
    expect(attrs.name).toBe("Orders API");
    expect(attrs.auth_schema_name).toBe("orderKey");
    expect(attrs.operations).toEqual(["getOrder"]);
    expect(attrs.privacy_score).toBe(25);
    const stored = JSON.parse(Buffer.from(attrs.oas_spec, "base64").toString("utf8"));
    expect(stored.info.description).toBe("Order lookups");
    expect(stored["x-tyk-api-gateway"].upstream.authentication.basic.password).toBe("***");
    expect(onClose).toHaveBeenCalled();
  });

  it("cannot advance past the connection step in the community edition", async () => {
    apiClient.get.mockImplementation((url) =>
      url === "/tyk-mcp/status" ? Promise.resolve({ data: { available: false, enabled: false } }) : Promise.reject(new Error(url)),
    );
    render(
      <ThemeProvider theme={testTheme}>
        <MemoryRouter>
          <ImportOpenAPIWizard open onClose={jest.fn()} onImport={jest.fn()} />
        </MemoryRouter>
      </ThemeProvider>,
    );
    fireEvent.click(screen.getByDisplayValue("tyk"));
    fireEvent.click(screen.getByTestId("wizard-next"));
    await screen.findByText("Enterprise Feature");
    expect(screen.getByTestId("wizard-next")).toBeDisabled();
    expect(apiClient.get).not.toHaveBeenCalledWith("/tools/import/tyk/connections");
  });
});
