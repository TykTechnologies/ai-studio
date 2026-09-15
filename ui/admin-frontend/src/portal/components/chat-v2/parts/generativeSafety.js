/**
 * Guards for values the model puts into generative UI props that end up in
 * attributes or inline styles. Text content is escaped by React and the
 * Markdown component renders without raw HTML, so only URL-bearing and
 * CSS-bearing props need checking.
 */

const SAFE_IMAGE_SCHEMES = ['http:', 'https:'];

/**
 * Returns the image source if it is an http(s) URL, a same-origin path, or
 * an inline image; otherwise undefined so nothing is loaded.
 */
export const safeImageSrc = (src) => {
  if (typeof src !== 'string') return undefined;
  const value = src.trim();
  if (!value) return undefined;
  if (value.startsWith('/') && !value.startsWith('//')) return value;
  if (/^data:image\/(png|jpeg|jpg|gif|webp|svg\+xml);base64,/i.test(value)) return value;
  try {
    const url = new URL(value);
    return SAFE_IMAGE_SCHEMES.includes(url.protocol) ? value : undefined;
  } catch {
    return undefined;
  }
};

/**
 * Accepts colours and gradients for `background`; rejects anything that
 * could fetch or evaluate (url(), image-set(), expression(), var()).
 */
export const safeBackground = (value) => {
  if (typeof value !== 'string') return undefined;
  const v = value.trim();
  if (!v || v.length > 300) return undefined;
  if (/url\s*\(|image-set|expression|javascript:|var\s*\(|@import|\\/i.test(v)) return undefined;
  if (/^[#a-z0-9 ,.%()\-]+$/i.test(v)) return v;
  return undefined;
};

export default { safeImageSrc, safeBackground };
