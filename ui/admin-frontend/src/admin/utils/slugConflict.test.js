import { slugConflictDetail } from "./slugConflict";

const err = (status, detail) => ({ response: { status, data: { errors: [{ detail }] } } });
const TOOL = 'tool name "Page Fetcher" is too close to tool 2 ("page-fetcher"): both have the slug "page-fetcher", and tool names must differ by more than case, spaces or punctuation';
const LLM = 'LLM name "Open AI" is too close to LLM 2 ("OpenAI"): both have the slug "open-ai", and LLM names must differ by more than case, spaces or punctuation';

describe("slugConflictDetail", () => {
  it("recognises the tool (409) and LLM (400) slug conflicts", () => {
    expect(slugConflictDetail(err(409, TOOL))).toBe(TOOL);
    expect(slugConflictDetail(err(400, LLM))).toBe(LLM);
  });

  it("ignores other errors", () => {
    expect(slugConflictDetail(err(400, "failover target 2: model not allowed"))).toBeNull();
    expect(slugConflictDetail(err(500, TOOL))).toBeNull();
    expect(slugConflictDetail({ message: "Network Error" })).toBeNull();
    expect(slugConflictDetail(undefined)).toBeNull();
  });
});
