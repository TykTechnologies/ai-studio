import {
  classifyCodePoint,
  constError,
  expiresAt,
  isInvisible,
  previousApproved,
  tokenize,
  utf8Length,
  validateTemplate,
} from "./promptTemplateText";

const pad = (s) => s + "x".repeat(40);

describe("constError mirrors the API", () => {
  it("measures the minimum in UTF-8 bytes, not characters", () => {
    expect(constError("x".repeat(31))).toMatch(/at least 32 bytes \(this one is 31\)/);
    expect(constError("x".repeat(32))).toBeNull();
    // 11 two-byte characters are 22 bytes; 16 are 32.
    expect(constError("\u00e9".repeat(11))).toMatch(/this one is 22/);
    expect(constError("\u00e9".repeat(16))).toBeNull();
  });

  it("refuses invisible characters, naming them and their byte offset", () => {
    expect(constError(pad("You are\u200breviewing"))).toMatch(/invisible characters \(ZWSP U\+200B at byte 7\)/);
    expect(constError(pad("abc\u202e"))).toMatch(/RLO U\+202E/);
    expect(constError(pad("abc\u{e0041}"))).toMatch(/U\+E0041/);
    expect(constError(pad("abc\ufe0f"))).toMatch(/U\+FE0F/);
    expect(constError(pad("abc\u00ad"))).toMatch(/SHY U\+00AD/);
  });

  it("refuses the placeholder and a leading combining mark", () => {
    expect(constError(pad("[app template]"))).toMatch(/placeholder/);
    expect(constError(pad("\u0301abc"))).toMatch(/combining mark/);
  });

  it("accepts ordinary non-ASCII text and a no-break space", () => {
    // The API refuses only invisible characters; the approval view marks
    // the rest.
    expect(constError(pad("caf\u00e9 \u00a0 \u0430"))).toBeNull();
  });
});

describe("isInvisible", () => {
  it.each([0x200b, 0x200d, 0x202e, 0x2066, 0xfeff, 0xe0001, 0xfe0f, 0xe0100, 0x034f, 0x3164, 0x2800, 0xfffd])(
    "U+%s is invisible",
    (cp) => expect(isInvisible(cp)).toBe(true)
  );
  it.each([0x41, 0x0a, 0xe9, 0x0430, 0x00a0, 0x0301])("U+%s is not", (cp) => expect(isInvisible(cp)).toBe(false));
});

describe("tokenize", () => {
  it("leaves printable ASCII and line feeds alone", () => {
    expect(tokenize("Review this.\nThen stop.")).toEqual([{ type: "text", text: "Review this.\nThen stop." }]);
  });

  it("labels each kind of hidden or lookalike character", () => {
    const kinds = tokenize("a\u200bb\u202ec\u{e0049}d\ufe0fe\u00a0f\u2003g\u00adh\u0430i\tj")
      .filter((t) => t.type === "marker")
      .map((t) => [t.kind, t.label]);
    expect(kinds).toEqual([
      ["zero-width", "ZWSP U+200B"],
      ["bidi", "RLO U+202E"],
      ["tag", "U+E0049"],
      ["variation-selector", "U+FE0F"],
      ["nbsp", "NBSP U+00A0"],
      ["space", "U+2003"],
      ["non-printing", "SHY U+00AD"],
      ["lookalike", "U+0430"],
      ["control", "TAB U+0009"],
    ]);
  });

  it("classifies code points", () => {
    expect(classifyCodePoint(0x41)).toBeNull();
    expect(classifyCodePoint(0x0d)).toBe("control");
    expect(classifyCodePoint(0x2060)).toBe("zero-width");
  });
});

describe("validateTemplate", () => {
  const good = {
    name: "pr-review",
    placement: { role: "user", message: "any", anchor: "both" },
    parts: [
      { kind: "const", const: "You are reviewing a pull request. Follow these rules.\n<diff>\n" },
      { kind: "slot", slot: "diff" },
      { kind: "const", const: "\n</diff>\nReport only real defects, one per line.\n" },
    ],
    expiresAfterDays: 30,
  };

  it("accepts a valid template", () => {
    expect(validateTemplate(good)).toEqual({ form: [], parts: {} });
  });

  it("reports the API's structural rules", () => {
    const v = validateTemplate({
      ...good,
      name: "pr review",
      expiresAfterDays: 0,
      parts: [{ kind: "slot", slot: "a" }, { kind: "slot", slot: "b" }, { kind: "const", const: "short" }],
    });
    expect(v.form).toEqual(
      expect.arrayContaining([
        expect.stringMatching(/^Name/),
        expect.stringMatching(/^Expires after/),
        "A template anchored at the start must start with a constant.",
      ])
    );
    expect(v.parts[1]).toMatch(/Two slots in a row/);
    expect(v.parts[2]).toMatch(/at least 32 bytes/);
  });

  it("refuses two constants in a row", () => {
    const v = validateTemplate({ ...good, parts: [good.parts[0], good.parts[2]] });
    expect(v.parts[1]).toMatch(/Two constants in a row/);
  });
});

describe("expiry and history", () => {
  it("counts expiry from the later of approval and last match", () => {
    const t = {
      status: "approved",
      expires_after_days: 30,
      approved_at: "2026-10-01T00:00:00Z",
      last_matched_at: "2026-10-10T00:00:00Z",
    };
    expect(expiresAt(t).toISOString()).toBe("2026-11-09T00:00:00.000Z");
    expect(expiresAt({ ...t, last_matched_at: null }).toISOString()).toBe("2026-10-31T00:00:00.000Z");
    expect(expiresAt({ ...t, status: "proposed" })).toBeNull();
  });

  it("finds the previously approved version of the same name", () => {
    const all = [
      { id: 1, name: "pr-review", version: 1, status: "retired", approved_at: "2026-09-01T00:00:00Z" },
      { id: 2, name: "pr-review", version: 2, status: "retired", approved_at: null },
      { id: 3, name: "pr-review", version: 3, status: "approved", approved_at: "2026-10-01T00:00:00Z" },
      { id: 4, name: "pr-review", version: 4, status: "proposed", approved_at: null },
      { id: 5, name: "summary", version: 9, status: "approved", approved_at: "2026-10-01T00:00:00Z" },
    ];
    expect(previousApproved(all[3], all).id).toBe(3);
    expect(previousApproved(all[2], all).id).toBe(1);
    expect(previousApproved(all[0], all)).toBeNull();
  });
});

it("utf8Length counts bytes", () => {
  expect(utf8Length("a\u00e9\u20ac\u{1f600}")).toBe(1 + 2 + 3 + 4);
});
