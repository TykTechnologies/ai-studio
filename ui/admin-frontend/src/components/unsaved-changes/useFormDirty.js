import { useCallback, useEffect, useRef, useState } from "react";
import { deepClone, deepEqual } from "./deepEqual";
import { useUnsavedChangesGuard } from "./UnsavedChangesProvider";

/**
 * Tracks whether a form's editable values differ from a baseline snapshot.
 *
 *   const { isDirty, markSaved, reset } = useFormDirty(values, { ready });
 *
 * - `values`: a plain object built from everything the user can edit.
 * - `ready`: `true` once the form's data is on screen (`true` for create
 *   forms, "loaded" for edit forms). The baseline is taken the first time it
 *   is true, so the fetch that fills an edit form never counts as a change.
 * - `markSaved()`: re-baselines on the current values. Call it right before
 *   the post-save redirect so the guard does not fire on the way out.
 * - `reset()`: drops the baseline; it is retaken on the next ready render.
 */
export const useFormDirty = (values, { ready = true } = {}) => {
  const latestRef = useRef(values);
  latestRef.current = values;

  const baselineRef = useRef(undefined);
  // Bumped whenever the baseline changes so the component re-renders and the
  // memoised `isDirty` below is recomputed.
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (ready && baselineRef.current === undefined) {
      baselineRef.current = deepClone(latestRef.current);
      setVersion((v) => v + 1);
    }
  }, [ready, version]);

  const markSaved = useCallback(() => {
    baselineRef.current = deepClone(latestRef.current);
    setVersion((v) => v + 1);
  }, []);

  const reset = useCallback(() => {
    baselineRef.current = undefined;
    setVersion((v) => v + 1);
  }, []);

  const isDirty = baselineRef.current !== undefined && !deepEqual(baselineRef.current, values);

  return { isDirty, markSaved, reset };
};

/**
 * `useFormDirty` plus registration with the navigation guard. Most forms want
 * exactly this.
 */
export const useUnsavedForm = (values, options) => {
  const state = useFormDirty(values, options);
  useUnsavedChangesGuard(state.isDirty);
  return state;
};

export default useFormDirty;
