/**
 * The server's message when a tool or LLM name would share its slug with an
 * existing one (models.SlugConflictError: tools answer 409, LLMs 400), or
 * null for any other error. The forms show it under the Name field instead
 * of a generic "failed to save".
 */
export const slugConflictDetail = (error) => {
  const status = error?.response?.status;
  if (status !== 409 && status !== 400) return null;
  const detail = error.response.data?.errors?.[0]?.detail;
  if (typeof detail !== "string") return null;
  return /^(tool|LLM) name ".*" is too close to /.test(detail) ? detail : null;
};

/** Brings the form's Name field into view and focuses it, after the error renders. */
export const focusNameField = () =>
  setTimeout(() => {
    const field = document.querySelector('input[name="name"]');
    if (!field) return;
    if (typeof field.scrollIntoView === "function") field.scrollIntoView({ behavior: "smooth", block: "center" });
    field.focus();
  }, 0);

export default slugConflictDetail;
