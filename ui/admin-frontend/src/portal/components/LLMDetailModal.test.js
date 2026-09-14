import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../admin/utils/testTheme";
import LLMDetailModal from "./LLMDetailModal";

let mockConfig = {};
jest.mock("../../config", () => ({
  getConfig: () => mockConfig,
}));

const llm = {
  id: "7",
  type: "llms",
  attributes: {
    name: "Acme OpenAI",
    vendor: "openai",
    short_description: "Fast general model",
    long_description: "Longer words about the model.",
    default_model: "gpt-4o",
    allowed_models: ["gpt-4o", "gpt-4o-*"],
    privacy_score: 40,
  },
};

const renderModal = (props = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <LLMDetailModal llm={llm} open handleClose={() => {}} {...props} />
    </ThemeProvider>
  );

// The modal showed the name and vendor only (UX review F-22 / Q16); a
// developer had to build an App to find out what the LLM serves.
describe("LLMDetailModal", () => {
  beforeEach(() => {
    mockConfig = { proxyURL: "http://gw.example.com/" };
  });

  it("shows descriptions, models, privacy level and the vendor", () => {
    renderModal();

    expect(screen.getByRole("heading", { name: "Acme OpenAI" })).toBeInTheDocument();
    expect(screen.getByText("Fast general model")).toBeInTheDocument();
    expect(screen.getByText("Longer words about the model.")).toBeInTheDocument();
    expect(screen.getByText("Default model:")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o", { selector: "p" })).toBeInTheDocument();
    expect(screen.getByText("gpt-4o-*")).toBeInTheDocument();
    // Privacy reads as the named level plus the score, same chip as the admin.
    expect(screen.getByTestId("privacy-level-chip")).toHaveTextContent("Internal · 40");
    expect(screen.getByText("Vendor:")).toBeInTheDocument();
  });

  // The base URL is <proxyURL>/ai/<slug>/v1, with the slug derived from the
  // name exactly as the gateway does and a trailing slash on proxyURL folded.
  it("shows the OpenAI-compatible base URL for the LLM slug", () => {
    renderModal();

    expect(
      screen.getByText("http://gw.example.com/ai/acme-openai/v1")
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy base URL" })).toBeInTheDocument();
  });

  it("falls back to the current host on port 9090 without a proxyURL", () => {
    mockConfig = {};

    renderModal();

    expect(screen.getByText(/\/\/localhost:9090\/ai\/acme-openai\/v1$/)).toBeInTheDocument();
  });

  it("says when no model restriction or privacy level is configured", () => {
    renderModal({
      llm: {
        ...llm,
        attributes: { ...llm.attributes, allowed_models: [], privacy_score: undefined, default_model: "" },
      },
    });

    expect(screen.getByText(/no restriction configured/)).toBeInTheDocument();
    expect(screen.getAllByText("Not set")).toHaveLength(2);
  });

  it("offers Build App for the LLM", () => {
    const onBuildApp = jest.fn();
    renderModal({ onBuildApp });

    fireEvent.click(screen.getByRole("button", { name: "Build App" }));

    expect(onBuildApp).toHaveBeenCalledWith("7");
  });
});
