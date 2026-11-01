// A character diff between two versions of a trusted prompt template, for
// the approval view. Each constant is compared code point by code point;
// a slot is one unit, so a renamed or moved slot shows as a change.

import { isConst, utf8Length } from "./promptTemplateText";

// A change smaller than this share of the new version's constant bytes
// gets the small-edit warning.
export const SMALL_EDIT_SHARE = 0.1;

// The diff gives up beyond this many edits (the versions are then shown
// side by side without a diff) to keep the page responsive.
const MAX_EDITS = 2000;

/** units flattens a template's parts into diff units. */
export const units = (parts) => {
  const out = [];
  for (const p of parts || []) {
    if (isConst(p)) {
      for (const ch of p.const) out.push({ t: "c", v: ch });
    } else {
      out.push({ t: "s", v: p.slot || "" });
    }
  }
  return out;
};

const same = (a, b) => a.t === b.t && a.v === b.v;

/**
 * myers returns the edit script from a to b as ops ("eq", "del", "ins"),
 * one per unit, or null when it needs more than MAX_EDITS edits.
 */
const myers = (a, b) => {
  const n = a.length;
  const m = b.length;
  const max = Math.min(n + m, MAX_EDITS);
  const offset = max + 1;
  let v = new Int32Array(2 * max + 3);
  const trace = [];
  for (let d = 0; d <= max; d++) {
    trace.push(v.slice());
    for (let k = -d; k <= d; k += 2) {
      let x;
      if (k === -d || (k !== d && v[offset + k - 1] < v[offset + k + 1])) {
        x = v[offset + k + 1];
      } else {
        x = v[offset + k - 1] + 1;
      }
      let y = x - k;
      while (x < n && y < m && same(a[x], b[y])) {
        x++;
        y++;
      }
      v[offset + k] = x;
      if (x >= n && y >= m) {
        return backtrack(trace, v, a, b, d, offset);
      }
    }
  }
  return null;
};

const backtrack = (trace, last, a, b, dEnd, offset) => {
  const ops = [];
  let x = a.length;
  let y = b.length;
  for (let d = dEnd; d > 0; d--) {
    const v = trace[d];
    const k = x - y;
    let prevK;
    if (k === -d || (k !== d && v[offset + k - 1] < v[offset + k + 1])) {
      prevK = k + 1;
    } else {
      prevK = k - 1;
    }
    const prevX = v[offset + prevK];
    const prevY = prevX - prevK;
    while (x > prevX && y > prevY) {
      ops.push({ op: "eq", unit: b[y - 1] });
      x--;
      y--;
    }
    if (x === prevX) {
      ops.push({ op: "ins", unit: b[y - 1] });
      y--;
    } else {
      ops.push({ op: "del", unit: a[x - 1] });
      x--;
    }
  }
  while (x > 0 && y > 0) {
    ops.push({ op: "eq", unit: b[y - 1] });
    x--;
    y--;
  }
  return ops.reverse();
};

/**
 * diffParts compares the parts of the previous version (before) with the
 * new one (after). It returns {hunks, changedBytes, totalBytes, runs,
 * insertedRuns, identical, tooLarge}:
 *
 * - hunks: consecutive ops of one kind, [{op, units}], for display;
 * - changedBytes: UTF-8 bytes of constant text inserted or deleted;
 * - totalBytes: the new version's constant bytes;
 * - runs: the number of separate changed places;
 * - insertedRuns: of those, how many only insert text;
 * - tooLarge: the diff gave up (more than MAX_EDITS edits).
 */
export const diffParts = (before, after) => {
  const a = units(before);
  const b = units(after);
  const totalBytes = b.reduce((n, u) => n + (u.t === "c" ? utf8Length(u.v) : 0), 0);

  // Trim the common prefix and suffix before the diff proper.
  let pre = 0;
  while (pre < a.length && pre < b.length && same(a[pre], b[pre])) pre++;
  let suf = 0;
  while (suf < a.length - pre && suf < b.length - pre && same(a[a.length - 1 - suf], b[b.length - 1 - suf])) suf++;
  const mid = myers(a.slice(pre, a.length - suf), b.slice(pre, b.length - suf));
  if (mid === null) {
    return { hunks: [], changedBytes: totalBytes, totalBytes, runs: 0, insertedRuns: 0, identical: false, tooLarge: true };
  }
  const ops = [
    ...b.slice(0, pre).map((unit) => ({ op: "eq", unit })),
    ...mid,
    ...b.slice(b.length - suf).map((unit) => ({ op: "eq", unit })),
  ];

  const hunks = [];
  for (const o of ops) {
    const lastHunk = hunks[hunks.length - 1];
    if (lastHunk && lastHunk.op === o.op) lastHunk.units.push(o.unit);
    else hunks.push({ op: o.op, units: [o.unit] });
  }

  let changedBytes = 0;
  let runs = 0;
  let insertedRuns = 0;
  let inRun = false;
  let runHasDelete = false;
  const closeRun = () => {
    if (inRun && !runHasDelete) insertedRuns++;
    inRun = false;
    runHasDelete = false;
  };
  for (const h of hunks) {
    if (h.op === "eq") {
      closeRun();
      continue;
    }
    if (!inRun) {
      runs++;
      inRun = true;
    }
    if (h.op === "del") runHasDelete = true;
    for (const u of h.units) {
      if (u.t === "c") changedBytes += utf8Length(u.v);
      else changedBytes += 1; // a slot change is never "no change"
    }
  }
  closeRun();

  return { hunks, changedBytes, totalBytes, runs, insertedRuns, identical: runs === 0, tooLarge: false };
};

/**
 * isSmallEdit reports a change small enough to hide an injection in: less
 * than SMALL_EDIT_SHARE of the constant bytes, or a single inserted run.
 */
export const isSmallEdit = (d) => {
  if (!d || d.identical || d.tooLarge) return false;
  if (d.totalBytes > 0 && d.changedBytes / d.totalBytes < SMALL_EDIT_SHARE) return true;
  return d.runs === 1 && d.insertedRuns === 1;
};

/** samePlacement reports whether two placements are the same. */
export const samePlacement = (a, b) =>
  (a?.role || "") === (b?.role || "") &&
  (a?.message || "any") === (b?.message || "any") &&
  (a?.anchor || "both") === (b?.anchor || "both");
