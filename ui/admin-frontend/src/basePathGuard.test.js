// Studio may be served under a base path (see runtimeConfig.js). The router
// adds it to in-app navigation and the API clients add it to their calls,
// but URLs the browser loads directly (img src, href, window.location,
// window.open, fetch, EventSource, raw axios) and URLs written straight to
// the History API (pushState/replaceState) must go through withBase. These
// tests fail on new root-absolute URLs in those places.
const fs = require("fs");
const path = require("path");

const SRC = __dirname;
const SKIP = new Set(["runtimeConfig.js", "setupProxy.js", "basePathGuard.test.js"]);

const PATTERNS = [
  [/\b(src|href)="\/(?!\/)/, 'src="/..." or href="/..." attribute'],
  [/window\.location\.(href\s*=|assign\(|replace\()\s*['"`]\//, "window.location to a root path"],
  [/window\.open\(\s*['"`]\//, "window.open of a root path"],
  [/\b(fetch|new EventSource)\(\s*['"`]\//, "fetch/EventSource of a root path"],
  [/\baxios\.(get|post|put|patch|delete)\(\s*['"`]\//, "axios call with a root path"],
  [/\.(src|action)\s*=\s*['"`]\//, "element src/action set to a root path"],
  [/['"`]\/logos\//, "/logos/ asset path"],
  [/history\.(pushState|replaceState)\([^)]*,\s*['"`]\//, "history.pushState/replaceState to a root path"],
];

const walk = (dir) =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return walk(full);
    return /\.jsx?$/.test(entry.name) && !/\.test\.jsx?$/.test(entry.name) ? [full] : [];
  });

it("sends root-absolute URLs through withBase", () => {
  const offences = [];
  for (const file of walk(SRC)) {
    if (SKIP.has(path.basename(file))) continue;
    fs.readFileSync(file, "utf8").split("\n").forEach((line, i) => {
      if (line.includes("withBase(")) return;
      for (const [re, what] of PATTERNS) {
        if (re.test(line)) {
          offences.push(`${path.relative(SRC, file)}:${i + 1}: ${what}: ${line.trim()}`);
        }
      }
    });
  }
  expect(offences).toEqual([]);
});

// history.pushState/replaceState bypass the router's basename, so their URL
// must be a withBase(...) call, or a variable this file assigns from one
// (the URL is often built on the line above, which the line check misses).
it("writes History API URLs through withBase", () => {
  const offences = [];
  const call = /history\.(pushState|replaceState)\(([^;]*)\)/g;
  for (const file of walk(SRC)) {
    if (SKIP.has(path.basename(file))) continue;
    const text = fs.readFileSync(file, "utf8");
    for (const m of text.matchAll(call)) {
      const args = m[2];
      if (args.includes("withBase(")) continue;
      const url = args.split(",").slice(2).join(",").trim();
      const assigned =
        /^[A-Za-z_$][\w$]*$/.test(url) &&
        new RegExp(`\\b(?:const|let|var)\\s+${url}\\s*=\\s*withBase\\(`).test(text);
      if (assigned) continue;
      const line = text.slice(0, m.index).split("\n").length;
      offences.push(`${path.relative(SRC, file)}:${line}: ${m[0]}`);
    }
  }
  expect(offences).toEqual([]);
});
