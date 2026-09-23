import { routerModelStrings } from "./routerModels";

describe("routerModelStrings", () => {
  it("lists literal patterns and mapping sources under the router slug, skipping globs", () => {
    const pools = [
      { model_pattern: "gpt-4o, gpt-4o-mini", vendors: [] },
      { model_pattern: "claude-*", vendors: [{ mappings: [{ source_model: "fast", target_model: "claude-haiku" }] }] },
      { model_pattern: "model-[ab]", vendors: [{ mappings: [{ source_model: "gpt-4o", target_model: "x" }] }] },
      { model_pattern: "?", vendors: [] },
    ];
    expect(routerModelStrings("prod", pools)).toEqual(["prod/gpt-4o", "prod/gpt-4o-mini", "prod/fast"]);
  });

  it("returns nothing when every pool is a glob", () => {
    expect(routerModelStrings("prod", [{ model_pattern: "*", vendors: [] }])).toEqual([]);
    expect(routerModelStrings("prod", undefined)).toEqual([]);
  });
});
