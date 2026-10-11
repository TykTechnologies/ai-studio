import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import LLMFailoverSection from "./LLMFailoverSection";

const llms = [
  { id: 1, name: "Claude", vendor: "anthropic", active: true, allowed_models: [], default_model: "claude" },
  { id: 2, name: "Jev", vendor: "systemone", active: true, allowed_models: [], default_model: "jev-latest" },
  { id: 3, name: "Luna", vendor: "openai", active: true, allowed_models: [], default_model: "gpt-6-luna" },
];

const renderSection = (props) =>
  render(
    <ThemeProvider theme={testTheme}>
      <LLMFailoverSection value={{ targets: [] }} onChange={() => {}} llms={llms} {...props} />
    </ThemeProvider>
  );

describe("LLMFailoverSection with decision models", () => {
  it("offers no fallback when the only other LLMs serve another request kind", () => {
    renderSection({
      currentId: 1,
      vendor: "anthropic",
      llms: llms.filter((l) => l.vendor !== "openai"),
    });
    expect(screen.getByText("No other LLMs to fall back to")).toBeInTheDocument();
  });

  it("offers a decision-capable fallback to a System One primary", () => {
    renderSection({ currentId: 2, vendor: "systemone", llms: llms.filter((l) => l.vendor !== "anthropic") });
    expect(screen.getByText("Add fallback")).toBeEnabled();
  });

  it("filters nothing before a vendor is chosen", () => {
    renderSection({ currentId: undefined, vendor: undefined });
    expect(screen.getByText("Add fallback")).toBeEnabled();
  });
});
