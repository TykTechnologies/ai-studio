import { basePath, hostLoginURL, hostLogoutURL, isHostAuth, stripBase, withBase } from '../../runtimeConfig';

// Pages a signed-out visitor must be able to stay on. A 401 there is the
// expected answer to any background request, not an expired session, so it
// must not send the browser to /login (which would bounce a visitor off the
// sign-up form, or reload the login page itself).
const PUBLIC_PATHS = [
  '/login',
  '/register',
  '/forgot-password',
  '/reset-password',
  '/auth/forgot-password',
  '/auth/reset-password',
];

export const isPublicPath = (pathname) => {
  const route = stripBase(pathname);
  return PUBLIC_PATHS.some((p) => route === p || route.startsWith(`${p}/`));
};

// RETURN_TO_PLACEHOLDER in the host's login URL (studio.Options.LoginURL,
// e.g. "/login?next={return_to}") is replaced with the page the visitor was
// on, so the host can send them back to it after signing in.
export const RETURN_TO_PLACEHOLDER = '{return_to}';

// hostSignInURL is the host application's sign-in page for a visitor at loc.
// The return address is a path on Studio's origin, under the base path, with
// the query and hash, URL-encoded. From a sign-in or password page it is
// the console's home instead, which must not lead back to sign-in. A login
// URL without the placeholder is used as it is.
export const hostSignInURL = (loc = window.location) => {
  const url = hostLoginURL();
  if (!url.includes(RETURN_TO_PLACEHOLDER)) {
    return url;
  }
  const back = isPublicPath(loc.pathname)
    ? `${basePath()}/`
    : `${loc.pathname}${loc.search || ''}${loc.hash || ''}`;
  return url.split(RETURN_TO_PLACEHOLDER).join(encodeURIComponent(back));
};

// RETURN_PARAM is the query parameter on Studio's /login that names the page
// to open after signing in.
export const RETURN_PARAM = 'next';

// safeReturnPath accepts a return address only when it is a console route on
// this origin: a single leading slash (no "//host" or "/\host", which
// browsers treat as another origin), no control characters, and not a
// sign-in or password page. Anything else yields null, so /login?next= can
// never send a user off-site.
export const safeReturnPath = (raw) => {
  if (typeof raw !== 'string' || raw.length > 2048) {
    return null;
  }
  // eslint-disable-next-line no-control-regex
  if (!raw.startsWith('/') || raw.startsWith('//') || /[\u0000-\u001f\u007f\\]/.test(raw)) {
    return null;
  }
  // Compare the path the browser will actually open: "/admin/../login" is
  // /login once dot segments are resolved.
  const pathOnly = new URL(raw, 'http://studio.invalid').pathname;
  if (pathOnly === '/' || isPublicPath(pathOnly)) {
    return null;
  }
  return raw;
};

// returnPathFrom is the route (without the base path) plus query and hash of
// loc, or null when there is nothing worth coming back to.
const returnPathFrom = (loc) =>
  safeReturnPath(`${stripBase(loc.pathname)}${loc.search || ''}${loc.hash || ''}`);

// studioLoginPath is Studio's login route (without the base path) for a
// visitor at loc, carrying that page as ?next= so signing in returns there.
export const studioLoginPath = (loc = window.location) => {
  const back = returnPathFrom(loc);
  return back ? `/login?${RETURN_PARAM}=${encodeURIComponent(back)}` : '/login';
};

// requestedReturnPath is the safe ?next= page in a login URL's query string,
// or null.
export const requestedReturnPath = (search = window.location.search) =>
  safeReturnPath(new URLSearchParams(search).get(RETURN_PARAM));

// loginLocation is where a signed-out user goes: the host application's
// sign-in page when it authenticates users, else Studio's own.
export const loginLocation = () =>
  (isHostAuth() && hostLoginURL() && hostSignInURL()) || withBase(studioLoginPath());

// redirectToLogin sends the browser to the login page after a 401, unless the
// current page is already a public one.
export const redirectToLogin = () => {
  if (isPublicPath(window.location.pathname)) {
    return false;
  }
  window.location.href = loginLocation();
  return true;
};

// redirectToLogout sends the browser on after signing out of Studio: to the
// host application's sign-out when it authenticates users, else to the login
// page.
export const redirectToLogout = () => {
  window.location.href = (isHostAuth() && hostLogoutURL()) || withBase('/login');
};
