import { generateCurlExample } from "./ToolDocumentationPage";

jest.mock("../../config", () => ({
  getConfig: () => ({ toolDisplayURL: "http://localhost:9091" }),
}));

jest.mock("../../admin/utils/pubClient", () => ({ get: jest.fn(), post: jest.fn() }));

const toolDetails = { attributes: { name: "Currency Exchange Rates" } };

// The operation reported in #488: an OpenAPI path with two path parameters and
// one query parameter.
const getExchangeRate = {
  operation_id: "getExchangeRate",
  method: "get",
  path: "/rate/{base}/{quote}",
  parameters: [
    { name: "base", in: "path", required: true, schema: { type: "string" } },
    { name: "quote", in: "path", required: true, schema: { type: "string" } },
    { name: "date", in: "query", required: false, schema: { type: "string" } },
  ],
};

const commandLine = (curl, predicate) =>
  curl
    .split("\\\n")
    .map((line) => line.trim())
    .find(predicate);

describe("generateCurlExample", () => {
  it("targets the bare tool endpoint", () => {
    const curl = generateCurlExample(getExchangeRate, toolDetails);
    const url = commandLine(curl, (line) => line.startsWith("http"));

    expect(url).toBe("http://localhost:9091/tools/currency-exchange-rates");
  });

  it("does not append the OpenAPI path template or query string", () => {
    const curl = generateCurlExample(getExchangeRate, toolDetails);

    // The old output was .../tools/currency-exchange-rates/{base}/{quote}?date={date},
    // which matches no route and 404s when pasted.
    expect(curl).not.toContain("{base}");
    expect(curl).not.toContain("{quote}");
    expect(curl).not.toContain("?date=");
  });

  it("uses POST regardless of the operation's HTTP method", () => {
    // The operation is a GET, but the tool endpoint always takes a POST with a
    // JSON body naming the operation.
    expect(generateCurlExample(getExchangeRate, toolDetails)).toContain("curl -X POST");
  });

  it("selects the operation and parameters in the body", () => {
    const curl = generateCurlExample(getExchangeRate, toolDetails);
    const body = curl.slice(curl.indexOf("-d '") + 4, curl.lastIndexOf("'"));
    const parsed = JSON.parse(body);

    expect(parsed.operation_id).toBe("getExchangeRate");
    // Parameters are arrays of strings, matching map[string][]string upstream.
    expect(parsed.parameters).toEqual({
      base: ["example_base"],
      quote: ["example_quote"],
      date: ["example_date"],
    });
  });

  it("emits a body for a GET operation", () => {
    // Previously the body was only generated for POST/PUT/PATCH operations, so
    // a GET produced a request the endpoint rejects for having no operation_id.
    expect(generateCurlExample(getExchangeRate, toolDetails)).toContain('"operation_id"');
  });

  it("sends JSON and the bearer token", () => {
    const curl = generateCurlExample(getExchangeRate, toolDetails, "app-key-123");

    expect(curl).toContain('-H "Content-Type: application/json"');
    expect(curl).toContain('-H "Authorization: Bearer app-key-123"');
  });

  it("falls back to a placeholder when no token is selected", () => {
    expect(generateCurlExample(getExchangeRate, toolDetails)).toContain("Bearer YOUR_API_KEY");
  });

  it("includes an example payload for an operation with a request body", () => {
    const createAlert = {
      operation_id: "createAlert",
      method: "post",
      path: "/alerts",
      parameters: [],
      request_body: {
        content_type: "application/json",
        schema: { properties: { threshold: { type: "number" }, note: { type: "string" } } },
      },
    };

    const curl = generateCurlExample(createAlert, toolDetails);
    const body = curl.slice(curl.indexOf("-d '") + 4, curl.lastIndexOf("'"));
    const parsed = JSON.parse(body);

    expect(parsed.operation_id).toBe("createAlert");
    expect(parsed.payload).toEqual({ threshold: 42, note: "example_note" });
  });

  it("handles a missing operation or tool", () => {
    expect(generateCurlExample(null, toolDetails)).toBe("curl example not available");
    expect(generateCurlExample(getExchangeRate, null)).toBe("curl example not available");
  });
});
