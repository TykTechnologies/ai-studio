// Trusted prompt templates: the text rules the API applies
// (guardrails/templates/templates.go), and how the approval view shows
// characters a reviewer could not otherwise see.
//
// Everything here works on code points and measures UTF-8 bytes, as the API
// does: a constant's 32-byte minimum is bytes, not characters.

export const MIN_CONST_BYTES = 32;
export const MAX_CONST_BYTES = 64 * 1024;
export const MAX_PARTS = 64;
export const MAX_SLOT_NAME_LEN = 64;
export const MAX_APPROVED_PER_APP = 10;
export const DEFAULT_EXPIRES_AFTER_DAYS = 30;
export const PLACEHOLDER = "[app template]";
export const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

export const ROLES = ["system", "user"];
export const MESSAGES = ["first", "last", "any"];
export const ANCHORS = ["start", "end", "both"];

const encoder = new TextEncoder();

/** utf8Length is the number of UTF-8 bytes of s. */
export const utf8Length = (s) => encoder.encode(s || "").length;

const isFormat = (ch) => /\p{Cf}/u.test(ch);
const isMark = (ch) => /\p{M}/u.test(ch);
const isSpaceSeparator = (ch) => /\p{Zs}/u.test(ch);
const isControl = (ch) => /\p{Cc}/u.test(ch);

/** isVariationSelector matches Go's unicode.Variation_Selector. */
const isVariationSelector = (cp) =>
  (cp >= 0x180b && cp <= 0x180d) ||
  cp === 0x180f ||
  (cp >= 0xfe00 && cp <= 0xfe0f) ||
  (cp >= 0xe0100 && cp <= 0xe01ef);

/**
 * isInvisible mirrors templates.Invisible: the characters the API refuses
 * in a constant because they render as nothing or change how their
 * neighbours render.
 */
export const isInvisible = (cp) => {
  const ch = String.fromCodePoint(cp);
  if (isFormat(ch) || isVariationSelector(cp)) return true;
  if (cp >= 0xe0000 && cp <= 0xe0fff) return true;
  switch (cp) {
    case 0x034f: // combining grapheme joiner
    case 0x115f:
    case 0x1160:
    case 0x3164:
    case 0xffa0: // Hangul fillers
    case 0x2800: // braille blank
    case 0x180e: // Mongolian vowel separator
    case 0xfffc:
    case 0xfffd: // object replacement, replacement character
      return true;
    default:
      return false;
  }
};

const hex = (cp) => "U+" + cp.toString(16).toUpperCase().padStart(4, "0");

const ZERO_WIDTH = new Set([0x200b, 0x200c, 0x200d, 0x2060, 0xfeff, 0x180e]);
const BIDI = new Set([0x061c, 0x200e, 0x200f, 0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2066, 0x2067, 0x2068, 0x2069]);
const NBSP = new Set([0x00a0, 0x202f, 0x2007, 0x2011]);
const SHORT_NAMES = {
  0x200b: "ZWSP",
  0x200c: "ZWNJ",
  0x200d: "ZWJ",
  0x2060: "WJ",
  0xfeff: "BOM",
  0x180e: "MVS",
  0x061c: "ALM",
  0x200e: "LRM",
  0x200f: "RLM",
  0x202a: "LRE",
  0x202b: "RLE",
  0x202c: "PDF",
  0x202d: "LRO",
  0x202e: "RLO",
  0x2066: "LRI",
  0x2067: "RLI",
  0x2068: "FSI",
  0x2069: "PDI",
  0x00a0: "NBSP",
  0x202f: "NNBSP",
  0x2007: "FIGURE SPACE",
  0x2011: "NB HYPHEN",
  0x00ad: "SHY",
  0x034f: "CGJ",
  0x0009: "TAB",
  0x000d: "CR",
};

/**
 * Marker kinds, each with the label the approval view shows. "lookalike"
 * is any other character outside ASCII: it renders, but may imitate an
 * ASCII one (a Cyrillic "a", a fullwidth letter, a combining accent).
 */
