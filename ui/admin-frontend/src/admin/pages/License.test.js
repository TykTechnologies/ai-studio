import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../utils/testTheme";
import License from "./License";
import RequireOwner from "../components/rbac/RequireOwner";
import apiClient from "../utils/apiClient";
import { PermissionsProvider } from "../context/PermissionsContext";

jest.mock("../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), delete: jest.fn() },
}));

const identity = (isOwner) => ({
  id: "2",
  attributes: { email: "a@example.com", is_admin: true, permissions: ["*"], is_owner: isOwner },
});

const report = (namespaces) => ({
  data: {
    data: {
      valid: true,
      skus: ["product-x"],
      expires_at: "2027-11-05T00:00:00Z",
      days_left: 365,
      features: [
        { key: "f1", label: "Portal", enabled: true },
        { key: "f2", label: "Roles", enabled: false },
      ],
      limits: [
        { key: "l1", label: "Apps", used: 3, max: 10, unlimited: false },
        { key: "l2", label: "Portal users", used: 7, max: 0, unlimited: true },
        { key: "l3", label: "Namespaces", used: 2, max: 2, unlimited: false },
      ],
      namespaces,
    },
  },
});

const renderLicense = (isOwner) =>
  render(
    <ThemeProvider theme={testTheme}>
      <MemoryRouter>
        <PermissionsProvider identity={identity(isOwner)}>
          <RequireOwner>
            <License />
          </RequireOwner>
        </PermissionsProvider>
      </MemoryRouter>
    </ThemeProvider>
  );

describe("License page", () => {
  beforeEach(() => {
    apiClient.get.mockReset();
    apiClient.delete.mockReset();
  });

  it("is blocked for an administrator who is not an Owner, without calling the API", () => {
    renderLicense(false);
    expect(screen.getByText("Only an Owner can view this page")).toBeInTheDocument();
    expect(apiClient.get).not.toHaveBeenCalledWith("/license");
  });

  it("shows the report to an Owner", async () => {
    apiClient.get.mockResolvedValue(report([]));
    renderLicense(true);

    expect(await screen.findByText("Products: product-x")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/license");
    expect(screen.getByTestId("license-expiry")).toHaveTextContent("(365 days left)");
    expect(screen.getByTestId("license-limit-l1")).toHaveTextContent("Apps3 of 10");
    expect(screen.getByTestId("license-limit-l2")).toHaveTextContent("7 of unlimited");
    expect(screen.getByTestId("license-limit-l3")).toHaveTextContent("At limit");
    expect(screen.getByTestId("license-limit-l1")).not.toHaveTextContent("At limit");
    expect(screen.getByTestId("license-feature-f1")).toHaveTextContent("Portal");
    expect(screen.getByText("No namespace holds a slot.")).toBeInTheDocument();
  });

  it("releases a slot with no connected edges and shows the server's refusal otherwise", async () => {
    apiClient.get.mockResolvedValue(
      report([
        { namespace: "team-a", claimed_at: "2026-11-01T00:00:00Z", connected_edges: 0 },
        { namespace: "team-b", claimed_at: "2026-11-02T00:00:00Z", connected_edges: 2 },
      ])
    );
    renderLicense(true);

    await screen.findByText("team-a");
    const buttons = screen.getAllByRole("button", { name: "Release slot" });
    expect(buttons[1]).toBeDisabled();

    apiClient.delete.mockRejectedValueOnce({
      response: { data: { errors: [{ detail: 'namespace "team-a" still has 1 connected edge gateway(s)' }] } },
    });
    fireEvent.click(buttons[0]);
    expect(await screen.findByText(/still has 1 connected edge gateway/)).toBeInTheDocument();
    expect(apiClient.delete).toHaveBeenCalledWith("/license/namespaces/team-a");

    apiClient.delete.mockResolvedValueOnce({ status: 204 });
    fireEvent.click(screen.getAllByRole("button", { name: "Release slot" })[0]);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledTimes(2));
  });
});
