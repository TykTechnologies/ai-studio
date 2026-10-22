import React from "react";
import { render } from "@testing-library/react";
// The filter editor's highlighter loads first here (admin, then chat), the
// reverse of FilterForm.afterChat.test.js: chat code blocks must stay
// highlighted in both chat UIs either way.
import highlightScript from "../../../admin/components/filters/highlightScript";
import MarkdownMessage from "../chat/MarkdownMessage";
import { PrismHighlighter } from "./MarkdownText";

// One sample per commonly used language, under its name and the aliases the
// Prism build registers (not "sh": refractor's bash only aliases "shell", so
// a sh block has always rendered plain).
const samples = {
  javascript: ["const total = items.map((i) => i.price);", ["js"]],
  typescript: ["interface User { id: number }", ["ts"]],
  python: ["def handler(event):\n    return None", ["py"]],
  bash: ['if [ -n "$HOME" ]; then echo ok; fi', ["shell"]],
  json: ['{"enabled": true, "count": 3}', []],
  yaml: ["enabled: true\ncount: 3", ["yml"]],
  go: ["func main() { return }", []],
  sql: ["SELECT id FROM users WHERE active = true", []],
};

const cases = Object.entries(samples).flatMap(([lang, [code, aliases]]) =>
  [lang, ...aliases].map((name) => [name, code]),
);

// A highlighted block has Prism token spans; a plain one has none.
const tokens = (container) =>
  // eslint-disable-next-line testing-library/no-node-access
  Array.from(container.querySelectorAll("code span")).filter((s) => /\btoken\b/.test(s.className));

describe("chat code blocks stay highlighted", () => {
  it.each(cases)("chat v1, %s", (lang, code) => {
    const { container } = render(<MarkdownMessage content={"```" + lang + "\n" + code + "\n```"} />);
    expect(tokens(container).length).toBeGreaterThan(0);
  });

  it.each(cases)("chat v2, %s", (lang, code) => {
    const { container } = render(<PrismHighlighter language={lang} code={code} />);
    expect(tokens(container).length).toBeGreaterThan(0);
  });

  // Admin first, then chat: Prism.highlight is patched to return hast
  // objects now, which is what crashed the editor; its highlighter is not.
  it("leaves the filter editor's highlighter returning markup", () => {
    render(<MarkdownMessage content={"```js\nconst x = 1;\n```"} />);
    expect(highlightScript("const x = 1;")).toBe(
      '<span class="token keyword">const</span> x <span class="token operator">=</span> <span class="token number">1</span><span class="token punctuation">;</span>',
    );
  });

  it("leaves an unlabelled block plain", () => {
    const { container } = render(<MarkdownMessage content={"```\nplain text\n```"} />);
    expect(tokens(container)).toHaveLength(0);
  });
});
