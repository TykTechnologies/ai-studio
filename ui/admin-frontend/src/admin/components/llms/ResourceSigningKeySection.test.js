import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import apiClient from "../../utils/apiClient";
import ResourceSigningKeySection from "./ResourceSigningKeySection";
import { PermissionsProvider } from "../../context/PermissionsContext";
import { clearIdentity } from "../../utils/identityStore";

jest.mock("../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn() },
}));

const identity = (permissions) => ({
  id: "9",
  attributes: { is_admin: false, has_admin_access: true, rbac_enabled: true, permissions },
});

const renderWith = (permissions) =>
  render(
    <ThemeProvider theme={testTheme}>
      <PermissionsProvider identity={identity(permissions)}>
        <ResourceSigningKeySection llmId="3" />
      </PermissionsProvider>
    </ThemeProvider>,
  );

const status = (data) => ({ data: { data: { llm_id: 3, ...data } } });

const future = "2099-01-01T00:00:00Z";

describe("ResourceSigningKeySection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    clearIdentity();
    apiClient.get.mockResolvedValue(
      status({
        source: "secret",
        kid: "a1b2c3d4",
        rotated_at: "2026-10-13T10:00:00Z",
        previous: { kid: "99887766", source: "secret", expires_at: future, active: true },
      }),
    );
    apiClient.post.mockResolvedValue(status({ source: "secret", kid: "0f0f0f0f", rotated_at: "2026-10-13T11:00:00Z" }));
  });

  it("shows the current and previous key IDs from the status endpoint", async () => {
    renderWith(["llms:read"]);
    expect(await screen.findByText("a1b2c3d4")).toBeInTheDocument();
    expect(apiClient.get).toHaveBeenCalledWith("/llms/3/resource-signing-key");
    expect(screen.getAllByText(/Hub-generated secret/)).toHaveLength(2);
    expect(screen.getByText("99887766")).toBeInTheDocument();
    expect(screen.getByText(/Accepted until/)).toBeInTheDocument();
  });

  it("explains a pre-upgrade previous key and an expired one", async () => {
    apiClient.get.mockResolvedValue(
      status({
        source: "secret",
        kid: "a1b2c3d4",
        previous: { kid: "11223344", source: "vendor_credential", expires_at: "2020-01-01T00:00:00Z", active: false },
      }),
    );
    renderWith(["llms:read"]);
    await screen.findByText("11223344");
    expect(screen.getByText(/Pre-upgrade key/)).toBeInTheDocument();
    expect(screen.queryByText(/Derived from the API key/)).not.toBeInTheDocument();
    expect(screen.getByText(/Expired/)).toBeInTheDocument();
  });

  it("labels a current key that is still derived from the API key", async () => {
    apiClient.get.mockResolvedValue(status({ source: "vendor_credential", kid: "55667788" }));
    renderWith(["llms:read"]);
    await screen.findByText("55667788");
    expect(screen.getByText(/Derived from the API key \(not upgraded yet\)/)).toBeInTheDocument();
  });

  it("read-only users get no Rotate button", async () => {
    renderWith(["llms:read"]);
    await screen.findByText("a1b2c3d4");
    expect(screen.queryByRole("button", { name: /Rotate signing key/ })).not.toBeInTheDocument();
  });

  it("rotates through a confirm dialog with the chosen grace period and shows the new key", async () => {
    const confirmSpy = jest.spyOn(window, "confirm");
    renderWith(["llms:read", "llms:write"]);
    await screen.findByText("a1b2c3d4");

    fireEvent.click(screen.getByRole("button", { name: /Rotate signing key/ }));
    const dialog = await screen.findByTestId("rotate-signing-key-dialog");
    // Rotating again inside the grace window drops the older key early.
    expect(within(dialog).getByText(/99887766/)).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();

    fireEvent.mouseDown(within(dialog).getByLabelText(/Keep the current key valid for/));
    fireEvent.click(await screen.findByRole("option", { name: /Revoke immediately/ }));
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate" }));

    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/llms/3/resource-signing-key/rotate", { grace_period_hours: 0 }),
    );
    expect(await screen.findByText("0f0f0f0f")).toBeInTheDocument();
    expect(confirmSpy).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("defaults the grace period to 30 days", async () => {
    renderWith(["llms:write"]);
    await screen.findByText("a1b2c3d4");
    fireEvent.click(screen.getByRole("button", { name: /Rotate signing key/ }));
    const dialog = await screen.findByTestId("rotate-signing-key-dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate" }));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/llms/3/resource-signing-key/rotate", { grace_period_hours: 720 }),
    );
  });

  it("reports a concurrent rotation and reloads the status", async () => {
    apiClient.post.mockRejectedValue({ response: { status: 409, data: { errors: [{ title: "Conflict" }] } } });
    renderWith(["llms:write"]);
    await screen.findByText("a1b2c3d4");
    fireEvent.click(screen.getByRole("button", { name: /Rotate signing key/ }));
    const dialog = await screen.findByTestId("rotate-signing-key-dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Rotate" }));

    expect(await screen.findByText(/rotated by someone else/)).toBeInTheDocument();
    await waitFor(() => expect(apiClient.get).toHaveBeenCalledTimes(2));
  });

  it("renders nothing when the status cannot be loaded", async () => {
    apiClient.get.mockRejectedValue({ response: { status: 404 } });
    const { container } = renderWith(["llms:read"]);
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });
});
