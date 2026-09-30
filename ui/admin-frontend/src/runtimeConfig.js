// Settings the server injects into index.html as window.__TYK_AI_STUDIO__,
// read synchronously so they apply before the first request. Under the
// development server nothing is injected and the defaults apply: served at
// the root, Studio's own sign-in and CSRF tokens.
//
// basePath is the path prefix Studio is served under ("" at the root), e.g.
// when embedded in another application at /ai-studio.

const bootstrap = () =>
  (typeof window !== "undefined" && window.__TYK_AI_STUDIO__) || {};

export const basePath = () => {
  const base = bootstrap().basePath;
  return typeof base === "string" ? base.replace(/\/+$/, "") : "";
};

// withBase puts an absolute path ("/api/v1/llms") under the base path. Full
// URLs, protocol-relative and relative paths, and paths already under the
// base are returned unchanged.
export const withBase = (path) => {
  const base = basePath();
  if (!base || typeof path !== "string" || !path.startsWith("/") || path.startsWith("//")) {
    return path;
  }
  if (path === base || path.startsWith(`${base}/`)) {
    return path;
  }
  return `${base}${path}`;
};

// stripBase turns a browser path (window.location.pathname) into the app's
// own route, without the base path.
export const stripBase = (pathname) => {
  const base = basePath();
  if (base && typeof pathname === "string" && (pathname === base || pathname.startsWith(`${base}/`))) {
    return pathname.slice(base.length) || "/";
  }
  return pathname;
};

// authMode is "host" when the application Studio is embedded in signs users
// in, and "local" when Studio does.
export const authMode = () => (bootstrap().authMode === "host" ? "host" : "local");
export const isHostAuth = () => authMode() === "host";
export const hostLoginURL = () => bootstrap().loginURL || "";
export const hostLogoutURL = () => bootstrap().logoutURL || "";

// The CSRF token for cookie-authenticated writes is fetched from
// csrfTokenURL, which returns it in the csrfTokenHeader response header,
// and sent back in that same request header.
export const csrfTokenHeader = () => bootstrap().csrfTokenHeader || "X-CSRF-Token";
export const csrfTokenURL = () => bootstrap().csrfTokenURL || withBase("/csrf-token");

// chrome is "none" when the host application draws the navigation: the
// console then renders pages only, without its top bar and drawers.
export const isChromeless = () => bootstrap().chrome === "none";

// The height of the console's top bar, as the CSS variable
// --studio-header-height that sticky page headers sit below. Without the
// bar it is 0.
export const HEADER_HEIGHT_VAR = "--studio-header-height";
export const applyChrome = (root = typeof document !== "undefined" ? document.documentElement : null) => {
  if (root && isChromeless()) {
    root.style.setProperty(HEADER_HEIGHT_VAR, "0px");
    root.dataset.studioChrome = "none";
  }
};
