/**
 * Field model behind the client-tool form builder, and its round trip to
 * JSON Schema. Kept free of React so it can be unit tested directly.
 *
 * A field is { name, label, type, required, description, options } where
 * `type` is one of FIELD_TYPES. `fieldsToSchema` produces the object schema
 * the model (parameters) or the person in the chat (response) fills;
 * `schemaToFields` reads a schema back, reporting whether anything in it
 * could not be represented (nested objects, arrays, oneOf, ...) so the
 * editor can fall back to raw JSON without losing information.
 */

export const FIELD_TYPES = [
  { value: "text", label: "Short text" },
  { value: "textarea", label: "Long text" },
  { value: "number", label: "Number" },
  { value: "boolean", label: "Yes / no" },
  { value: "date", label: "Date" },
  { value: "email", label: "Email" },
  { value: "phone", label: "Phone" },
  { value: "url", label: "Web address" },
  { value: "select", label: "Choice (dropdown)" },
];

const FORMAT_BY_TYPE = { date: "date", email: "email", url: "uri" };
const TYPE_BY_FORMAT = { date: "date", email: "email", uri: "url" };

let counter = 0;
/** A blank field with a stable key for React lists. */
export const newField = (overrides = {}) => ({
  key: `f${Date.now().toString(36)}${(counter += 1)}`,
  name: "",
  label: "",
  type: "text",
  required: false,
  description: "",
  options: [],
  ...overrides,
});

/** snake_case identifier from a label, e.g. "Postal code" -> "postal_code". */
export const nameFromLabel = (label) =>
  String(label || "")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 64);

const labelFromName = (name) =>
  String(name || "")
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());

/** Fields -> JSON Schema object. Fields without a name are skipped. */
export const fieldsToSchema = (fields) => {
  const properties = {};
  const required = [];
  (fields || []).forEach((field) => {
    const name = (field.name || nameFromLabel(field.label)).trim();
    if (!name) return;
    const prop = {};
    switch (field.type) {
      case "number":
        prop.type = "number";
        break;
      case "boolean":
        prop.type = "boolean";
        break;
      case "select":
        prop.type = "string";
        prop.enum = (field.options || []).map((o) => String(o).trim()).filter(Boolean);
        break;
      case "phone":
        // Not a JSON Schema format (ajv would warn); an input-type hint the
        // chat form and the builder both understand.
        prop.type = "string";
        prop["x-input-type"] = "tel";
        break;
      case "textarea":
        prop.type = "string";
        prop["x-multiline"] = true;
        break;
      default:
        prop.type = "string";
        if (FORMAT_BY_TYPE[field.type]) prop.format = FORMAT_BY_TYPE[field.type];
    }
    if (field.label && field.label.trim()) prop.title = field.label.trim();
    if (field.description && field.description.trim()) prop.description = field.description.trim();
    properties[name] = prop;
    if (field.required) required.push(name);
  });
  const schema = { type: "object", properties };
  if (required.length) schema.required = required;
  return schema;
};

const isPlainObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);

/**
 * JSON Schema object -> fields. Returns { fields, unsupported } where
 * `unsupported` lists property names the builder cannot express; when it is
 * non-empty the caller should keep the raw JSON editor.
 */
export const schemaToFields = (schema) => {
  if (!isPlainObject(schema)) return { fields: [], unsupported: [] };
  const properties = isPlainObject(schema.properties) ? schema.properties : {};
  const required = new Set(Array.isArray(schema.required) ? schema.required : []);
  const fields = [];
  const unsupported = [];
  Object.entries(properties).forEach(([name, prop]) => {
    if (!isPlainObject(prop)) {
      unsupported.push(name);
      return;
    }
    const base = {
      name,
      label: typeof prop.title === "string" ? prop.title : labelFromName(name),
      required: required.has(name),
      description: typeof prop.description === "string" ? prop.description : "",
    };
    const type = Array.isArray(prop.type) ? prop.type.find((t) => t !== "null") : prop.type;
    if (Array.isArray(prop.enum) && prop.enum.every((v) => typeof v === "string")) {
      fields.push(newField({ ...base, type: "select", options: [...prop.enum] }));
    } else if (type === "string" || type === undefined) {
      if (type === undefined && Object.keys(prop).some((k) => ["properties", "items", "oneOf", "anyOf", "allOf", "$ref"].includes(k))) {
        unsupported.push(name);
        return;
      }
      const t = TYPE_BY_FORMAT[prop.format] || (prop["x-input-type"] === "tel" ? "phone" : prop["x-multiline"] ? "textarea" : "text");
      fields.push(newField({ ...base, type: t }));
    } else if (type === "number" || type === "integer") {
      fields.push(newField({ ...base, type: "number" }));
    } else if (type === "boolean") {
      fields.push(newField({ ...base, type: "boolean" }));
    } else {
      unsupported.push(name);
    }
  });
  // Anything beyond a flat object (composition keywords) is out of reach.
  ["oneOf", "anyOf", "allOf", "$ref", "patternProperties", "if"].forEach((k) => {
    if (k in schema) unsupported.push(k);
  });
  return { fields, unsupported };
};

/** Example arguments the model might supply, used by the preview. */
export const sampleValues = (fields) => {
  const out = {};
  (fields || []).forEach((field) => {
    const name = (field.name || nameFromLabel(field.label)).trim();
    if (!name) return;
    switch (field.type) {
      case "number":
        out[name] = 42;
        break;
      case "boolean":
        out[name] = true;
        break;
      case "date":
        out[name] = "2026-01-15";
        break;
      case "email":
        out[name] = "ada@example.com";
        break;
      case "phone":
        out[name] = "+1 555 0100";
        break;
      case "url":
        out[name] = "https://example.com";
        break;
      case "select":
        out[name] = (field.options || [])[0] || "";
        break;
      default:
        out[name] = field.description ? `Example ${field.label || name}` : `Example ${field.label || name}`;
    }
  });
  return out;
};

/** Parses a schema string; empty -> null; invalid -> { error }. */
export const parseSchemaText = (text) => {
  if (!text || !text.trim()) return { schema: null, error: null };
  try {
    const value = JSON.parse(text);
    if (!isPlainObject(value)) return { schema: null, error: "must be a JSON object" };
    return { schema: value, error: null };
  } catch (e) {
    return { schema: null, error: e.message };
  }
};

export default fieldsToSchema;
