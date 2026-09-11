/**
 * Tiny publish/subscribe channel the API client uses to announce a denied
 * mutation, so one toaster mounted in the layout can tell the user without
 * every page owning its own snackbar.
 */
const listeners = new Set();

export const subscribePermissionDenied = (fn) => {
  listeners.add(fn);
  return () => listeners.delete(fn);
};

export const emitPermissionDenied = (error) => {
  listeners.forEach((fn) => {
    try {
      fn(error);
    } catch (e) {
      console.error('permission denied listener failed', e);
    }
  });
};
