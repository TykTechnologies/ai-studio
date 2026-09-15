import { fieldsToSchema, schemaToFields, sampleValues, nameFromLabel, newField, parseSchemaText } from "./schemaFields";
import { CLIENT_TOOL_PRESETS, presetsForKind } from "./clientToolPresets";

describe("schemaFields", () => {
  it("round-trips every field type through JSON Schema", () => {
    const fields = [
      newField({ name: "full_name", label: "Full name", type: "text", required: true, description: "As on the parcel" }),
      newField({ name: "notes", label: "Notes", type: "textarea" }),
      newField({ name: "qty", label: "Quantity", type: "number", required: true }),
      newField({ name: "express", label: "Express", type: "boolean" }),
      newField({ name: "when", label: "When", type: "date" }),
      newField({ name: "email", label: "Email", type: "email" }),
      newField({ name: "phone", label: "Phone", type: "phone" }),
      newField({ name: "site", label: "Site", type: "url" }),
      newField({ name: "size", label: "Size", type: "select", options: ["S", "M", "L"] }),
    ];
    const schema = fieldsToSchema(fields);
    expect(schema.type).toBe("object");
    expect(schema.required).toEqual(["full_name", "qty"]);
    expect(schema.properties.full_name).toEqual({ type: "string", title: "Full name", description: "As on the parcel" });
    expect(schema.properties.notes["x-multiline"]).toBe(true);
    expect(schema.properties.qty.type).toBe("number");
    expect(schema.properties.express.type).toBe("boolean");
    expect(schema.properties.when.format).toBe("date");
    expect(schema.properties.email.format).toBe("email");
    expect(schema.properties.phone.format).toBe("tel");
    expect(schema.properties.site.format).toBe("uri");
    expect(schema.properties.size.enum).toEqual(["S", "M", "L"]);

    const back = schemaToFields(schema);
    expect(back.unsupported).toEqual([]);
    expect(back.fields.map((f) => [f.name, f.type, f.required])).toEqual(fields.map((f) => [f.name, f.type, f.required]));
    expect(back.fields[8].options).toEqual(["S", "M", "L"]);
  });

  it("derives a name from the label and skips blank fields", () => {
    expect(nameFromLabel("Postal code (ZIP)")).toBe("postal_code_zip");
    const schema = fieldsToSchema([newField({ label: "City" }), newField({})]);
    expect(Object.keys(schema.properties)).toEqual(["city"]);
  });

  it("reports what the builder cannot represent", () => {
    const { fields, unsupported } = schemaToFields({
      type: "object",
      properties: { name: { type: "string" }, address: { type: "object", properties: {} }, tags: { type: "array" } },
      oneOf: [],
    });
    expect(fields.map((f) => f.name)).toEqual(["name"]);
    expect(unsupported).toEqual(["address", "tags", "oneOf"]);
  });

  it("produces sample values for the preview", () => {
    const values = sampleValues([
      newField({ name: "amount", type: "number" }),
      newField({ name: "currency", type: "select", options: ["USD"] }),
      newField({ name: "reason", label: "Reason", type: "text" }),
    ]);
    expect(values).toEqual({ amount: 42, currency: "USD", reason: "Example Reason" });
  });

  it("parses schema text", () => {
    expect(parseSchemaText("")).toEqual({ schema: null, error: null });
    expect(parseSchemaText("[1]").error).toMatch(/object/);
    expect(parseSchemaText("{nope").error).toBeTruthy();
    expect(parseSchemaText('{"type":"object"}').schema).toEqual({ type: "object" });
  });
});

describe("clientToolPresets", () => {
  it("ships the three form examples and approval examples that all convert cleanly", () => {
    const forms = presetsForKind("form").map((p) => p.name);
    expect(forms).toEqual(["Shipping address (US)", "Shipping address (international)", "Contact information"]);
    expect(presetsForKind("approval").length).toBeGreaterThan(0);
    CLIENT_TOOL_PRESETS.forEach((preset) => {
      expect(preset.description.length).toBeGreaterThan(20);
      const params = fieldsToSchema(preset.parameters);
      expect(Object.keys(params.properties).length).toBeGreaterThan(0);
      expect(schemaToFields(params).unsupported).toEqual([]);
    });
    presetsForKind("form").forEach((preset) => {
      const response = fieldsToSchema(preset.response);
      expect(Object.keys(response.properties).length).toBeGreaterThan(2);
      expect(schemaToFields(response).unsupported).toEqual([]);
    });
  });
});
