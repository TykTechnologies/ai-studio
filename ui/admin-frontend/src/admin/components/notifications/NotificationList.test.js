import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import NotificationList from "./NotificationList";
import { useNotifications } from "../../context/NotificationContext";

jest.mock("../../context/NotificationContext", () => ({
  useNotifications: jest.fn(),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const longContent = `App "Billing Copilot" requested access to OpenAI Prod. ${"Review it in the admin console. ".repeat(10)}`;

const at = (daysAgo, hour = 10) => {
  const d = new Date();
  d.setDate(d.getDate() - daysAgo);
  d.setHours(hour, 0, 0, 0);
  return d.toISOString();
};

const items = [
  { id: 1, title: "New app request", content: longContent, read: false, type: "app", link: "/admin/apps/7", sentAt: at(0) },
  { id: 2, title: "Budget warning", content: "80% of the monthly budget is used.", read: true, type: "budget", link: "", sentAt: at(1) },
  { id: 3, title: "Old plugin note", content: "Installed.", read: true, type: "plugin:x", link: "", sentAt: "2025-03-02T08:00:00Z" },
];

describe("NotificationList", () => {
  const refresh = jest.fn();
  const loadMore = jest.fn();
  const markAsRead = jest.fn();
  const markAllAsRead = jest.fn();
  let context;

  beforeEach(() => {
    jest.clearAllMocks();
    refresh.mockResolvedValue(null);
    loadMore.mockResolvedValue(null);
    markAsRead.mockResolvedValue(true);
    markAllAsRead.mockResolvedValue(true);
    context = {
      items,
      total: 3,
      hasMore: false,
      loading: false,
      error: null,
      unreadCount: 1,
      refresh,
      loadMore,
      markAsRead,
      markAllAsRead,
    };
    useNotifications.mockImplementation(() => context);
  });

  it("loads on mount and groups by day: Today, Yesterday, then dated", () => {
    render(<NotificationList />);
    expect(refresh).toHaveBeenCalledWith({ unread: false });
    const headers = screen.getAllByRole("listitem").filter((li) => li.querySelector("ul"));
    expect(headers).toHaveLength(3);
    expect(within(headers[0]).getByText("Today")).toBeInTheDocument();
    expect(within(headers[0]).getByText("New app request")).toBeInTheDocument();
    expect(within(headers[1]).getByText("Yesterday")).toBeInTheDocument();
    expect(within(headers[1]).getByText("Budget warning")).toBeInTheDocument();
    expect(within(headers[2]).getByText("2 March 2025")).toBeInTheDocument();
    expect(within(headers[2]).getByText("Old plugin note")).toBeInTheDocument();
    expect(screen.getByTestId("notification-type-plugin:x")).toBeInTheDocument();
  });

  it("renders content as a plain-text preview", () => {
    render(<NotificationList />);
    const preview = screen.getByText((text) => text.startsWith('App "Billing Copilot" requested access'));
    expect(preview.textContent.length).toBeLessThan(longContent.length);
    expect(preview.textContent.endsWith("…")).toBe(true);
    expect(screen.getByText("80% of the monthly budget is used.")).toBeInTheDocument();
  });

  it("marks the item read and navigates to its link when clicked", async () => {
    render(<NotificationList />);
    fireEvent.click(screen.getByText("New app request"));
    await waitFor(() => expect(markAsRead).toHaveBeenCalledWith(1));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/7");
  });

  it("does not navigate for notifications without a link", async () => {
    render(<NotificationList />);
    fireEvent.click(screen.getByText("Budget warning"));
    await waitFor(() => expect(mockNavigate).not.toHaveBeenCalled());
    expect(markAsRead).not.toHaveBeenCalled();
  });

  it("the unread toggle refetches with unread=true and refetches after marking read", async () => {
    render(<NotificationList />);
    fireEvent.click(screen.getByRole("checkbox", { name: "Unread only (1)" }));
    await waitFor(() => expect(refresh).toHaveBeenLastCalledWith({ unread: true }));
    fireEvent.click(screen.getByRole("button", { name: "mark as read" }));
    await waitFor(() => expect(markAsRead).toHaveBeenCalledWith(1));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(3));
  });

  it("shows Load more with the remaining count and calls loadMore", () => {
    context.total = 23;
    context.hasMore = true;
    render(<NotificationList />);
    fireEvent.click(screen.getByRole("button", { name: "Load more (20 remaining)" }));
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("hides Load more when everything is loaded", () => {
    render(<NotificationList />);
    expect(screen.queryByRole("button", { name: /Load more/ })).not.toBeInTheDocument();
  });

  it("shows the caught-up empty state", () => {
    context.items = [];
    context.total = 0;
    context.unreadCount = 0;
    render(<NotificationList />);
    expect(screen.getByText("You're all caught up.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark all as read" })).toBeDisabled();
  });
});
