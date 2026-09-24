import React, { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import EmbedderFormFields from "./EmbedderFormFields";
import { MODE_LINKED, emptyDraft } from "./embedderModel";

const llms = [
  { id: "1", attributes: { name: "Prod OpenAI", vendor: "openai" } },
  { id: "2", attributes: { name: "Claude", vendor: "anthropic" } },
];
const vendors = ["google_ai", "huggingface", "ollama", "openai", "vertex"];

// Controlled harness: the fields are driven by the caller's state.
let lastDraft;
const Harness = ({ initial = emptyDraft(), lockedReason = "" }) => {
  const [draft, setDraft] = useState(initial);
  lastDraft = draft;
  return (
    <ThemeProvider theme={testTheme}>
      <EmbedderFormFields draft={draft} onChange={setDraft} llms={llms} vendors={vendors} lockedReason={lockedReason} />
    </ThemeProvider>
  );
};

const pick = (label, option) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: label }));
  fireEvent.click(within(screen.getByRole("listbox")).getByText(option));
};

describe("EmbedderFormFields", () => {
  it("fills vendor defaults and clears the other mode's fields on switch", () => {
    render(<Harness />);
    pick("API compatibility", "OpenAI");
    expect(lastDraft.vendor).toBe("openai");
    expect(lastDraft.model).toBe("text-embedding-3-small");
    expect(lastDraft.endpoint).toBe("https://api.openai.com/v1");

    fireEvent.change(screen.getByLabelText(/API key/), { target: { value: "sk-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Use an LLM provider" }));
    expect(lastDraft).toMatchObject({ mode: MODE_LINKED, vendor: "", endpoint: "", api_key: "" });
  });

  it("offers only LLM providers whose vendor embeds", () => {
    render(<Harness initial={{ ...emptyDraft(), mode: MODE_LINKED }} />);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "LLM provider" }));
    const list = within(screen.getByRole("listbox"));
    expect(list.getByText(/Prod OpenAI/)).toBeInTheDocument();
    expect(list.queryByText(/Claude/)).not.toBeInTheDocument();
  });

  it("labels the Vertex endpoint as project and location", () => {
    render(<Harness initial={{ ...emptyDraft(), vendor: "vertex" }} />);
    expect(screen.getByLabelText("Project and location")).toBeInTheDocument();
  });

  it("locks the model and compatibility while data sources use the embedder", () => {
    render(
      <Harness
        initial={{ ...emptyDraft(), name: "e", vendor: "openai", model: "m" }}
        lockedReason="Used by Handbook."
      />,
    );
    expect(screen.getByText("Used by Handbook.")).toBeInTheDocument();
    expect(screen.getByTestId("embedder-model")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Use an LLM provider" })).toBeDisabled();
    expect(screen.getByLabelText(/API key/)).not.toBeDisabled();
  });
});
