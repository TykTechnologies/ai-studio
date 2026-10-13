/**
 * A multi-select's menu stays open after each pick, by design, so several
 * values can be chosen. MUI then closes it on the next click outside, but
 * that click lands on the menu's invisible backdrop and never reaches what
 * the user clicked: the button, link or field "did nothing" (#502 E6).
 *
 * This lets that click through, for every multi-select at once: when the
 * invisible backdrop of a multi-select menu (its listbox is
 * aria-multiselectable) is clicked, MUI still closes the menu, and the click
 * is then replayed on the element underneath it. Single-select menus close
 * on selection and keep MUI's behaviour.
 */

const MENU_ROOT = '.MuiMenu-root';
// Longest wait for the closing menu to leave the DOM before replaying.
const MAX_WAIT_MS = 600;

const isMultiSelectBackdrop = (target) => {
  if (!(target instanceof Element) || !target.classList.contains('MuiBackdrop-invisible')) return null;
  const root = target.closest(MENU_ROOT);
  if (!root || !root.querySelector('[role="listbox"][aria-multiselectable="true"]')) return null;
  return root;
};

// The element under (x, y) once the menu is out of the way.
const elementBeneath = (root, x, y) => {
  const previous = root.style.pointerEvents;
  root.style.pointerEvents = 'none';
  const el = document.elementFromPoint(x, y);
  root.style.pointerEvents = previous;
  return el && !root.contains(el) ? el : null;
};

// A dismissing click is not replayed onto a destructive control: closing a
// menu must never also delete something (some deletes have no confirm).
const DESTRUCTIVE = '.MuiButton-colorError, .MuiIconButton-colorError, [data-destructive="true"]';
const isDestructive = (el) => Boolean(el.closest(DESTRUCTIVE));

const replay = (el, { clientX, clientY, button }) => {
  const init = { bubbles: true, cancelable: true, view: window, clientX, clientY, button };
  // MUI Select openers react to mousedown, buttons and links to click.
  el.dispatchEvent(new MouseEvent('mousedown', init));
  if (typeof el.focus === 'function') el.focus({ preventScroll: true });
  el.dispatchEvent(new MouseEvent('mouseup', init));
  el.dispatchEvent(new MouseEvent('click', init));
};

export const installMultiSelectClickThrough = (doc = document) => {
  const onClick = (event) => {
    const root = isMultiSelectBackdrop(event.target);
    if (!root) return;
    const beneath = elementBeneath(root, event.clientX, event.clientY);
    if (!beneath || isDestructive(beneath)) return;
    const { clientX, clientY, button } = event;
    // Let MUI close the menu first (it handles the backdrop click). Wait for
    // the menu to leave the DOM, because on its way out it hands focus back
    // to the select; then deliver the click to what the user aimed at.
    const started = Date.now();
    const deliver = () => {
      if (root.isConnected && Date.now() - started < MAX_WAIT_MS) {
        setTimeout(deliver, 16);
        return;
      }
      if (beneath.isConnected) replay(beneath, { clientX, clientY, button });
    };
    setTimeout(deliver, 0);
  };
  doc.addEventListener('click', onClick, true);
  return () => doc.removeEventListener('click', onClick, true);
};

export default installMultiSelectClickThrough;
