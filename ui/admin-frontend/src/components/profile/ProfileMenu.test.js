import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import ProfileMenu from "./ProfileMenu";
import { usePermissions } from "../../admin/context/PermissionsContext";
import { getMe } from "../../admin/services/meService";

jest.mock("../../admin/context/PermissionsContext", () => ({
  usePermissions: jest.fn(),
}));
jest.mock("../../admin/services/meService", () => ({
  ...jest.requireActual("../../admin/services/meService"),
  getMe: jest.fn(),
  getMyPreferences: jest.fn(),
  updateMyPreferences: jest.fn(),
  rollMyApiKey: jest.fn(),
  revokeMyApiKey: jest.fn(),
}));
jest.mock("../../admin/utils/pubClient", () => ({
  __esModule: true,
  default: {},
  logout: jest.fn(),
}));

const meDoc = {
  id: "1",
  attributes: {
    name: "Ada Lovelace",
    email: "ada@tyk.io",
    is_admin: true,
    account_type: "Admin",
    auth_source: "local",
    has_api_key: true,
    roles: [{ id: 1, name: "Owner", slug: "owner" }, { id: 2, name: "Auditor", slug: "auditor", via: "group" }],
  },
};

describe("ProfileMenu", () => {
  const onLogout = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    usePermissions.mockReturnValue({
      identity: { name: "Ada Lovelace", email: "ada@tyk.io", roles: [], raw: { attributes: { name: "Ada Lovelace", email: "ada@tyk.io" } } },
    });
    getMe.mockResolvedValue(meDoc);
    require("../../admin/services/meService").getMyPreferences.mockResolvedValue({});
  });

  it("renders an avatar with initials and opens a labelled menu", async () => {
    render(<ProfileMenu onLogout={onLogout} />);
    const button = screen.getByRole("button", { name: "Account menu" });
    expect(button).toHaveTextContent("AL");
    expect(button).toHaveAttribute("aria-haspopup", "menu");

    fireEvent.click(button);
    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(getMe).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.getByText("Administrator · Self-registered")).toBeInTheDocument());
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.getByText("ada@tyk.io")).toBeInTheDocument();
    expect(screen.getByText("Owner")).toBeInTheDocument();
    expect(screen.getByText("Auditor")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "My API key" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Notification preferences" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Log out" })).toBeInTheDocument();
  });

  it("falls back to the boot identity when the refetch fails", async () => {
    getMe.mockRejectedValue(new Error("offline"));
    render(<ProfileMenu onLogout={onLogout} />);
    fireEvent.click(screen.getByRole("button", { name: "Account menu" }));
    expect(await screen.findByText("Ada Lovelace")).toBeInTheDocument();
    expect(screen.getByText("ada@tyk.io")).toBeInTheDocument();
  });

  it("Log out calls the logout handler", async () => {
    render(<ProfileMenu onLogout={onLogout} />);
    fireEvent.click(screen.getByRole("button", { name: "Account menu" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Log out" }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });

  it("My API key opens the API key dialog", async () => {
    render(<ProfileMenu onLogout={onLogout} />);
    fireEvent.click(screen.getByRole("button", { name: "Account menu" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "My API key" }));
    expect(await screen.findByRole("dialog", { name: "My API key" })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("api-key-status")).toHaveTextContent("You have an API key"));
  });

  it("Notification preferences opens the preferences dialog", async () => {
    render(<ProfileMenu onLogout={onLogout} />);
    fireEvent.click(screen.getByRole("button", { name: "Account menu" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Notification preferences" }));
    expect(await screen.findByRole("dialog", { name: "Notification preferences" })).toBeInTheDocument();
  });
});
