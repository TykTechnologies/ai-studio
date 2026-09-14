// Small, dependency-free structural comparison used by the unsaved-changes
// tracking. It deliberately treats a key whose value is `undefined` the same
// as a missing key: forms routinely spread server payloads that omit optional
// attributes, and a baseline of `{ description: undefined }` must not read as
// different from `{}`.

const isPlainObject = (value) =>
  value !== null &&
  typeof value === "object" &&
  (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);

const definedKeys = (obj) => Object.keys(obj).filter((key) => obj[key] !== undefined);

export const deepEqual = (a, b) => {
  if (Object.is(a, b)) return true;

  if (a instanceof Date || b instanceof Date) {
    return a instanceof Date && b instanceof Date && a.getTime() === b.getTime();
  }

  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i += 1) {
      if (!deepEqual(a[i], b[i])) return false;
    }
    return true;
  }

  if (a instanceof Set || b instanceof Set) {
    if (!(a instanceof Set) || !(b instanceof Set) || a.size !== b.size) return false;
    for (const item of a) {
      if (!b.has(item)) return false;
    }
    return true;
  }

  if (isPlainObject(a) && isPlainObject(b)) {
    const keysA = definedKeys(a);
    const keysB = definedKeys(b);
    if (keysA.length !== keysB.length) return false;
    for (const key of keysA) {
      if (!Object.prototype.hasOwnProperty.call(b, key)) return false;
      if (!deepEqual(a[key], b[key])) return false;
    }
    return true;
  }

  // Anything else (numbers that are not identical, strings, functions, class
  // instances) is compared by identity, which Object.is already rejected.
  return false;
};

// Structural clone for the baseline snapshot. `structuredClone` is not
// available in every test runtime, and the values forms hand us are plain
// data anyway.
export const deepClone = (value) => {
  if (value === null || typeof value !== "object") return value;
  if (value instanceof Date) return new Date(value.getTime());
  if (Array.isArray(value)) return value.map(deepClone);
  if (value instanceof Set) return new Set([...value].map(deepClone));
  if (isPlainObject(value)) {
    const out = {};
    for (const key of Object.keys(value)) {
      if (value[key] !== undefined) out[key] = deepClone(value[key]);
    }
    return out;
  }
  // Class instances (e.g. File) are kept by reference; they are compared by
  // identity, which is the right call for an attached upload.
  return value;
};
