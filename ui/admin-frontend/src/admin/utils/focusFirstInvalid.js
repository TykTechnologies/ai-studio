/**
 * After a failed submit, bring the first invalid control into view and focus
 * it (#502 E8). Long forms keep their submit button below the fold, so a
 * validation failure used to look like a button that did nothing.
 *
 * It works from the rendered errors rather than a per-form field list: MUI
 * marks an errored input with aria-invalid="true" and its FormControl with
 * .Mui-error, so the first such element in document order is the first thing
 * the user has to fix. Call it right after setting the errors; it waits for
 * React to render them.
 */

const FOCUSABLE =
  'input:not([type="hidden"]):not([aria-hidden="true"]):not([tabindex="-1"]), textarea, select, button, [role="combobox"], [tabindex="0"]';

// The element to focus for an invalid marker: MUI Select puts aria-invalid on
// a hidden native input, so use its visible combobox; a FormControl or a
// custom picker marked .Mui-error gets its first focusable descendant.
export const focusTargetFor = (el) => {
  if (!el) return null;
  if (el.classList.contains('MuiSelect-nativeInput')) {
    return el.parentElement?.querySelector('[role="combobox"]') || null;
  }
  if (el.matches(FOCUSABLE)) return el;
  const scope = el.closest('.MuiFormControl-root') || el;
  return scope.querySelector(FOCUSABLE);
};

export const findFirstInvalid = (root = document) =>
  root.querySelector('[aria-invalid="true"], .Mui-error') || null;

export const focusFirstInvalidNow = (root = document) => {
  const marker = findFirstInvalid(root);
  if (!marker) return false;
  const target = focusTargetFor(marker) || marker;
  const visible = target.closest('.MuiFormControl-root') || target;
  if (typeof visible.scrollIntoView === 'function') {
    visible.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
  if (typeof target.focus === 'function') {
    target.focus({ preventScroll: true });
  }
  return true;
};

// The form an event target belongs to (a submit handler hands over the form,
// a button's click handler the button), else the whole document.
const scopeOf = (el) => (el && typeof el.closest === 'function' && el.closest('form')) || document;

/**
 * Schedules focusFirstInvalidNow for after the errors have rendered. Pass the
 * submit event's currentTarget (read synchronously, before any await) so only
 * that form is searched.
 */
const focusFirstInvalid = (from) => {
  const scope = scopeOf(from);
  return new Promise((resolve) => {
    setTimeout(() => resolve(focusFirstInvalidNow(scope)), 0);
  });
};

export default focusFirstInvalid;
