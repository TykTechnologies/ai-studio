import { renderHook, act } from "@testing-library/react";
import { useFormDirty } from "./useFormDirty";
import { deepEqual, deepClone } from "./deepEqual";

describe("useFormDirty", () => {
  it("is clean on the first render and dirty after a change", () => {
    const { result, rerender } = renderHook(({ values }) => useFormDirty(values), {
      initialProps: { values: { name: "", tags: [] } },
    });
    expect(result.current.isDirty).toBe(false);

    rerender({ values: { name: "x", tags: [] } });
    expect(result.current.isDirty).toBe(true);

    rerender({ values: { name: "", tags: [] } });
    expect(result.current.isDirty).toBe(false);
  });

  it("waits for ready before taking the baseline", () => {
    const { result, rerender } = renderHook(({ values, ready }) => useFormDirty(values, { ready }), {
      initialProps: { values: { name: "" }, ready: false },
    });
    // Data arrives: not a user change.
    rerender({ values: { name: "loaded" }, ready: false });
    expect(result.current.isDirty).toBe(false);
    rerender({ values: { name: "loaded" }, ready: true });
    expect(result.current.isDirty).toBe(false);

    rerender({ values: { name: "loaded!" }, ready: true });
    expect(result.current.isDirty).toBe(true);
  });

  it("markSaved re-baselines on the current values", () => {
    const { result, rerender } = renderHook(({ values }) => useFormDirty(values), {
      initialProps: { values: { name: "a" } },
    });
    rerender({ values: { name: "b" } });
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.markSaved());
    expect(result.current.isDirty).toBe(false);

    rerender({ values: { name: "a" } });
    expect(result.current.isDirty).toBe(true);
  });

  it("reset drops the baseline and retakes it on the next ready render", () => {
    const { result, rerender } = renderHook(({ values }) => useFormDirty(values), {
      initialProps: { values: { name: "a" } },
    });
    rerender({ values: { name: "b" } });
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.reset());
    expect(result.current.isDirty).toBe(false);
    rerender({ values: { name: "c" } });
    expect(result.current.isDirty).toBe(true);
  });

  it("compares nested objects and arrays structurally", () => {
    const initial = {
      pools: [{ name: "p1", vendors: [{ llm_id: 1, mappings: [] }] }],
      config: { nested: { deep: true } },
    };
    const { result, rerender } = renderHook(({ values }) => useFormDirty(values), {
      initialProps: { values: initial },
    });
    // A structurally identical object built fresh is not a change.
    rerender({ values: deepClone(initial) });
    expect(result.current.isDirty).toBe(false);

    rerender({
      values: {
        pools: [{ name: "p1", vendors: [{ llm_id: 1, mappings: [{ source_model: "a" }] }] }],
        config: { nested: { deep: true } },
      },
    });
    expect(result.current.isDirty).toBe(true);

    rerender({
      values: { pools: [{ name: "p1", vendors: [{ llm_id: 1, mappings: [] }] }], config: { nested: { deep: false } } },
    });
    expect(result.current.isDirty).toBe(true);
  });

  it("is not fooled by mutation of the values object", () => {
    const values = { tags: ["a"] };
    const { result, rerender } = renderHook(({ v }) => useFormDirty(v), { initialProps: { v: values } });
    values.tags.push("b");
    rerender({ v: values });
    expect(result.current.isDirty).toBe(true);
  });
});

describe("deepEqual", () => {
  it("treats undefined keys as missing", () => {
    expect(deepEqual({ a: 1, b: undefined }, { a: 1 })).toBe(true);
    expect(deepEqual({ a: 1 }, { a: 1, b: null })).toBe(false);
  });

  it("handles primitives, arrays, sets and dates", () => {
    expect(deepEqual(1, 1)).toBe(true);
    expect(deepEqual(1, "1")).toBe(false);
    expect(deepEqual([1, 2], [1, 2])).toBe(true);
    expect(deepEqual([1, 2], [2, 1])).toBe(false);
    expect(deepEqual(new Set([1, 2]), new Set([2, 1]))).toBe(true);
    expect(deepEqual(new Set([1]), new Set([1, 2]))).toBe(false);
    expect(deepEqual(new Date(1000), new Date(1000))).toBe(true);
    expect(deepEqual(new Date(1000), new Date(2000))).toBe(false);
    expect(deepEqual(null, undefined)).toBe(false);
    expect(deepEqual(NaN, NaN)).toBe(true);
  });
});
