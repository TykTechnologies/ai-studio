import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import VisibleText from "./VisibleText";

const markers = () => screen.getAllByTestId("char-marker").map((m) => [m.dataset.kind, m.textContent]);

describe("VisibleText", () => {
  it("replaces each invisible character with a labelled marker and never renders it", () => {
    const text =
      "Follow\u200b these\u202e rules\u{e0041} only\ufe0f.\u00a0Report\u00ad defects\u2003now\u0007.";
    const { container } = render(<VisibleText text={text} label="Constant 1" />);
    expect(markers()).toEqual([
      ["zero-width", "zero-width ZWSP U+200B"],
      ["bidi", "bidi control RLO U+202E"],
      ["tag", "Unicode tag U+E0041"],
      ["variation-selector", "variation selector U+FE0F"],
      ["nbsp", "no-break space NBSP U+00A0"],
      ["non-printing", "non-printing SHY U+00AD"],
      ["space", "unusual space U+2003"],
      ["control", "control character U+0007"],
    ]);
    // None of the hidden characters reaches the page.
    for (const ch of ["\u200b", "\u202e", "\u{e0041}", "\ufe0f", "\u00a0", "\u00ad", "\u2003", "\u0007"]) {
      expect(container.textContent).not.toContain(ch);
    }
    expect(screen.getByTestId("marker-summary")).toHaveTextContent(/Marked: 1 zero-width, 1 bidi control/);
  });

  it("shows a lookalike with its code point, and a combining mark by its code alone", () => {
    render(<VisibleText text={"p\u0430ssword re\u0301view"} />);
    expect(markers()).toEqual([
      ["lookalike", "\u0430 U+0430"],
      ["lookalike", "combining U+0301"],
    ]);
  });

  it("says when a constant is plain ASCII, and marks trailing spaces", () => {
    render(<VisibleText text={"Report only real defects.  \nOne per line."} />);
    expect(screen.queryByTestId("char-marker")).not.toBeInTheDocument();
    expect(screen.getByTestId("marker-summary")).toHaveTextContent("41 bytes, plain ASCII.");
    expect(screen.getByTestId("trailing-space")).toHaveTextContent("\u00b7\u00b7");
  });
});
