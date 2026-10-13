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

  it("masks secrets inside array items, as strings and as object fields", () => {
    const arraySchema = {
      type: "object",
      properties: {
        tokens: { type: "array", title: "Tokens", items: { type: "string", format: "password" } },
        repos: {
          type: "array",
          title: "Repositories",
          items: {
            type: "object",
            properties: {
              name: { type: "string", title: "Name" },
              deploy_key: { type: "string", writeOnly: true, title: "Deploy key", description: LONG },
            },
          },
        },
      },
    };
    render(
      <SchemaFormRenderer
        schema={arraySchema}
        formData={{ tokens: ["tok-a"], repos: [{ name: "core", deploy_key: "key-b" }] }}
        onChange={() => {}}
      />
    );
    expect(screen.getByDisplayValue("tok-a")).toHaveAttribute("type", "password");
    expect(screen.getByDisplayValue("key-b")).toHaveAttribute("type", "password");
    expect(screen.getByDisplayValue("core")).toHaveAttribute("type", "text");

    const ui = fieldUiSchemas(arraySchema);
    expect(ui.tokens.items).toEqual({ "ui:widget": "password" });
    expect(ui.repos.items.deploy_key).toEqual({ "ui:widget": "password" });
  });

  it("masks secrets declared through local $ref definitions", () => {
    const refSchema = {
      type: "object",
      definitions: {
        backend: {
          type: "object",
          properties: {
            host: { type: "string", title: "Host" },
            password: { $ref: "#/definitions/secret" },
          },
        },
        secret: { type: "string", format: "password", title: "Password", description: LONG },
      },
      properties: {
        primary: { $ref: "#/definitions/backend", title: "Primary" },
        pool: { type: "array", title: "Pool", items: { $ref: "#/definitions/backend" } },
      },
    };
    render(
      <SchemaFormRenderer
        schema={refSchema}
        formData={{ primary: { host: "a", password: "pw-1" }, pool: [{ host: "b", password: "pw-2" }] }}
        onChange={() => {}}
      />
    );
    expect(screen.getByDisplayValue("pw-1")).toHaveAttribute("type", "password");
    expect(screen.getByDisplayValue("pw-2")).toHaveAttribute("type", "password");

    const ui = fieldUiSchemas(refSchema);
    expect(ui.primary.password).toEqual({ "ui:widget": "password" });
    expect(ui.pool.items.password).toEqual({ "ui:widget": "password" });
  });

  it("handles a self-referencing definition without recursing forever", () => {
    const tree = {
      type: "object",
      definitions: {
        node: {
          type: "object",
          properties: {
            token: { type: "string", format: "password", title: "Token" },
            child: { $ref: "#/definitions/node" },
          },
        },
      },
      properties: { root: { $ref: "#/definitions/node", title: "Root" } },
    };
    let ui;
    expect(() => {
      ui = fieldUiSchemas(tree);
    }).not.toThrow();
    expect(ui.root.token).toEqual({ "ui:widget": "password" });
    // The definition is walked once; the self-reference is not expanded again.
    expect(ui.root.child).toBeUndefined();
  });

  it("stays fast when every property of a definition refers back to it", () => {
    const fanout = 12;
    const properties = { secret: { type: "string", writeOnly: true, title: "Secret" } };
    for (let i = 0; i < fanout; i += 1) properties[`n${i}`] = { $ref: "#/definitions/node" };
    const wide = {
      type: "object",
      definitions: { node: { type: "object", properties } },
      properties: { root: { $ref: "#/definitions/node" }, list: { type: "array", items: { $ref: "#/definitions/node" } } },
    };
    const started = Date.now();
    const ui = fieldUiSchemas(wide);
    // Exponential before (fan-out 8 took ~7 s); linear now.
    expect(Date.now() - started).toBeLessThan(200);
    expect(ui.root.secret).toEqual({ "ui:widget": "password" });
    expect(ui.list.items.secret).toEqual({ "ui:widget": "password" });
  });

  it("stays fast on a chain of distinct definitions that each refer many times to the next", () => {
    // D0 -> D1 -> ... -> D7, each with 8 properties pointing at the next:
    // acyclic, so the path check does not help; the per-definition memo does.
    const definitions = {};
    for (let d = 0; d < 8; d += 1) {
      const properties = { key: { type: "string", writeOnly: true, title: "Key" } };
      if (d < 7) for (let i = 0; i < 8; i += 1) properties[`p${i}`] = { $ref: `#/definitions/D${d + 1}` };
      definitions[`D${d}`] = { type: "object", properties };
    }
    const dag = { type: "object", definitions, properties: { top: { $ref: "#/definitions/D0" } } };
    const started = Date.now();
    const ui = fieldUiSchemas(dag);
    expect(Date.now() - started).toBeLessThan(200);
    expect(ui.top.key).toEqual({ "ui:widget": "password" });
    expect(ui.top.p0.key).toEqual({ "ui:widget": "password" });
    expect(ui.top.p3.p5.key).toEqual({ "ui:widget": "password" });
  });

  it("gives up on a $ref cycle or an unresolvable ref instead of looping", () => {
    const odd = {
      type: "object",
      definitions: { loop: { $ref: "#/definitions/loop" } },
      properties: {
        weird: { $ref: "#/definitions/loop" },
        missing: { $ref: "#/definitions/nope" },
        remote: { $ref: "https://example.com/schema.json" },
      },
    };
    expect(fieldUiSchemas(odd)).toEqual({});
  });

  it("builds a password widget for secrets and leaves the textarea rule for the rest", () => {
    const ui = fieldUiSchemas(schema);
    expect(ui.secret_encryption_key).toEqual({ "ui:widget": "password" });
    expect(ui.api_token).toEqual({ "ui:widget": "password" });
    expect(ui.notes["ui:widget"]).toBe("textarea");
    expect(ui.vault.token).toEqual({ "ui:widget": "password" });
  });
});
