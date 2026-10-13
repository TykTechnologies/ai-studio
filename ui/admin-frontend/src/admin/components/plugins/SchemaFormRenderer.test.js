import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import SchemaFormRenderer, { fieldUiSchemas } from "./SchemaFormRenderer";

// midsommar-clean-yyo: a string field with a description over 100 characters
// was turned into a textarea, which overrode format "password" and showed
// plugin secrets (github-rag-ingest's secret_encryption_key) in clear text.

const LONG = "Key used to encrypt stored credentials. Generate a random 32-byte value and keep it out of version control at all times.";

const schema = {
  type: "object",
  properties: {
    secret_encryption_key: { type: "string", format: "password", title: "Secret encryption key", description: LONG },
    api_token: { type: "string", writeOnly: true, title: "API token", description: LONG },
    notes: { type: "string", title: "Notes", description: LONG },
    vault: {
      type: "object",
      title: "Vault",
      properties: {
        token: { type: "string", format: "password", title: "Vault token", description: LONG },
      },
    },
  },
};

const formData = {
  secret_encryption_key: "s3cr3t-key",
  api_token: "tok-123",
  notes: "plain text",
  vault: { token: "hvs.abc" },
};

describe("SchemaFormRenderer secret fields", () => {
  it("keeps long-description secret fields masked, top level and nested", () => {
    const { container } = render(
      <SchemaFormRenderer schema={schema} formData={formData} onChange={() => {}} />
    );
    for (const value of ["s3cr3t-key", "tok-123", "hvs.abc"]) {
      const input = screen.getByDisplayValue(value);
      expect(input).toHaveAttribute("type", "password");
    }
    // An ordinary long-description field is still a textarea.
    expect(screen.getByDisplayValue("plain text").tagName).toBe("TEXTAREA");
    // No secret is rendered anywhere as visible text.
    expect(container).not.toHaveTextContent("s3cr3t-key");
  });

  it("builds a password widget for secrets and leaves the textarea rule for the rest", () => {
    const ui = fieldUiSchemas(schema);
    expect(ui.secret_encryption_key).toEqual({ "ui:widget": "password" });
    expect(ui.api_token).toEqual({ "ui:widget": "password" });
    expect(ui.notes["ui:widget"]).toBe("textarea");
    expect(ui.vault.token).toEqual({ "ui:widget": "password" });
  });
});
