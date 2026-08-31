import {
  commitPendingSelection,
  hasPendingSelection,
} from "./pendingSelection";

const available = [
  { id: 1, name: "OpenAI" },
  { id: 2, name: "Anthropic" },
];

describe("commitPendingSelection", () => {
  it("folds a pending selection into the list", () => {
    // The defect this exists to prevent: the user picked OpenAI, never pressed
    // +, and pressed save -- which used to produce an empty catalog.
    expect(commitPendingSelection([], 1, available)).toEqual([
      { id: 1, name: "OpenAI" },
    ]);
  });

  it("appends to an existing list without disturbing it", () => {
    const items = [{ id: 2, name: "Anthropic" }];
    expect(commitPendingSelection(items, 1, available)).toEqual([
      { id: 2, name: "Anthropic" },
      { id: 1, name: "OpenAI" },
    ]);
  });

  it("returns the list unchanged when nothing is pending", () => {
    const items = [{ id: 1, name: "OpenAI" }];
    expect(commitPendingSelection(items, "", available)).toBe(items);
  });

  it("does not duplicate an item already added", () => {
    const items = [{ id: 1, name: "OpenAI" }];
    expect(commitPendingSelection(items, 1, available)).toBe(items);
  });

  it("ignores a selection that cannot be resolved", () => {
    const items = [];
    expect(commitPendingSelection(items, 99, available)).toBe(items);
    expect(commitPendingSelection(items, 1, undefined)).toBe(items);
  });
});

describe("hasPendingSelection", () => {
  it("is true only for a selection not yet added", () => {
    expect(hasPendingSelection([], 1)).toBe(true);
    expect(hasPendingSelection([{ id: 1 }], 1)).toBe(false);
    expect(hasPendingSelection([], "")).toBe(false);
  });
});
