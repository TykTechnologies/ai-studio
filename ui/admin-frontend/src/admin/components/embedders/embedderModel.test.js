import {
  MODE_LINKED,
  MODE_STANDALONE,
  attributesFromDraft,
  describeEmbedder,
  draftFromAttributes,
  emptyDraft,
  endpointField,
  validateDraft,
} from "./embedderModel";

describe("embedderModel", () => {
  it("sends only the fields of the chosen mode", () => {
    const standalone = {
      ...emptyDraft(),
      name: " OpenAI small ",
      vendor: "openai",
      endpoint: " https://api.openai.com/v1 ",
      api_key: "[redacted]",
      model: "text-embedding-3-small",
      privacy_score: 40,
    };
    expect(attributesFromDraft(standalone)).toEqual({
      name: "OpenAI small",
      description: "",
      model: "text-embedding-3-small",
      llm_id: null,
      vendor: "openai",
      endpoint: "https://api.openai.com/v1",
      api_key: "[redacted]",
      privacy_score: 40,
    });

    const linked = { ...emptyDraft(), mode: MODE_LINKED, name: "Via LLM", llm_id: "7", model: "m", vendor: "openai" };
    expect(attributesFromDraft(linked)).toEqual({ name: "Via LLM", description: "", model: "m", llm_id: 7 });
  });

  it("reads an API row into the right mode", () => {
    expect(draftFromAttributes({ name: "x", linked: true, llm_id: 3, vendor: "openai", model: "m", privacy_score: 80 }))
      .toMatchObject({ mode: MODE_LINKED, llm_id: "3", vendor: "", api_key: "" });
    expect(draftFromAttributes({ name: "y", linked: false, vendor: "ollama", endpoint: "http://o", api_key: "[redacted]", model: "n" }))
      .toMatchObject({ mode: MODE_STANDALONE, vendor: "ollama", api_key: "[redacted]" });
  });

  it("validates per mode", () => {
    expect(validateDraft(emptyDraft())).toEqual({
      name: "Name is required",
      model: "Model is required",
      vendor: "Pick the API compatibility",
    });
    expect(validateDraft({ ...emptyDraft(), mode: MODE_LINKED, name: "a", model: "m" })).toEqual({
      llm_id: "Pick the LLM provider to link to",
    });
    expect(validateDraft({ ...emptyDraft(), name: "a", model: "m", vendor: "openai", privacy_score: 101 }))
      .toHaveProperty("privacy_score");
  });

  it("labels the Vertex endpoint as project:location", () => {
    expect(endpointField("vertex").label).toBe("Project and location");
    expect(endpointField("openai").label).toBe("Endpoint URL");
  });

  it("describes where an embedder sends text", () => {
    expect(describeEmbedder({ model: "m", linked: true, llm_name: "Prod OpenAI" })).toBe("m · Prod OpenAI");
    expect(describeEmbedder({ model: "m", vendor: "ollama" })).toBe("m · ollama");
  });
});
