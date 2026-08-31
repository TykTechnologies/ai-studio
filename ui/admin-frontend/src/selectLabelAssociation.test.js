const fs = require("fs");
const path = require("path");

/**
 * Every MUI <Select> must be programmatically associated with a label.
 *
 * Before this sweep the codebase had 63 <InputLabel>s carrying 2 ids and 70
 * <Select>s carrying zero labelIds, so on essentially every select in the
 * product the visible label was not associated with the control: screen
 * readers announced an unnamed combobox and getByLabel resolved to nothing.
 *
 * This asserts the count stays at zero rather than trusting a one-off fix.
 */

const SRC = path.join(__dirname);

const collectFiles = (dir, acc = []) => {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules") continue;
      collectFiles(full, acc);
    } else if (entry.name.endsWith(".js") && !entry.name.includes(".test.")) {
      acc.push(full);
    }
  }
  return acc;
};

/** Returns the index of the '>' closing an opening tag starting at `start`. */
const openingTagEnd = (source, start) => {
  let depth = 0;
  for (let i = start; i < source.length; i += 1) {
    const ch = source[i];
    if (ch === "{") depth += 1;
    else if (ch === "}") depth -= 1;
    else if (ch === ">" && depth === 0) return i;
  }
  return -1;
};

/**
 * Spans of source covered by a `.map(` callback.
 *
 * A literal label id inside one of these is written once but rendered once per
 * item, so the id repeats in the DOM and every label after the first points at
 * the first control -- invalid HTML, and it undoes the association this file
 * exists to enforce. The per-file uniqueness check above cannot see it: the id
 * appears once in the source. Quoted text is skipped so brackets in ordinary
 * prose ("(see docs)") do not throw the bracket matching off.
 */
const mapCallbackSpans = (source) => {
  const spans = [];
  const re = /\.map\(/g;
  let match;

  while ((match = re.exec(source)) !== null) {
    const start = match.index + match[0].length;
    let depth = 1;
    let quote = null;

    for (let i = start; i < source.length; i += 1) {
      const ch = source[i];

      if (quote) {
        if (ch === "\\") i += 1;
        else if (ch === quote) quote = null;
        continue;
      }
      if (ch === '"' || ch === "'" || ch === "`") {
        quote = ch;
        continue;
      }
      if (ch === "(" || ch === "{" || ch === "[") depth += 1;
      else if (ch === ")" || ch === "}" || ch === "]") {
        depth -= 1;
        if (depth === 0) {
          spans.push([start, i]);
          break;
        }
      }
    }
  }

  return spans;
};

describe("Select label association", () => {
  const files = collectFiles(SRC);

  it("has no <Select> without labelId, aria-labelledby or aria-label", () => {
    const offenders = [];

    for (const file of files) {
      const source = fs.readFileSync(file, "utf8");
      const re = /<Select\b/g;
      let match;
      while ((match = re.exec(source)) !== null) {
        const end = openingTagEnd(source, match.index + match[0].length);
        if (end === -1) continue;
        const tag = source.slice(match.index, end);
        const associated =
          /\blabelId=/.test(tag) ||
          /\baria-labelledby=/.test(tag) ||
          /\baria-label=/.test(tag);
        if (!associated) {
          const line = source.slice(0, match.index).split("\n").length;
          offenders.push(`${path.relative(SRC, file)}:${line}`);
        }
      }
    }

    expect(offenders).toEqual([]);
  });

  it("uses unique labelIds within each file", () => {
    const collisions = [];

    for (const file of files) {
      const source = fs.readFileSync(file, "utf8");
      const ids = [...source.matchAll(/labelId="([^"]+)"/g)].map((m) => m[1]);
      const seen = new Set();
      for (const id of ids) {
        if (seen.has(id)) collisions.push(`${path.relative(SRC, file)}: ${id}`);
        seen.add(id);
      }
    }

    expect(collisions).toEqual([]);
  });

  it("has no literal label id inside a .map() callback", () => {
    const offenders = [];

    for (const file of files) {
      const source = fs.readFileSync(file, "utf8");
      const spans = mapCallbackSpans(source);
      if (spans.length === 0) continue;

      // labelId on the Select, and the InputLabel id it points at. Other ids
      // are left alone: only these two have to be unique per rendered row for
      // the label association to survive.
      const re = /\blabelId="([^"]+)"|<InputLabel\b[^>]*?\sid="([^"]+)"/g;
      let match;
      while ((match = re.exec(source)) !== null) {
        const inLoop = spans.some(
          ([start, end]) => match.index > start && match.index < end,
        );
        if (!inLoop) continue;
        const line = source.slice(0, match.index).split("\n").length;
        offenders.push(
          `${path.relative(SRC, file)}:${line} ${match[1] || match[2]}`,
        );
      }
    }

    expect(offenders).toEqual([]);
  });

  it("has no literal label id in a shared component", () => {
    // A component under common/ is a field other screens embed, so a page can
    // hold several of them. A literal id there repeats in the DOM for the same
    // reason a literal id inside a map does -- it is just the rendering that
    // loops rather than the source. useId() gives each instance its own.
    const offenders = [];

    for (const file of files) {
      const relative = path.relative(SRC, file);
      if (!relative.includes(`${path.sep}common${path.sep}`)) continue;

      const source = fs.readFileSync(file, "utf8");
      const re = /\blabelId="([^"]+)"|<InputLabel\b[^>]*?\sid="([^"]+)"/g;
      let match;
      while ((match = re.exec(source)) !== null) {
        const line = source.slice(0, match.index).split("\n").length;
        offenders.push(`${relative}:${line} ${match[1] || match[2]}`);
      }
    }

    expect(offenders).toEqual([]);
  });
});
