import { PRIVACY_LEVELS, normalizePrivacyScore, privacyLevelForScore, formatPrivacyLevel, isValidPrivacyScore } from "./privacyLevels";

describe("privacyLevels", () => {
  it("bands cover 0–100 without gaps or overlap", () => {
    let next = 0;
    PRIVACY_LEVELS.forEach((l) => { expect(l.min).toBe(next); next = l.max + 1; });
    expect(next).toBe(101);
  });
  it("maps every score to exactly one level", () => {
    expect(privacyLevelForScore(0).key).toBe("public");
    expect(privacyLevelForScore(25).key).toBe("public");
    expect(privacyLevelForScore(26).key).toBe("internal");
    expect(privacyLevelForScore(75).key).toBe("confidential");
    expect(privacyLevelForScore(76).key).toBe("restricted");
    expect(privacyLevelForScore(100).key).toBe("restricted");
    expect(privacyLevelForScore(null)).toBeNull();
    expect(privacyLevelForScore("")).toBeNull();
  });
  it("normalises strings and clamps out-of-range values", () => {
    expect(normalizePrivacyScore("40")).toBe(40);
    expect(normalizePrivacyScore(140)).toBe(100);
    expect(normalizePrivacyScore(-3)).toBe(0);
    expect(normalizePrivacyScore("abc")).toBeNull();
  });
  it("formats level and number together", () => {
    expect(formatPrivacyLevel(40)).toBe("Internal · 40");
    expect(formatPrivacyLevel(undefined)).toBe("Not set");
  });
  it("validates the stored range", () => {
    expect(isValidPrivacyScore(0)).toBe(true);
    expect(isValidPrivacyScore(100)).toBe(true);
    expect(isValidPrivacyScore(101)).toBe(false);
    expect(isValidPrivacyScore("")).toBe(false);
    expect(isValidPrivacyScore(12.5)).toBe(false);
  });
});
