import React from "react";
import { render, screen, act, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { NotificationProvider, useNotifications, UNREAD_POLL_INTERVAL_MS } from "./NotificationContext";
import {
  listNotifications,
  getUnreadCount,
  markNotificationRead,
  markAllNotificationsRead,
} from "../services/notificationService";

jest.mock("../services/notificationService", () => ({
  PAGE_SIZE: 20,
  listNotifications: jest.fn(),
  getUnreadCount: jest.fn(),
  markNotificationRead: jest.fn(),
  markAllNotificationsRead: jest.fn(),
}));

const Consumer = () => {
  const { unreadCount, items, total, hasMore, refresh, loadMore, markAsRead, markAllAsRead } = useNotifications();
  return (
    <div>
      <span data-testid="unread">{unreadCount}</span>
      <span data-testid="count">{items.length}</span>
      <span data-testid="total">{total}</span>
      <span data-testid="has-more">{String(hasMore)}</span>
      <ul>
        {items.map((n) => (
          <li key={n.id} data-testid={`item-${n.id}`}>
            {n.title}:{n.read ? "read" : "unread"}
          </li>
        ))}
      </ul>
      <button onClick={() => refresh()}>refresh</button>
      <button onClick={() => loadMore()}>more</button>
      <button onClick={() => markAsRead(1)}>read-1</button>
      <button onClick={() => markAllAsRead()}>read-all</button>
    </div>
  );
};

const renderProvider = () =>
  render(
    <NotificationProvider>
      <Consumer />
    </NotificationProvider>,
  );

describe("NotificationProvider", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    getUnreadCount.mockResolvedValue(3);
    markNotificationRead.mockResolvedValue(true);
    markAllNotificationsRead.mockResolvedValue(true);
    listNotifications.mockResolvedValue({
      items: [
        { id: 1, title: "One", content: "", read: false, type: "app", link: "", sentAt: null },
        { id: 2, title: "Two", content: "", read: true, type: "user", link: "", sentAt: null },
      ],
      total: 5,
      unread: 3,
      limit: 20,
      offset: 0,
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("fetches the unread count on mount and again every minute", async () => {
    jest.useFakeTimers();
    renderProvider();
    expect(getUnreadCount).toHaveBeenCalledTimes(1);
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByTestId("unread")).toHaveTextContent("3");

    getUnreadCount.mockResolvedValue(7);
    await act(async () => {
      jest.advanceTimersByTime(UNREAD_POLL_INTERVAL_MS);
      await Promise.resolve();
    });
    expect(getUnreadCount).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("unread")).toHaveTextContent("7");

    await act(async () => {
      jest.advanceTimersByTime(UNREAD_POLL_INTERVAL_MS - 1);
    });
    expect(getUnreadCount).toHaveBeenCalledTimes(2);
  });

  it("stops polling when unmounted", async () => {
    jest.useFakeTimers();
    const { unmount } = renderProvider();
    await act(async () => {
      await Promise.resolve();
    });
    unmount();
    await act(async () => {
      jest.advanceTimersByTime(UNREAD_POLL_INTERVAL_MS * 3);
    });
    expect(getUnreadCount).toHaveBeenCalledTimes(1);
  });

  it("refresh loads the first page and loadMore appends the next one by offset", async () => {
    renderProvider();
    fireEvent.click(screen.getByText("refresh"));
    await waitFor(() => expect(screen.getByTestId("count")).toHaveTextContent("2"));
    expect(listNotifications).toHaveBeenLastCalledWith({ limit: 20, offset: 0, unread: false });
    expect(screen.getByTestId("total")).toHaveTextContent("5");
    expect(screen.getByTestId("has-more")).toHaveTextContent("true");

    listNotifications.mockResolvedValue({
      items: [
        { id: 2, title: "Two", content: "", read: true, type: "", link: "", sentAt: null }, // duplicate, dropped
        { id: 3, title: "Three", content: "", read: false, type: "", link: "", sentAt: null },
      ],
      total: 3,
      limit: 20,
      offset: 2,
    });
    fireEvent.click(screen.getByText("more"));
    await waitFor(() => expect(screen.getByTestId("count")).toHaveTextContent("3"));
    expect(listNotifications).toHaveBeenLastCalledWith({ limit: 20, offset: 2, unread: false });
    expect(screen.getByTestId("has-more")).toHaveTextContent("false");
  });

  it("markAsRead updates the item and decrements the unread count without refetching", async () => {
    renderProvider();
    fireEvent.click(screen.getByText("refresh"));
    await waitFor(() => expect(screen.getByTestId("item-1")).toHaveTextContent("One:unread"));
    expect(screen.getByTestId("unread")).toHaveTextContent("3");

    fireEvent.click(screen.getByText("read-1"));
    await waitFor(() => expect(screen.getByTestId("item-1")).toHaveTextContent("One:read"));
    expect(markNotificationRead).toHaveBeenCalledWith(1);
    expect(screen.getByTestId("unread")).toHaveTextContent("2");
    expect(getUnreadCount).toHaveBeenCalledTimes(1);

    // Already read: no second request.
    fireEvent.click(screen.getByText("read-1"));
    await act(async () => {
      await Promise.resolve();
    });
    expect(markNotificationRead).toHaveBeenCalledTimes(1);
  });

  it("markAllAsRead clears every item and the badge", async () => {
    renderProvider();
    fireEvent.click(screen.getByText("refresh"));
    await waitFor(() => expect(screen.getByTestId("item-1")).toHaveTextContent("One:unread"));
    fireEvent.click(screen.getByText("read-all"));
    await waitFor(() => expect(screen.getByTestId("unread")).toHaveTextContent("0"));
    expect(markAllNotificationsRead).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("item-1")).toHaveTextContent("One:read");
  });
});
