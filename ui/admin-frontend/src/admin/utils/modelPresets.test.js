import modelPresets from "./modelPresets";

// 0 is not sent (models/llm_settings.go), so a new call setting leaves
// sampling to the vendor: Claude Sonnet 5 and later reject temperature and
// top_p with a 400.
test("the default call settings send no temperature or top_p", () => {
  expect(modelPresets.default.temperature).toBe(0);
  expect(modelPresets.default.top_p).toBe(0);
});
