import { encodeSpec, fetchSpecFromURL } from "./specUtils";

// Decodes what encodeSpec produced back into a JS string, the way the server
// does: base64 -> bytes -> UTF-8.
const decodeSpec = (encoded) => {
  const binary = atob(encoded);
  const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
};

describe("encodeSpec", () => {
  it("round-trips an ASCII spec", () => {
    const spec = '{"openapi":"3.0.0","info":{"title":"Rates"}}';
    expect(decodeSpec(encodeSpec(spec))).toBe(spec);
  });

  it("round-trips characters above the Latin-1 range", () => {
    // Curly quotes and an em dash: btoa() throws on these.
    const spec = '{"info":{"title":"Canada’s holidays — “v1”"}}';
    expect(decodeSpec(encodeSpec(spec))).toBe(spec);
  });

  it("round-trips Latin-1 characters as UTF-8", () => {
    // btoa() accepted these but wrote them as single bytes, so the stored
    // base64 was not valid UTF-8 and came back as mojibake.
    const spec = '{"info":{"title":"Jours fériés"}}';
    expect(decodeSpec(encodeSpec(spec))).toBe(spec);
  });

  it("round-trips multi-byte scripts and astral characters", () => {
    const spec = '{"info":{"title":"祝日 🍁"}}';
    expect(decodeSpec(encodeSpec(spec))).toBe(spec);
  });

  it("stringifies an object spec", () => {
    const spec = { info: { title: "café" } };
    expect(decodeSpec(encodeSpec(spec))).toBe(JSON.stringify(spec));
  });

  it("handles a spec larger than one encoding chunk", () => {
    const spec = `{"description":"${"é".repeat(40000)}"}`;
    expect(decodeSpec(encodeSpec(spec))).toBe(spec);
  });
});

describe("fetchSpecFromURL", () => {
  it("returns the body of a successful response", async () => {
    const fetchImpl = jest.fn().mockResolvedValue({ ok: true, status: 200, text: () => Promise.resolve("openapi: 3.0.0") });
    await expect(fetchSpecFromURL("https://example.com/spec.yaml", fetchImpl)).resolves.toBe("openapi: 3.0.0");
  });

  it("explains CORS and suggests upload or paste when the browser fetch fails", async () => {
    const fetchImpl = jest.fn().mockRejectedValue(new TypeError("Failed to fetch"));
    const err = await fetchSpecFromURL("https://example.com/spec.yaml", fetchImpl).catch((e) => e);
    expect(err.message).toMatch(/CORS/);
    expect(err.message).toMatch(/upload it, or paste/);
  });

  it("names the HTTP status when the server answers with an error", async () => {
    const fetchImpl = jest.fn().mockResolvedValue({ ok: false, status: 404, text: () => Promise.resolve("") });
    await expect(fetchSpecFromURL("https://example.com/missing", fetchImpl)).rejects.toThrow(/HTTP 404/);
  });
});
