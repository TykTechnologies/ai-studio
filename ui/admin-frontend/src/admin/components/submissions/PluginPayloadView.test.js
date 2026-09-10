import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import PluginPayloadView from "./PluginPayloadView";

const schema = {
  type: "object",
  required: ["name"],
  properties: {
    name: { type: "string", title: "Agent Name" },
    system_prompt: {
      type: "string",
      title: "System Prompt",
      description: "Instructions the agent starts every conversation with",
    },
    enabled: { type: "boolean", title: "Enabled" },
    api_key: { type: "string", title: "API Key", format: "password" },
    tools: { type: "array", title: "Tools", items: { type: "string" } },
  },
};

describe("PluginPayloadView", () => {
  it("renders schema titles and descriptions, then extra keys", () => {
    render(
      <PluginPayloadView
        schema={schema}
        payload={{
          name: "Support Bot",
          system_prompt: "Be helpful.",
          enabled: true,
          api_key: "sk-secret",
          tools: ["search", "calendar"],
          custom_field: "extra value",
        }}
      />
    );

    expect(screen.getByText("Agent Name")).toBeInTheDocument();
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("System Prompt")).toBeInTheDocument();
    expect(
      screen.getByText("Instructions the agent starts every conversation with")
    ).toBeInTheDocument();
    expect(screen.getByText("Be helpful.")).toBeInTheDocument();

    // Booleans render as chips, secrets are masked, arrays as JSON.
    expect(screen.getByText("Yes")).toBeInTheDocument();
    expect(screen.getByText("***")).toBeInTheDocument();
    expect(screen.queryByText("sk-secret")).not.toBeInTheDocument();
    expect(screen.getByText(/"calendar"/)).toBeInTheDocument();

    // Keys outside the schema are still shown, labelled by their key.
    expect(screen.getByText("custom_field")).toBeInTheDocument();
    expect(screen.getByText("extra value")).toBeInTheDocument();
  });

  it("lists schema properties in schema order before extra keys", () => {
    render(
      <PluginPayloadView
        schema={schema}
        payload={{ zzz_extra: "last", enabled: false, name: "A" }}
      />
    );
    const labels = screen
      .getAllByText(/Agent Name|System Prompt|Enabled|API Key|Tools|zzz_extra/)
      .map((el) => el.textContent);
    expect(labels).toEqual([
      "Agent Name",
      "System Prompt",
      "Enabled",
      "API Key",
      "Tools",
      "zzz_extra",
    ]);
    // Absent schema fields render as a dash rather than vanishing.
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
    expect(screen.getByText("No")).toBeInTheDocument();
  });

  it("falls back to raw keys when there is no schema", () => {
    render(
      <PluginPayloadView payload={{ name: "Thing", model: "gpt-4o" }} />
    );
    expect(screen.getByText("name")).toBeInTheDocument();
    expect(screen.getByText("Thing")).toBeInTheDocument();
    expect(screen.getByText("model")).toBeInTheDocument();
    expect(screen.getByText("gpt-4o")).toBeInTheDocument();
  });

  it("says so when the payload is empty", () => {
    render(<PluginPayloadView payload={{}} />);
    expect(
      screen.getByText("This submission carries no resource fields.")
    ).toBeInTheDocument();
  });
});
