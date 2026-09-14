import pubClient from "../utils/pubClient";
import { handleApiError } from "./utils/errorHandler";

// In-app notifications for the signed-in user. The endpoints live under
// /common (not /api/v1) so they go through pubClient, which carries the CSRF
// token on the PUTs and the session cookie on everything.
const BASE = "/common/api/v1/notifications";

export const PAGE_SIZE = 20;

/**
 * Normalises one notification. The API now returns lowercase keys
 * (id, title, content, read, type, sent_at, link, created_at); older
 * responses used capitalised GORM field names, so both are accepted until
 * the transition is over.
 */
export const normalizeNotification = (raw) => {
  if (!raw || typeof raw !== "object") return null;
  const attributes = raw.attributes || {};
  const pick = (...keys) => {
    for (const key of keys) {
      if (raw[key] !== undefined && raw[key] !== null) return raw[key];
      if (attributes[key] !== undefined && attributes[key] !== null) return attributes[key];
    }
    return undefined;
  };
  const id = pick("id", "ID");
  return {
    id: id === undefined ? undefined : Number(id) || id,
    title: pick("title", "Title") || "",
    content: pick("content", "Content") || "",
    read: Boolean(pick("read", "Read")),
    type: pick("type", "Type") || "",
    link: pick("link", "Link") || "",
    sentAt: pick("sent_at", "SentAt", "created_at", "CreatedAt") || null,
    createdAt: pick("created_at", "CreatedAt") || null,
  };
};

/**
 * Lists notifications, newest first.
 * Returns { items, total, unread, limit, offset }. A bare array (the old
 * response shape) is tolerated: total is then unknown and reported as the
 * number of items received plus one page when the page was full.
 */
export const listNotifications = async ({ limit = PAGE_SIZE, offset = 0, unread = false } = {}) => {
  try {
    const params = { limit, offset };
    if (unread) params.unread = true;
    const response = await pubClient.get(BASE, { params });
    const body = response.data;
    const rawItems = Array.isArray(body) ? body : Array.isArray(body?.data) ? body.data : [];
    const items = rawItems.map(normalizeNotification).filter(Boolean);
    const meta = (!Array.isArray(body) && body?.meta) || {};
    const total =
      typeof meta.total === "number"
        ? meta.total
        : offset + items.length + (items.length >= limit ? limit : 0);
    return {
      items,
      total,
      unread: typeof meta.unread === "number" ? meta.unread : undefined,
      limit: typeof meta.limit === "number" ? meta.limit : limit,
      offset: typeof meta.offset === "number" ? meta.offset : offset,
    };
  } catch (error) {
    throw handleApiError(error);
  }
};

export const getUnreadCount = async () => {
  try {
    const response = await pubClient.get(`${BASE}/unread/count`);
    return Number(response.data?.count) || 0;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const markNotificationRead = async (id) => {
  try {
    await pubClient.put(`${BASE}/${id}/read`);
    return true;
  } catch (error) {
    throw handleApiError(error);
  }
};

export const markAllNotificationsRead = async () => {
  try {
    await pubClient.put(`${BASE}/read-all`);
    return true;
  } catch (error) {
    throw handleApiError(error);
  }
};
