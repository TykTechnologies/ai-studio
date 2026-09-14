import pubClient from "../utils/pubClient";
import { listNotifications, normalizeNotification, getUnreadCount } from "./notificationService";

jest.mock("../utils/pubClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), put: jest.fn() },
}));

describe("notificationService", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("normalises the new lowercase shape", () => {
    expect(
      normalizeNotification({
        id: 4,
        title: "T",
        content: "C",
        read: false,
        type: "plugin:cache",
        sent_at: "2026-09-14T10:00:00Z",
        link: "/admin/apps/1",
        created_at: "2026-09-14T09:59:00Z",
      }),
    ).toEqual({
      id: 4,
      title: "T",
      content: "C",
      read: false,
      type: "plugin:cache",
      link: "/admin/apps/1",
      sentAt: "2026-09-14T10:00:00Z",
      createdAt: "2026-09-14T09:59:00Z",
    });
  });

  it("still reads the old capitalised GORM shape", () => {
    const n = normalizeNotification({ ID: 9, Title: "Old", Content: "Body", Read: true, Link: "/x", CreatedAt: "2026-01-01T00:00:00Z" });
    expect(n).toMatchObject({ id: 9, title: "Old", content: "Body", read: true, link: "/x", sentAt: "2026-01-01T00:00:00Z" });
  });

  it("lists with limit/offset and reads {data, meta}", async () => {
    pubClient.get.mockResolvedValue({
      data: { data: [{ id: 1, title: "A", read: false }], meta: { total: 12, unread: 4, limit: 8, offset: 0 } },
    });
    const page = await listNotifications({ limit: 8, offset: 0, unread: true });
    expect(pubClient.get).toHaveBeenCalledWith("/common/api/v1/notifications", {
      params: { limit: 8, offset: 0, unread: true },
    });
    expect(page.items).toHaveLength(1);
    expect(page.total).toBe(12);
    expect(page.unread).toBe(4);
  });

  it("tolerates a bare array response", async () => {
    pubClient.get.mockResolvedValue({ data: [{ ID: 1, Title: "A" }, { ID: 2, Title: "B" }] });
    const page = await listNotifications({ limit: 2, offset: 0 });
    expect(page.items.map((n) => n.id)).toEqual([1, 2]);
    // A full page means there may be more.
    expect(page.total).toBe(4);
    expect(page.unread).toBeUndefined();
  });

  it("reads the unread count", async () => {
    pubClient.get.mockResolvedValue({ data: { count: 6 } });
    await expect(getUnreadCount()).resolves.toBe(6);
  });
});
