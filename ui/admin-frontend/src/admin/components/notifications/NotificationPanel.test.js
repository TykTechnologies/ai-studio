import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import NotificationPanel, { PANEL_SIZE, openNotificationLink } from "./NotificationPanel";
import { useNotifications } from "../../context/NotificationContext";

jest.mock("../../context/NotificationContext", () => ({
  useNotifications: jest.fn(),
}));
const mockNavigate = jest.fn();
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
}));

const minutesAgo = (m) => new Date(Date.now() - m * 60000).toISOString();

const items = [
  { id: 1, title: "New app request", content: "App Billing requested access to OpenAI.", read: false, type: "app", link: "/admin/apps/7", sentAt: minutesAgo(5) },
  { id: 2, title: "Plugin installed", content: "Cache plugin is live.", read: true, type: "plugin:cache", link: "https://example.com/plugins", sentAt: minutesAgo(180) },
  { id: 3, title: "New user", content: "dev9 registered.", read: false, type: "user", link: "", sentAt: minutesAgo(1) },
];

describe("NotificationPanel", () => {
  const refresh = jest.fn();
  const markAsRead = jest.fn();
  const markAllAsRead = jest.fn();
  const onClose = jest.fn();
  let context;

  beforeEach(() => {
    jest.clearAllMocks();
    refresh.mockResolvedValue(null);
    markAsRead.mockResolvedValue(true);
    markAllAsRead.mockResolvedValue(true);
    context = { items, total: 14, unreadCount: 2, loading: false, error: null, refresh, markAsRead, markAllAsRead };
    useNotifications.mockImplementation(() => context);
    window.open = jest.fn();
  });

  it("refetches on open and renders the items with icons, previews, times and unread dots", () => {
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    expect(refresh).toHaveBeenCalledWith({ limit: PANEL_SIZE });
    expect(screen.getByText("New app request")).toBeInTheDocument();
    expect(screen.getByText("App Billing requested access to OpenAI.")).toBeInTheDocument();
    expect(screen.getByText("5m ago")).toBeInTheDocument();
    expect(screen.getByText("3h ago")).toBeInTheDocument();
    expect(screen.getByTestId("notification-type-app")).toBeInTheDocument();
    expect(screen.getByTestId("notification-type-plugin:cache")).toBeInTheDocument();
    expect(screen.getByTestId("notification-type-user")).toBeInTheDocument();
    expect(screen.getAllByRole("img", { name: "Unread" })).toHaveLength(2);
    expect(screen.getByText("2 unread")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "View all (14)" })).toBeInTheDocument();
  });

  it("shows at most PANEL_SIZE items", () => {
    context.items = Array.from({ length: PANEL_SIZE + 3 }, (_, i) => ({
      id: i + 1, title: `N${i + 1}`, content: "", read: true, type: "", link: "", sentAt: null,
    }));
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    expect(screen.getAllByRole("button", { name: /^N\d+/ })).toHaveLength(PANEL_SIZE);
  });

  it("clicking an unread item marks it read, navigates in-app and closes", async () => {
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    fireEvent.click(screen.getByText("New app request"));
    await waitFor(() => expect(markAsRead).toHaveBeenCalledWith(1));
    expect(mockNavigate).toHaveBeenCalledWith("/admin/apps/7");
    expect(onClose).toHaveBeenCalled();
  });

  it("opens external links in a new tab and does not re-mark read items", async () => {
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    fireEvent.click(screen.getByText("Plugin installed"));
    await waitFor(() => expect(window.open).toHaveBeenCalledWith("https://example.com/plugins", "_blank", "noopener"));
    expect(markAsRead).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("items without a link only get marked read", async () => {
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    fireEvent.click(screen.getByText("New user"));
    await waitFor(() => expect(markAsRead).toHaveBeenCalledWith(3));
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(window.open).not.toHaveBeenCalled();
  });

  it("footer marks all read and navigates to the full page", () => {
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    fireEvent.click(screen.getByRole("button", { name: "Mark all as read" }));
    expect(markAllAsRead).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "View all (14)" }));
    expect(mockNavigate).toHaveBeenCalledWith("/notifications");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows the caught-up message when empty and disables mark-all", () => {
    context.items = [];
    context.total = 0;
    context.unreadCount = 0;
    render(<NotificationPanel onClose={onClose} titleId="t" />);
    expect(screen.getByText("You're all caught up.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark all as read" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "View all" })).toBeInTheDocument();
  });
});

describe("openNotificationLink", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/");
  });

  it("routes an /admin link through the router, hash included", () => {
    const navigate = jest.fn();
    openNotificationLink("/admin/enterprise/asset-catalog/requests#req_1", navigate);
    expect(navigate).toHaveBeenCalledWith("/admin/enterprise/asset-catalog/requests#req_1");
  });

  it("also fires popstate when the link only changes the hash of the current page", () => {
    // Plugin web components read the hash on popstate; react-router's
    // navigate alone would leave them on the old sub-route.
    window.history.replaceState({}, "", "/admin/enterprise/asset-catalog/requests#req_0");
    const navigate = jest.fn();
    const onPop = jest.fn();
    window.addEventListener("popstate", onPop);
    try {
      openNotificationLink("/admin/enterprise/asset-catalog/requests#req_1", navigate);
      expect(navigate).toHaveBeenCalledWith("/admin/enterprise/asset-catalog/requests#req_1");
      expect(onPop).toHaveBeenCalledTimes(1);
    } finally {
      window.removeEventListener("popstate", onPop);
    }
  });

  it("does not fire popstate for a link to another page", () => {
    window.history.replaceState({}, "", "/admin/apps");
    const navigate = jest.fn();
    const onPop = jest.fn();
    window.addEventListener("popstate", onPop);
    try {
      openNotificationLink("/admin/enterprise/asset-catalog/requests#req_1", navigate);
      expect(navigate).toHaveBeenCalledWith("/admin/enterprise/asset-catalog/requests#req_1");
      expect(onPop).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener("popstate", onPop);
    }
  });
});
