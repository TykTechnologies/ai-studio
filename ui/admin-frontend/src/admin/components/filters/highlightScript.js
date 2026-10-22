import Prism from "prismjs/components/prism-core";

// highlightScript returns the script editor's HTML: the same markup as
// Prism.highlight(code, Prism.languages.js, "javascript") (Prism 1.30, no
// plugins), built from Prism's tokens only.
//
// The chat Markdown views use react-syntax-highlighter's Prism build, which
// loads refractor. refractor shares the prismjs module and changes it:
// - it replaces Prism.Token.stringify and Prism.util.encode, so
//   Prism.highlight returns hast objects and the editor crashed ("Objects
//   are not valid as a React child");
// - it removes the global Prism after loading, and Prism's language files
//   need it, so importing them after a chat page threw "Prism is not
//   defined".
// So the global is put back before the language files load, and the
// tokens are turned into HTML here. tokenize and the grammars are left
// alone, so the editor keeps its prism-tomorrow look whatever has loaded.
if (!window.Prism) {
  window.Prism = Prism;
}
require("prismjs/components/prism-clike");
require("prismjs/components/prism-javascript");

const encode = (text) => text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/ /g, " ");

const stringify = (token) => {
  if (typeof token === "string") {
    return encode(token);
  }
  if (Array.isArray(token)) {
    return token.map(stringify).join("");
  }
  const classes = ["token", token.type];
  if (token.alias) {
    classes.push(...(Array.isArray(token.alias) ? token.alias : [token.alias]));
  }
  return `<span class="${classes.join(" ")}">${stringify(token.content)}</span>`;
};

const highlightScript = (code) => stringify(Prism.tokenize(code, Prism.languages.javascript));

export default highlightScript;
