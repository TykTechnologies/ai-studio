// Which content of each template version this browser's reviewer last saw,
// so the review can say when a proposed version changed in place since then
// (o4a.11). Only the content hash and the time are kept, never the template
// text. A convenience in this browser only: the approval itself is bound to
// the reviewed bytes by content_hash on the server. Every read and write
// survives storage being unavailable.

export const SEEN_KEY = "promptTemplateReview.seen";
const MAX_ENTRIES = 200;

const readAll = () => {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(SEEN_KEY) || "{}");
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
};

const keyOf = (appId, templateId) => `${appId}:${templateId}`;

/** lastSeen returns { hash, at } or null. */
export const lastSeen = (appId, templateId) => {
  const e = readAll()[keyOf(appId, templateId)];
  if (!e || typeof e.hash !== "string" || typeof e.at !== "string") return null;
  return { hash: e.hash, at: e.at };
};

/** rememberSeen records the version the reviewer is looking at now. */
export const rememberSeen = (appId, template, now = new Date()) => {
  if (!template?.id || !template.content_hash) return;
  try {
    const entry = { hash: String(template.content_hash).slice(0, 128), at: now.toISOString() };
    const all = readAll();
    all[keyOf(appId, template.id)] = entry;
    const keys = Object.keys(all);
    if (keys.length > MAX_ENTRIES) {
      keys
        .sort((a, b) => String(all[a].at).localeCompare(String(all[b].at)))
        .slice(0, keys.length - MAX_ENTRIES)
        .forEach((k) => delete all[k]);
    }
    window.localStorage.setItem(SEEN_KEY, JSON.stringify(all));
  } catch {
    // Storage full or unavailable: the review works without it.
  }
};
