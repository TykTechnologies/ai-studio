/**
 * Helpers for the "pick an item, press +, then save" pattern used by the
 * catalog forms.
 *
 * The forms hold the picked item in local state and only the + button moved it
 * into the array that gets POSTed. Selecting an option and pressing *Create
 * catalog* therefore produced a catalog with zero members -- no error, no
 * warning, and the select still displaying the choice as the user pressed save.
 * The symptom surfaced much later, as a developer with an empty portal.
 *
 * Rather than block the save or add a warning the user has to read, treat a
 * pending selection as what it plainly is: something the user asked for.
 * commitPendingSelection folds it into the list at save time, so pressing + is
 * an accelerator rather than the actual commit.
 */

/**
 * Returns `items` with the pending selection appended, or `items` unchanged
 * when nothing is pending, the selection is already present, or it cannot be
 * resolved against the available options.
 *
 * @param {Array<{id: *}>} items          the already-added items
 * @param {*} selectedId                  the id held in the picker, or "" when idle
 * @param {Array<{id: *}>} availableItems the options the picker was populated from
 * @returns {Array<{id: *}>}              a new array when something was added, else `items`
 */
export const commitPendingSelection = (items, selectedId, availableItems) => {
  if (!selectedId) return items;
  if (items.some((item) => item.id === selectedId)) return items;

  const pending = (availableItems || []).find((item) => item.id === selectedId);
  if (!pending) return items;

  return [...items, pending];
};

/**
 * True when the picker holds a selection that has not been added to the list.
 * Used to label the pending row in the UI, so the state is visible rather than
 * merely handled.
 */
export const hasPendingSelection = (items, selectedId) =>
  Boolean(selectedId) && !items.some((item) => item.id === selectedId);