export const MARKER_KINDS = {
  "zero-width": "zero-width",
  bidi: "bidi control",
  tag: "Unicode tag",
  "variation-selector": "variation selector",
  nbsp: "no-break space",
  space: "unusual space",
  control: "control character",
  "non-printing": "non-printing",
  lookalike: "non-ASCII",
};

/**
 * classifyCodePoint returns the marker kind of a code point, or null for
 * one shown as it is (printable ASCII, and the line feed).
 */
export const classifyCodePoint = (cp) => {
  if (cp === 0x0a) return null;
  if (cp >= 0x20 && cp < 0x7f) return null;
  if (ZERO_WIDTH.has(cp)) return "zero-width";
  if (BIDI.has(cp)) return "bidi";
  if (cp >= 0xe0000 && cp <= 0xe007f) return "tag";
  if (isVariationSelector(cp)) return "variation-selector";
  if (NBSP.has(cp)) return "nbsp";
  const ch = String.fromCodePoint(cp);
  if (isControl(ch)) return "control";
  if (isInvisible(cp)) return "non-printing";
  if (isSpaceSeparator(ch)) return "space";
  return "lookalike";
};

/**
 * describeCodePoint is a marker's text: a short name where there is one,
 * and always the code point, e.g. "ZWSP U+200B".
 */
export const describeCodePoint = (cp) => {
  const name = SHORT_NAMES[cp];
  return name ? `${name} ${hex(cp)}` : hex(cp);
};

/**
 * tokenize splits text into runs of plain text and one token per
 * character that needs a marker: [{type: "text", text}, {type: "marker",
 * kind, label, cp, char}].
 */
export const tokenize = (text) => {
  const out = [];
  let plain = "";
  for (const ch of text || "") {
    const cp = ch.codePointAt(0);
    const kind = classifyCodePoint(cp);
    if (!kind) {
      plain += ch;
      continue;
    }
    if (plain) {
      out.push({ type: "text", text: plain });
      plain = "";
    }
    out.push({ type: "marker", kind, label: describeCodePoint(cp), cp, char: ch });
  }
  if (plain) out.push({ type: "text", text: plain });
  return out;
};

/** markerCounts counts a text's markers by kind. */
export const markerCounts = (text) => {
  const counts = {};
  for (const t of tokenize(text)) {
    if (t.type === "marker") counts[t.kind] = (counts[t.kind] || 0) + 1;
  }
  return counts;
};

/**
 * constError is the API's refusal of one constant, or null when it would
 * accept it (templates.validConst).
 */
export const constError = (text) => {
  const bytes = utf8Length(text);
  if (bytes < MIN_CONST_BYTES) {
    return `A constant must be at least ${MIN_CONST_BYTES} bytes (this one is ${bytes}).`;
  }
  if (bytes > MAX_CONST_BYTES) {
    return `A constant must be at most ${MAX_CONST_BYTES} bytes (this one is ${bytes}).`;
  }
  let offset = 0;
  for (const ch of text) {
    const cp = ch.codePointAt(0);
    if (isInvisible(cp)) {
      return `A constant must not contain invisible characters (${describeCodePoint(cp)} at byte ${offset}).`;
    }
    offset += utf8Length(ch);
  }
  if (text.includes(PLACEHOLDER)) {
    return `A constant must not contain the placeholder "${PLACEHOLDER}".`;
  }
  const first = String.fromCodePoint(text.codePointAt(0));
  if (isMark(first)) {
    return "A constant must not start with a combining mark.";
  }
  return null;
};

/** isConst reports a constant part. */
export const isConst = (part) => typeof part?.const === "string" && part.const !== "";

/**
 * validateTemplate checks a template as the API does (templates.Validate
 * plus the service's name and expiry rules). It returns {form, parts}:
 * form-level errors, and an error per part index.
 */
