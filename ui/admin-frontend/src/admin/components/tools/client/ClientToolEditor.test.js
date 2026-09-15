import React, { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../../utils/testTheme";
import ClientToolEditor from "./ClientToolEditor";

const Harness = ({ initial, onPreset }) => {
  const [value, setValue] = useState(initial);
  const [tool, setTool] = useState({ name: "", description: "" });
  return (
    <ThemeProvider theme={testTheme}>
      <ClientToolEditor
        value={value}
        onChange={setValue}
        toolName={tool.name}
        toolDescription={tool.description}
        onApplyPreset={(p) => {
          setTool({ name: p.name, description: p.description });
          onPreset?.(p);
        }}
      />
      <pre data-testid="state">{JSON.stringify({ value, tool })}</pre>
    </ThemeProvider>
  );
};

const readState = () => JSON.parse(screen.getByTestId("state").textContent);

const blank = { kind: "approval", title: "", description: "", parameters: "", responseSchema: "" };

describe("ClientToolEditor", () => {
  it("builds the parameters schema from fields", () => {
    render(<Harness initial={blank} />);
    fireEvent.click(screen.getByTestId("client-parameters-add"));
    fireEvent.change(screen.getByLabelText("Detail 1 label"), { target: { value: "Amount to refund" } });
    fireEvent.click(screen.getByLabelText(/Required/));
    const { value } = readState();
    const schema = JSON.parse(value.parameters);
    expect(schema.properties.amount_to_refund).toEqual({ type: "string", title: "Amount to refund" });
    expect(schema.required).toEqual(["amount_to_refund"]);
  });

  it("switches to JSON for schemas the builder cannot show", () => {
    const nested = JSON.stringify({ type: "object", properties: { address: { type: "object", properties: {} } } });
    render(<Harness initial={{ ...blank, parameters: nested }} />);
    expect(screen.getByTestId("client-parameters-json")).toBeInTheDocument();
    expect(screen.getByText(/cannot show/)).toBeInTheDocument();
  });

  it("applies a preset to the definition and the tool", () => {
    const onPreset = jest.fn();
    render(<Harness initial={{ ...blank, kind: "form" }} onPreset={onPreset} />);
    fireEvent.click(screen.getByText("Contact information"));
    const { value, tool } = readState();
    expect(onPreset).toHaveBeenCalledTimes(1);
    expect(tool.name).toBe("Contact information");
    expect(value.title).toBe("How can we reach you?");
    const response = JSON.parse(value.responseSchema);
    expect(response.properties.email.format).toBe("email");
    expect(response.required).toEqual(["full_name", "email"]);
    // Preview shows the assistant-facing function name and the person's card.
    expect(screen.getByTestId("client-tool-preview")).toHaveTextContent("contact-information");
    expect(screen.getByTestId("client-tool-preview-card")).toHaveTextContent("How can we reach you?");
  });

  it("shows what the assistant receives when the preview is answered", () => {
    render(<Harness initial={{ ...blank, parameters: JSON.stringify({ type: "object", properties: { action: { type: "string" } } }) }} />);
    fireEvent.click(screen.getByRole("button", { name: "Approve" }));
    expect(screen.getByTestId("client-tool-preview-answer")).toHaveTextContent('"approved": true');
  });

  it("hides the builders for generative UI", () => {
    render(<Harness initial={{ ...blank, kind: "present" }} />);
    expect(screen.getByTestId("present-schema-note")).toBeInTheDocument();
    expect(screen.queryByTestId("client-parameters-builder")).not.toBeInTheDocument();
    expect(screen.queryByTestId("client-tool-preview")).not.toBeInTheDocument();
  });
});
