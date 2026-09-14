import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import ApiKeyDialog from "./ApiKeyDialog";
import { rollMyApiKey, revokeMyApiKey } from "../../admin/services/meService";

jest.mock("../../admin/services/meService", () => ({
  rollMyApiKey: jest.fn(),
  revokeMyApiKey: jest.fn(),
}));

describe("ApiKeyDialog", () => {
  const onClose = jest.fn();
  const onChanged = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    rollMyApiKey.mockResolvedValue("tyk-new-key-123");
    revokeMyApiKey.mockResolvedValue(true);
    Object.assign(navigator, { clipboard: { writeText: jest.fn().mockResolvedValue(undefined) } });
  });

  it("shows no-key state with a Create button and no Revoke", () => {
    render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: false }} />);
    expect(screen.getByTestId("api-key-status")).toHaveTextContent("You do not have an API key yet.");
    expect(screen.getByRole("button", { name: "Create key" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });

  it("shows existing-key state with last used", () => {
    render(
      <ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: true, api_key_last_used_at: "2026-09-13T10:00:00Z" }} />,
    );
    expect(screen.getByTestId("api-key-status")).toHaveTextContent(/You have an API key \(last used /);
    expect(screen.getByRole("button", { name: "Roll key" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Revoke" })).toBeInTheDocument();
  });

  it("rolling shows the new key once with a copy button and the store-it warning", async () => {
    render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: true }} onChanged={onChanged} />);
    fireEvent.click(screen.getByRole("button", { name: "Roll key" }));
    expect(await screen.findByTestId("api-key-value")).toHaveTextContent("tyk-new-key-123");
    expect(screen.getByText("Store it now, it will not be shown again.")).toBeInTheDocument();
    expect(onChanged).toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Copy API key" }));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith("tyk-new-key-123"));
  });

  it("the rolled key is gone after the dialog closes and reopens", async () => {
    const { rerender } = render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: true }} />);
    fireEvent.click(screen.getByRole("button", { name: "Roll key" }));
    await screen.findByTestId("api-key-value");
    rerender(<ApiKeyDialog open={false} onClose={onClose} attributes={{ has_api_key: true }} />);
    rerender(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: true }} />);
    expect(screen.queryByTestId("api-key-value")).not.toBeInTheDocument();
    expect(screen.getByTestId("api-key-status")).toBeInTheDocument();
  });

  it("revoke asks for confirmation before calling the API", async () => {
    render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: true }} onChanged={onChanged} />);
    fireEvent.click(screen.getByTestId("api-key-revoke"));
    expect(screen.getByTestId("api-key-revoke-confirm")).toBeInTheDocument();
    expect(revokeMyApiKey).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Keep key" }));
    expect(screen.queryByTestId("api-key-revoke-confirm")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("api-key-revoke"));
    fireEvent.click(screen.getByRole("button", { name: "Revoke", exact: true }));
    await waitFor(() => expect(revokeMyApiKey).toHaveBeenCalledTimes(1));
    expect(await screen.findByTestId("api-key-notice")).toHaveTextContent("revoked");
    expect(onChanged).toHaveBeenCalled();
  });

  it("shows the policy message instead of the buttons when SSO keys are not allowed", () => {
    render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: false, sso_api_keys_allowed: false }} />);
    expect(screen.getByTestId("api-key-policy")).toHaveTextContent(/not available for accounts provisioned by single sign-on/);
    expect(screen.queryByRole("button", { name: /Roll key|Create key/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });

  it("surfaces the server's 403 message on roll", async () => {
    rollMyApiKey.mockRejectedValue(new Error("SSO-provisioned users may not hold API keys"));
    render(<ApiKeyDialog open onClose={onClose} attributes={{ has_api_key: false }} />);
    fireEvent.click(screen.getByRole("button", { name: "Create key" }));
    expect(await screen.findByText("SSO-provisioned users may not hold API keys")).toBeInTheDocument();
    expect(screen.queryByTestId("api-key-value")).not.toBeInTheDocument();
  });
});
