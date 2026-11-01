import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../utils/testTheme";
import PromptTemplateEditor from "./PromptTemplateEditor";

jest.mock("../../../utils/apiClient", () => ({
  __esModule: true,
  default: { get: jest.fn(), post: jest.fn(), patch: jest.fn(), delete: jest.fn() },
}));

const apiClient = require("../../../utils/apiClient").default;

const intro = "You are reviewing a pull request. Follow these rules.\n<diff>\n";
const outro = "\n</diff>\nReport only real defects, one per line.\n";

const renderEditor = (props = {}) => {
  const onSaved = jest.fn();
  render(
    <ThemeProvider theme={testTheme}>
      <PromptTemplateEditor open appId="7" template={null} onClose={jest.fn()} onSaved={onSaved} {...props} />
    </ThemeProvider>
  );
  return { onSaved };
};

const fill = (name, value) => fireEvent.change(screen.getByLabelText(name), { target: { value } });

describe("PromptTemplateEditor", () => {
  beforeEach(() => jest.clearAllMocks());

  it("proposes a template made of constants and slots", async () => {
    apiClient.post.mockResolvedValue({ data: { data: { id: 1, ref: "pr-review@1", status: "proposed" } } });
    const { onSaved } = renderEditor();
    fill("Name", "pr-review");
    fill("Constant 1", intro);
    fill("Slot 1 name", "diff");
    fill("Constant 2", outro);
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    await waitFor(() =>
      expect(apiClient.post).toHaveBeenCalledWith("/apps/7/prompt-templates", {
        name: "pr-review",
        placement: { role: "user", message: "any", anchor: "both" },
        parts: [{ const: intro }, { slot: "diff" }, { const: outro }],
        expires_after_days: 30,
      })
    );
    expect(onSaved).toHaveBeenCalled();
  });

  it("refuses a short constant and an invisible character before calling the API", async () => {
    renderEditor();
    fill("Name", "pr-review");
    fill("Constant 1", "Too short.");
    fill("Slot 1 name", "diff");
    fill("Constant 2", outro.replace("real", "re\u200bal"));
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    expect(await screen.findByText(/at least 32 bytes \(this one is 10\)/)).toBeInTheDocument();
    expect(screen.getByText(/invisible characters \(ZWSP U\+200B/)).toBeInTheDocument();
    expect(apiClient.post).not.toHaveBeenCalled();
  });

  it("shows the API's error", async () => {
    apiClient.post.mockRejectedValue({
      response: { data: { errors: [{ title: "Bad Request", detail: "invalid prompt template: something the UI missed" }] } },
    });
    renderEditor();
    fill("Name", "pr-review");
    fill("Constant 1", intro);
    fill("Slot 1 name", "diff");
    fill("Constant 2", outro);
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    expect(await screen.findByTestId("api-error")).toHaveTextContent("invalid prompt template: something the UI missed");
  });

  it("says a change to an approved version makes a new one", async () => {
    apiClient.patch.mockResolvedValue({ data: { data: { id: 2, ref: "pr-review@2", status: "proposed" } } });
    const template = {
      id: 1,
      name: "pr-review",
      version: 1,
      status: "approved",
      placement: { role: "user", message: "any", anchor: "both" },
      parts: [{ const: intro }, { slot: "diff" }, { const: outro }],
      expires_after_days: 30,
    };
    renderEditor({ template });
    expect(screen.getByText(/creates a new version, proposed/)).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(apiClient.patch).toHaveBeenCalledWith("/apps/7/prompt-templates/1", expect.any(Object)));
  });
});