export const validateTemplate = ({ name, placement, parts, expiresAfterDays }, { checkName = true } = {}) => {
  const form = [];
  const partErrors = {};
  if (checkName && !NAME_PATTERN.test(name || "")) {
    form.push("Name: 1-128 letters, digits, '.', '_' or '-', starting with a letter or digit.");
  }
  const days = Number(expiresAfterDays);
  if (!Number.isInteger(days) || days < 1 || days > 3650) {
    form.push("Expires after: 1-3650 days.");
  }
  if (!ROLES.includes(placement?.role)) form.push("Choose the role the template is placed on.");
  if (!parts || parts.length === 0) {
    form.push("Add at least one constant.");
    return { form, parts: partErrors };
  }
  if (parts.length > MAX_PARTS) form.push(`At most ${MAX_PARTS} parts.`);
  let consts = 0;
  parts.forEach((p, i) => {
    if (p.kind === "slot") {
      const n = p.slot || "";
      if (n === "" || n.length > MAX_SLOT_NAME_LEN || n.trim() !== n) {
        partErrors[i] = `A slot name must be 1-${MAX_SLOT_NAME_LEN} characters without surrounding spaces.`;
      } else if (i > 0 && parts[i - 1].kind === "slot") {
        partErrors[i] = "Two slots in a row cannot be told apart.";
      }
      return;
    }
    consts += 1;
    if (i > 0 && parts[i - 1].kind === "const") {
      partErrors[i] = "Two constants in a row must be one constant.";
      return;
    }
    const err = constError(p.const || "");
    if (err) partErrors[i] = err;
  });
  if (consts === 0) form.push("A template needs at least one constant.");
  const anchor = placement?.anchor || "both";
  if ((anchor === "start" || anchor === "both") && parts[0].kind !== "const") {
    form.push("A template anchored at the start must start with a constant.");
  }
  if ((anchor === "end" || anchor === "both") && parts[parts.length - 1].kind !== "const") {
    form.push("A template anchored at the end must end with a constant.");
  }
  return { form, parts: partErrors };
};

/** toEditorParts turns API parts into editor parts. */
export const toEditorParts = (parts) =>
  (parts || []).map((p) => (isConst(p) ? { kind: "const", const: p.const } : { kind: "slot", slot: p.slot || "" }));

/** fromEditorParts turns editor parts into API parts. */
export const fromEditorParts = (parts) =>
  parts.map((p) => (p.kind === "const" ? { const: p.const } : { slot: p.slot }));

/** constBytes is the total UTF-8 bytes of a template's constants. */
export const constBytes = (parts) =>
  (parts || []).reduce((n, p) => n + (isConst(p) ? utf8Length(p.const) : 0), 0);

/** describePlacement says in words which messages a template is tried on. */
export const describePlacement = (placement) => {
  const role = placement?.role || "user";
  const which = { first: `the first ${role} message`, last: `the last ${role} message`, any: `any ${role} message` }[
    placement?.message || "any"
  ];
  const how = {
    start: "starts the message; text may follow it",
    end: "ends the message; text may precede it",
    both: "is the whole message",
  }[placement?.anchor || "both"];
  return `Tried on ${which}. The template ${how}.`;
};

/** placementLabel is a short placement, e.g. "user / any / both". */
export const placementLabel = (placement) =>
  `${placement?.role || "?"} / ${placement?.message || "any"} / ${placement?.anchor || "both"}`;

/**
 * expiresAt is when an approved template lapses back to proposed without
 * a match (models.AppPromptTemplate.Lapsed), or null when it does not
 * apply.
 */
export const expiresAt = (t) => {
  if (!t || t.status !== "approved" || !(t.expires_after_days > 0)) return null;
  const approved = t.approved_at ? new Date(t.approved_at) : null;
  const matched = t.last_matched_at ? new Date(t.last_matched_at) : null;
  let since = matched;
  if (!since || (approved && approved > since)) since = approved;
  if (!since) return null;
  return new Date(since.getTime() + t.expires_after_days * 24 * 60 * 60 * 1000);
};

/**
 * previousApproved is the version of the same name that was approved
 * before t: the highest lower version that carries an approval (approved,
 * or retired after approval). null when there is none.
 */
export const previousApproved = (t, all) => {
  let best = null;
  for (const o of all || []) {
    if (o.name !== t.name || o.version >= t.version || !o.approved_at) continue;
    if (!best || o.version > best.version) best = o;
  }
  return best;
};
