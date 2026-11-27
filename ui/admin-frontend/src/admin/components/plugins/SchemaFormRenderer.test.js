import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import SchemaFormRenderer, { fieldUiSchemas, hasRedactedValue, REDACTED_VALUE, withPatternProperties } from "./SchemaFormRenderer";

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

// midsommar-clean-6bg5.9: the API returns a stored secret as "[redacted]",
// never the value. The form keeps the field masked and says what saving does.
describe("SchemaFormRenderer with secrets redacted by the API", () => {
  const redactedSchema = {
    type: "object",
    properties: {
      redis_addr: { type: "string", title: "Redis address" },
      redis_password: { type: "string", format: "password", title: "Redis password" },
      backend: {
        type: "object",
        properties: { redis: { type: "object", properties: { password: { type: "string", format: "password", title: "Nested password" } } } },
      },
    },
  };

  it("finds a redacted value at any depth", () => {
    expect(hasRedactedValue({ a: "x", b: { c: [{ d: REDACTED_VALUE }] } })).toBe(true);
    expect(hasRedactedValue({ a: "x", b: { c: ["y"] }, n: 1, z: null })).toBe(false);
    expect(hasRedactedValue(undefined)).toBe(false);
  });

  it("explains that an untouched secret is kept, and shows no note without one", () => {
    const { unmount } = render(
      <SchemaFormRenderer
        schema={redactedSchema}
        formData={{ redis_addr: "redis:6379", redis_password: REDACTED_VALUE, backend: { redis: { password: REDACTED_VALUE } } }}
        onChange={() => {}}
      />
    );
    expect(screen.getByTestId("plugin-config-redacted-note")).toHaveTextContent(/keeps its stored value/);
    for (const input of screen.getAllByDisplayValue(REDACTED_VALUE)) {
      expect(input).toHaveAttribute("type", "password");
    }
    expect(screen.getAllByDisplayValue(REDACTED_VALUE)).toHaveLength(2);
    unmount();

    render(<SchemaFormRenderer schema={redactedSchema} formData={{ redis_addr: "redis:6379" }} onChange={() => {}} />);
    expect(screen.queryByTestId("plugin-config-redacted-note")).not.toBeInTheDocument();
  });
});

// RJSF does not render patternProperties, so settings a plugin keeps at the
// top level under user-chosen names (Prompt Compressor's engine_<name>_url
// and engine_<name>_token) had no field in the form.
describe("SchemaFormRenderer with patternProperties", () => {
  const engineSchema = {
    type: "object",
    additionalProperties: false,
    properties: {
      mode: { type: "string", title: "Mode" },
    },
    patternProperties: {
      "^engine_[a-z0-9]+_url$": { type: "string", description: "Engine URL" },
      "^engine_[a-z0-9]+_token$": { type: "string", writeOnly: true, description: "Engine token" },
    },
  };

  it("adds configured keys that match a pattern as properties with the pattern's schema", () => {
    const out = withPatternProperties(engineSchema, {
      mode: "enforce",
      engine_lingua_url: "https://lingua.example.com",
      engine_lingua_token: "$SECRET/LINGUA_TOKEN",
      unrelated: "x",
    });
    expect(out.properties.engine_lingua_url).toEqual({
      title: "engine_lingua_url",
      type: "string",
      description: "Engine URL",
    });
    expect(out.properties.engine_lingua_token.writeOnly).toBe(true);
    expect(out.properties.unrelated).toBeUndefined();
    // The declared properties are kept.
    expect(out.properties.mode).toEqual(engineSchema.properties.mode);
  });

  it("lets a closed schema add keys whose names match a pattern", () => {
    const out = withPatternProperties(engineSchema, { mode: "audit" });
    expect(out.additionalProperties).toEqual({
      anyOf: [engineSchema.patternProperties["^engine_[a-z0-9]+_url$"], engineSchema.patternProperties["^engine_[a-z0-9]+_token$"]],
    });
    expect(out.propertyNames).toEqual({
      anyOf: [
        { enum: ["mode"] },
        { pattern: "^engine_[a-z0-9]+_url$" },
        { pattern: "^engine_[a-z0-9]+_token$" },
      ],
    });
  });

  it("uses the pattern's schema directly when there is one pattern", () => {
    const one = { ...engineSchema, patternProperties: { "^x_[a-z]+$": { type: "string" } } };
    expect(withPatternProperties(one, {}).additionalProperties).toEqual({ type: "string" });
  });

  it("leaves an open schema's additionalProperties alone", () => {
    const open = { ...engineSchema, additionalProperties: undefined };
    const out = withPatternProperties(open, { engine_h_url: "http://h" });
    expect(out.properties.engine_h_url.type).toBe("string");
    expect(out.additionalProperties).toBeUndefined();
    expect(out.propertyNames).toBeUndefined();
  });

  it("returns the schema unchanged without usable patterns", () => {
    const plain = { type: "object", properties: { a: { type: "string" } } };
    expect(withPatternProperties(plain, { a: "1" })).toBe(plain);
    const broken = { type: "object", additionalProperties: false, patternProperties: { "([": { type: "string" } } };
    expect(withPatternProperties(broken, { "([": "1" })).toBe(broken);
  });

  it("renders the matching keys, masking the secret ones", () => {
    render(
      <SchemaFormRenderer
        schema={engineSchema}
        formData={{ mode: "enforce", engine_lingua_url: "https://lingua.example.com", engine_lingua_token: "tok-lingua" }}
        onChange={() => {}}
      />
    );
    expect(screen.getByDisplayValue("https://lingua.example.com")).toHaveAttribute("type", "text");
    expect(screen.getByDisplayValue("tok-lingua")).toHaveAttribute("type", "password");
  });
});
