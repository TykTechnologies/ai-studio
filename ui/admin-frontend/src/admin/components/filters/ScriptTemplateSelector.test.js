import React from "react";
import fs from "fs";
import path from "path";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import ScriptTemplateSelector from "./ScriptTemplateSelector";

// The Go test that proves the template in every request format
// (TestScanToolResultsTemplate) holds the script; the UI copy must not drift.
const goTemplate = () => {
  const src = fs.readFileSync(
    path.resolve(__dirname, "../../../../../../scripting/script_segments_ent_test.go"),
    "utf8"
  );
  const m = src.match(/const ScanToolResultsTemplate = `([\s\S]*?)`\n/);
  return m && m[1];
};

const pick = async (filterType, name) => {
  const onTemplateSelect = jest.fn();
  render(
    <ThemeProvider theme={testTheme}>
      <ScriptTemplateSelector onTemplateSelect={onTemplateSelect} currentScript="" filterType={filterType} />
    </ThemeProvider>
  );
  fireEvent.mouseDown(screen.getByLabelText(/load template/i));
  fireEvent.click(await screen.findByText(name));
  return onTemplateSelect;
};

describe("ScriptTemplateSelector: scan tool results (4uc.11)", () => {
  it("loads the scan-tool-results template exactly as the Go test runs it", async () => {
    const expected = goTemplate();
    expect(expected).toBeTruthy();
    const onTemplateSelect = await pick("request", "Built-in library: scan tool results");
    expect(onTemplateSelect).toHaveBeenCalledWith(expected);
  });

  it("is a request template only", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <ScriptTemplateSelector onTemplateSelect={jest.fn()} currentScript="" filterType="response" />
      </ThemeProvider>
    );
    fireEvent.mouseDown(screen.getByLabelText(/load template/i));
    await screen.findAllByRole("option");
    expect(screen.queryByText("Built-in library: scan tool results")).not.toBeInTheDocument();
  });
});
