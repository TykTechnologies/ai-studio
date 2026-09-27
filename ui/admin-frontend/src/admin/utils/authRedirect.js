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

export const isPublicPath = (pathname) =>
  PUBLIC_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));

// redirectToLogin sends the browser to the login page after a 401, unless the
// current page is already a public one.
export const redirectToLogin = () => {
  if (isPublicPath(window.location.pathname)) {
    return false;
  }
  window.location.href = '/login';
  return true;
};
