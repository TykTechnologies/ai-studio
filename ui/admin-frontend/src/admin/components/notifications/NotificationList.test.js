import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import axios from "axios";
import NotificationList from "./NotificationList";
import { useNotifications } from "../../context/NotificationContext";

jest.mock("axios");
jest.mock("../../context/NotificationContext", () => ({
  useNotifications: jest.fn(),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const longContent = `App "Billing Copilot" requested access to OpenAI Prod. ${"Review it in the admin console. ".repeat(10)}`;

describe("NotificationList", () => {
  const markAsRead = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    markAsRead.mockResolvedValue(true);
    useNotifications.mockReturnValue({ markAsRead, markAllAsRead: jest.fn() });
    axios.get.mockResolvedValue({
      data: [
        { ID: 1, Title: "New app request", Content: longContent, Read: false, Link: "/admin/apps/7" },
        { ID: 2, Title: "Budget warning", Content: "80% of the monthly budget is used.", Read: true, Link: "" },
      ],
    });
  });

  it("renders content as a plain-text preview", async () => {
    render(<NotificationList />);
    expect(await screen.findByText("New app request")).toBeInTheDocument();
    const preview = screen.getByText((text) => text.startsWith('App "Billing Copilot" requested access'));
    expect(preview.textContent.length).toBeLessThan(longContent.length);
    expect(preview.textContent.endsWith("…")).toBe(true);
    expect(screen.getByText("80% of the monthly budget is used.")).toBeInTheDocument();
  });

  it("marks the item read and navigates to its link when clicked", async () => {
    render(<NotificationList />);
    fireEvent.click(await screen.findByText("New app request"));
    await waitFor(() => expect(markAsRead).toHaveBeenCalledWith(1));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/7");
  });

  it("does not navigate for notifications without a link", async () => {
    render(<NotificationList />);
    fireEvent.click(await screen.findByText("Budget warning"));
    await waitFor(() => expect(mockNavigate).not.toHaveBeenCalled());
  });
});
