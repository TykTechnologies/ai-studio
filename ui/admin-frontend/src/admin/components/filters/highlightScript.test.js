import { highlight, languages } from "prismjs/components/prism-core";
import "prismjs/components/prism-clike";
import "prismjs/components/prism-javascript";
import highlightScript from "./highlightScript";

// highlightScript must produce exactly what Prism.highlight did for the
// script editor before, so the editor looks the same (prism-tomorrow).
// No chat code is loaded in this file, so Prism.highlight is unpatched.
describe("highlightScript", () => {
  const scripts = [
    "",
    "const a = 1;",
    `// block requests mentioning a <secret> & more here
function filter(input) {
  if (input.messages.some((m) => /secret/i.test(m.content))) {
    return { block: true, message: \`blocked: \${input.app_id}\` };
  }
  return { block: false }; /* done */
}`,
  ];

  it.each(scripts.map((s) => [s]))("matches Prism.highlight for %j", (script) => {
    expect(highlightScript(script)).toBe(highlight(script, languages.js, "javascript"));
  });

  it("escapes markup in the script", () => {
    expect(highlightScript('"<img src=x>"')).toContain("&lt;img src=x>");
  });

  // Security review of the editor highlighter: script text never becomes
  // markup, wherever the grammar puts it.
  describe("hostile payloads", () => {
    const payloads = ["</span><img src=x onerror=alert(1)>", "<script>alert(1)</script>"];
    const places = {
      comment: (p) => `// ${p}\n/* ${p} */`,
      string: (p) => `const a = "${p}"; const b = '${p}';`,
      template: (p) => `const t = \`${p} \${x} ${p}\`;`,
      regex: (p) => `const r = /${p.replace(/\//g, "\\/")}/g;`,
      bare: (p) => p,
    };
    const cases = payloads.flatMap((payload) =>
      Object.entries(places).map(([place, wrap]) => [place, payload, wrap(payload)]),
    );

    it.each(cases)("in a %s, %s, matches Prism.highlight", (_place, _payload, script) => {
      expect(highlightScript(script)).toBe(highlight(script, languages.js, "javascript"));
    });

    it.each(cases)("in a %s, %s, renders as text only", (_place, _payload, script) => {
      const div = document.createElement("div");
      div.innerHTML = highlightScript(script);
      // eslint-disable-next-line testing-library/no-node-access
      expect(div.querySelector("img, script")).toBeNull();
      expect(div.textContent).toBe(script);
    });
  });
});
