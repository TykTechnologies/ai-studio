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
});
