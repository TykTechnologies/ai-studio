import { parseOpenAPIOperations } from "./openapiOperations";

const jsonSpec = JSON.stringify({
  openapi: "3.0.0",
  paths: {
    "/pets": {
      get: { operationId: "listPets", summary: "List pets" },
      post: { operationId: "createPet" },
    },
    "/pets/{id}": {
      get: { summary: "No operationId here" },
      delete: { operationId: "deletePet", description: "Remove a pet" },
    },
  },
});

const yamlSpec = `
openapi: 3.0.0
paths:
  /weather:
    get:
      operationId: getWeather
      summary: Current weather
`;

describe("parseOpenAPIOperations", () => {
  it("lists operations with an operationId from a JSON spec", () => {
    expect(parseOpenAPIOperations(jsonSpec)).toEqual([
      { operationId: "listPets", method: "GET", path: "/pets", summary: "List pets" },
      { operationId: "createPet", method: "POST", path: "/pets", summary: "" },
      { operationId: "deletePet", method: "DELETE", path: "/pets/{id}", summary: "Remove a pet" },
    ]);
  });

  it("parses YAML specs", () => {
    expect(parseOpenAPIOperations(yamlSpec)).toEqual([
      { operationId: "getWeather", method: "GET", path: "/weather", summary: "Current weather" },
    ]);
  });

  it("returns null for empty, unparsable, or path-less input", () => {
    expect(parseOpenAPIOperations("")).toBeNull();
    expect(parseOpenAPIOperations("   ")).toBeNull();
    expect(parseOpenAPIOperations("{ not: valid: json: [")).toBeNull();
    expect(parseOpenAPIOperations('{"openapi":"3.0.0"}')).toBeNull();
  });

  it("returns an empty list when no operation has an operationId", () => {
    expect(parseOpenAPIOperations('{"paths":{"/x":{"get":{"summary":"x"}}}}')).toEqual([]);
  });
});
