import pubClient from "../utils/pubClient";
import { handleApiError } from "./utils/errorHandler";

// The signed-in user's own account: identity, notification preferences and
// personal API key. All of it lives under /common/me, so the calls go through
// pubClient (origin base URL, CSRF token on writes).

// account_type is the user's role name from the API ("Super Admin", "Admin",
// "Developer", "Chat user"); keys here are its snake_case form.
const ACCOUNT_TYPE_LABELS = {
  super_admin: "Super administrator",
  admin: "Administrator",
  developer: "Developer",
  chat_user: "Chat user",
  user: "User",
};

// Same wording as the admin Users page (userService.AUTH_SOURCE_LABELS).
const AUTH_SOURCE_LABELS = {
  local: "Self-registered",
  admin: "Admin-created",
  sso: "SSO",
};

const titleCase = (value) =>
  String(value)
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());

/** Human label for the account_type attribute; falls back to the flags. */
export const accountTypeLabel = (attributes = {}) => {
  const explicit = attributes.account_type;
  if (explicit) {
    const key = String(explicit).trim().toLowerCase().replace(/[\s-]+/g, "_");
    return ACCOUNT_TYPE_LABELS[key] || titleCase(explicit);
  }
  if (attributes.is_super_admin) return ACCOUNT_TYPE_LABELS.super_admin;
  if (attributes.is_admin) return ACCOUNT_TYPE_LABELS.admin;
  if (attributes.has_admin_access) return "Administrator (limited)";
  return ACCOUNT_TYPE_LABELS.developer;
};

/** Human label for the auth_source attribute ("" when unknown). */
export const authSourceLabel = (source) => {
  if (!source) return "";
  return AUTH_SOURCE_LABELS[source] || titleCase(source);
};

/** Initials for the avatar: first letters of the first two words of the name, else the email's first letter. */
export const initialsFor = (name, email) => {
  const words = String(name || "")
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  if (words.length >= 2) return `${words[0][0]}${words[1][0]}`.toUpperCase();
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase();
  if (email) return String(email)[0].toUpperCase();
  return "?";
};

/** GET /common/me → the JSON:API-ish { id, type, attributes } document. */
export const getMe = async () => {
  try {
    const response = await pubClient.get("/common/me");
    return response.data;
  } catch (error) {
    throw handleApiError(error);
  }
};

/** GET /common/me/preferences → { notifications_enabled, email_notifications_enabled }. */
export const getMyPreferences = async () => {
  try {
    const response = await pubClient.get("/common/me/preferences");
    const body = response.data;
    return body?.data?.attributes || body?.data || body || {};
  } catch (error) {
    throw handleApiError(error);
  }
};

/** PATCH /common/me/preferences with a partial { notifications_enabled?, email_notifications_enabled? }. */
export const updateMyPreferences = async (preferences) => {
  try {
    const response = await pubClient.patch("/common/me/preferences", preferences);
    const body = response.data;
    return body?.data?.attributes || body?.data || body || preferences;
  } catch (error) {
    throw handleApiError(error);
  }
};

/**
 * POST /common/me/api-key/roll → the new key, returned once. A 403 carries
 * the policy message when SSO users may not hold API keys.
 */
export const rollMyApiKey = async () => {
  try {
    const response = await pubClient.post("/common/me/api-key/roll");
    const body = response.data;
    return body?.data?.api_key || body?.api_key || body?.data?.attributes?.api_key || "";
  } catch (error) {
    throw handleApiError(error);
  }
};

/** DELETE /common/me/api-key → 204. */
export const revokeMyApiKey = async () => {
  try {
    await pubClient.delete("/common/me/api-key");
    return true;
  } catch (error) {
    throw handleApiError(error);
  }
};
