/**
 * One privacy scale for the whole product.
 *
 * The stored value is `privacy_score`, an integer 0–100 (0 lowest, 100
 * highest). Every screen presents it as a named level plus the number, so
 * an administrator never has to remember what "25" means. The bands cover
 * the full range (the quick-start's older mapping had nothing below 25):
 *
 *   Public        0–25   safe to share (blogs, press releases)
 *   Internal     26–50   limited to the organisation (reports, policies)
 *   Confidential 51–75   sensitive (financials, strategies)
 *   Restricted   76–100  personal or regulated data (PII, customer records)
 *
 * The comparison rule stays numeric: an LLM provider may only be used with
 * tools and data sources whose level is at or below its own.
 */
export const PRIVACY_MIN = 0;
export const PRIVACY_MAX = 100;

export const PRIVACY_LEVELS = [
  { key: "public", label: "Public", min: 0, max: 25, defaultScore: 25, description: "Safe to share data (e.g. blogs, press releases)" },
  { key: "internal", label: "Internal", min: 26, max: 50, defaultScore: 50, description: "Limited to users within the organisation (e.g. reports, policies)" },
  { key: "confidential", label: "Confidential", min: 51, max: 75, defaultScore: 75, description: "Sensitive data (e.g. financials, strategies)" },
  { key: "restricted", label: "Restricted", min: 76, max: 100, defaultScore: 100, description: "Personal or regulated data (e.g. names, emails, customer records)" },
];

/** Clamp and coerce anything the API or a form might hand us to 0–100, or null. */
export function normalizePrivacyScore(value) {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  if (Number.isNaN(n)) return null;
  return Math.min(PRIVACY_MAX, Math.max(PRIVACY_MIN, Math.round(n)));
}

/** The band a score falls in; null when the score is not set. */
export function privacyLevelForScore(value) {
  const score = normalizePrivacyScore(value);
  if (score === null) return null;
  return PRIVACY_LEVELS.find((l) => score >= l.min && score <= l.max) || null;
}

export function privacyLevelByKey(key) {
  return PRIVACY_LEVELS.find((l) => l.key === key) || null;
}

/** "Internal · 40", or "Not set". */
export function formatPrivacyLevel(value) {
  const score = normalizePrivacyScore(value);
  if (score === null) return "Not set";
  const level = privacyLevelForScore(score);
  return level ? `${level.label} · ${score}` : String(score);
}

export function isValidPrivacyScore(value) {
  const n = Number(value);
  return value !== "" && value !== null && value !== undefined && Number.isInteger(n) && n >= PRIVACY_MIN && n <= PRIVACY_MAX;
}
