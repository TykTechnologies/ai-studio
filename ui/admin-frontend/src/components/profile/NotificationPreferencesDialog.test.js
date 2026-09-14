import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import NotificationPreferencesDialog from "./NotificationPreferencesDialog";
import { getMyPreferences, updateMyPreferences } from "../../admin/services/meService";

jest.mock("../../admin/services/meService", () => ({
  getMyPreferences: jest.fn(),
  updateMyPreferences: jest.fn(),
}));

describe("NotificationPreferencesDialog", () => {
  const onClose = jest.fn();
  const onChanged = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    getMyPreferences.mockResolvedValue({ notifications_enabled: true, email_notifications_enabled: false });
    updateMyPreferences.mockImplementation(async (patch) => patch);
  });

  it("starts from the /common/me values and then applies the preferences endpoint", async () => {
    render(
      <NotificationPreferencesDialog
        open
        onClose={onClose}
        attributes={{ notifications_enabled: true, email_notifications_enabled: true }}
      />,
    );
    const inApp = screen.getByRole("checkbox", { name: "In-app notifications" });
    const email = screen.getByRole("checkbox", { name: "Email notifications" });
    expect(inApp).toBeChecked();
    expect(email).toBeChecked();
    await waitFor(() => expect(email).not.toBeChecked());
    expect(inApp).toBeChecked();
  });

  it("toggling a switch PATCHes just that preference", async () => {
    render(<NotificationPreferencesDialog open onClose={onClose} onChanged={onChanged} attributes={{}} />);
    await waitFor(() => expect(getMyPreferences).toHaveBeenCalled());
    const email = screen.getByRole("checkbox", { name: "Email notifications" });
    await waitFor(() => expect(email).not.toBeChecked());

    fireEvent.click(email);
    await waitFor(() => expect(updateMyPreferences).toHaveBeenCalledWith({ email_notifications_enabled: true }));
    expect(email).toBeChecked();
    expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ email_notifications_enabled: true }));

    fireEvent.click(screen.getByRole("checkbox", { name: "In-app notifications" }));
    await waitFor(() => expect(updateMyPreferences).toHaveBeenLastCalledWith({ notifications_enabled: false }));
  });

  it("reverts the switch and shows the error when the PATCH fails", async () => {
    updateMyPreferences.mockRejectedValue(new Error("nope"));
    render(<NotificationPreferencesDialog open onClose={onClose} attributes={{}} />);
    const email = screen.getByRole("checkbox", { name: "Email notifications" });
    await waitFor(() => expect(email).not.toBeChecked());
    fireEvent.click(email);
    expect(await screen.findByText("nope")).toBeInTheDocument();
    expect(email).not.toBeChecked();
  });

  it("keeps the /common/me values when the preferences endpoint is unavailable", async () => {
    getMyPreferences.mockRejectedValue(new Error("404"));
    render(
      <NotificationPreferencesDialog
        open
        onClose={onClose}
        attributes={{ notifications_enabled: false, email_notifications_enabled: true }}
      />,
    );
    await waitFor(() => expect(getMyPreferences).toHaveBeenCalled());
    expect(screen.getByRole("checkbox", { name: "In-app notifications" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Email notifications" })).toBeChecked();
  });

  it("Done closes the dialog", async () => {
    render(<NotificationPreferencesDialog open onClose={onClose} attributes={{}} />);
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Email notifications" })).not.toBeChecked());
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(onClose).toHaveBeenCalled();
  });
});
