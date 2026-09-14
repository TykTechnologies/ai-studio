import yaml from "js-yaml";

const HTTP_METHODS = ["get", "post", "put", "delete", "options", "head", "patch", "trace"];

/**
 * Parses an OpenAPI document (JSON or YAML) and lists its operations.
 *
 * Mirrors the backend: an operation is only usable as a tool when it carries
 * an `operationId`, so operations without one are skipped. Returns `null`
 * when the text cannot be parsed or has no `paths` block, so the caller can
 * fall back to free-text entry.
 *
 * @param {string} specText
 * @returns {Array<{operationId: string, method: string, path: string, summary: string}>|null}
 */
export const parseOpenAPIOperations = (specText) => {
  if (!specText || typeof specText !== "string" || !specText.trim()) {
    return null;
  }

  let doc;
  try {
    doc = JSON.parse(specText);
  } catch (jsonError) {
    try {
      doc = yaml.load(specText);
    } catch (yamlError) {
      return null;
    }
  }

  if (!doc || typeof doc !== "object" || !doc.paths || typeof doc.paths !== "object") {
    return null;
  }

  const operations = [];
  Object.entries(doc.paths).forEach(([path, pathItem]) => {
    if (!pathItem || typeof pathItem !== "object") return;
    HTTP_METHODS.forEach((method) => {
      const op = pathItem[method];
      if (op && typeof op === "object" && typeof op.operationId === "string" && op.operationId.trim()) {
        operations.push({
          operationId: op.operationId.trim(),
          method: method.toUpperCase(),
          path,
          summary: op.summary || op.description || "",
        });
      }
    });
  });

  return operations;
};

export default parseOpenAPIOperations;
