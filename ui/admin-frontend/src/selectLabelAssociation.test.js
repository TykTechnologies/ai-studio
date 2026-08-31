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
});
