import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter, Routes, Route, useLocation } from "react-router-dom";
import AdminRoutes from "./AdminRoutes";
import { PermissionsProvider } from "../admin/context/PermissionsContext";
import { clearIdentity } from "../admin/utils/identityStore";

jest.mock("../admin/utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn() },
}));
jest.mock("../admin/utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(() => Promise.resolve({ data: { data: [] }, headers: {} })) },
}));
jest.mock("../admin/components/plugins/DynamicPluginRoute", () => ({
  usePluginRoutes: () => ({ routes: [], isLoading: false }),
}));
jest.mock("../admin/hooks/useSystemFeatures", () => ({
  __esModule: true,
  default: () => ({ features: { feature_groups: false, feature_model_router: true } }),
}));
// The pages the redirects land on are stubbed; everything else in routes.js
// loads for real so the test exercises the real descriptors.
jest.mock("../admin/components/llms/LLMForm", () => ({
  __esModule: true,
  default: () => <div>LLM form page</div>,
}));
jest.mock("../admin/components/tools/ToolForm", () => ({
  __esModule: true,
  default: () => <div>Tool form page</div>,
}));
// FilterForm pulls in a prismjs stylesheet from node_modules that Jest does not transform.
jest.mock("../admin/components/filters/FilterForm", () => ({
  __esModule: true,
  default: () => <div>Filter form page</div>,
}));
jest.mock("../admin/pages/MetadataCompliance", () => ({
  __esModule: true,
  default: () => <div>Metadata coverage page</div>,
}));

const fullAdmin = { id: "1", attributes: { is_admin: true, has_admin_access: true, rbac_enabled: false, permissions: ["*"] } };

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
};

const renderAt = (path) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <PermissionsProvider identity={fullAdmin}>
        <Routes>
          <Route
            path="/admin/*"
            element={
              <>
                <LocationProbe />
                <AdminRoutes uiOptions={{}} />
              </>
            }
          />
        </Routes>
      </PermissionsProvider>
    </MemoryRouter>,
  );

describe("AdminRoutes", () => {
  beforeEach(() => clearIdentity());

  it("redirects the :resource/:id/edit shape to the real edit route", async () => {
    renderAt("/admin/llms/7/edit");
    expect(await screen.findByText("LLM form page")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/admin/llms/edit/7");
  });

  it("redirects tools/:id/edit as well", async () => {
    renderAt("/admin/tools/3/edit");
    expect(await screen.findByText("Tool form page")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/admin/tools/edit/3");
  });

  it("shows a Page not found page with a link back to the overview for unknown paths", async () => {
    renderAt("/admin/nope");
    expect(await screen.findByText("Page not found")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to overview/i })).toHaveAttribute("href", "/admin");
  });

  it("keeps the old metadata/compliance path working as a redirect to metadata/coverage", async () => {
    renderAt("/admin/metadata/compliance");
    expect(await screen.findByText("Metadata coverage page")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent("/admin/metadata/coverage");
  });

  it("serves metadata/coverage directly", async () => {
    renderAt("/admin/metadata/coverage");
    expect(await screen.findByText("Metadata coverage page")).toBeInTheDocument();
  });
});
