import {
  chatCapableLLMs,
  failoverCompatible,
  getChatVendorCodes,
  getVendorCapabilities,
  getVendorCodes,
  getVendorDefaultEndpoint,
  isDecisionOnlyVendor,
  vendorRequiresAccessDetails,
  vendorServesDecisions,
} from "./vendorLogos";

describe("vendor capabilities", () => {
  it("knows which vendors serve decision models", () => {
    expect(getVendorCapabilities("systemone")).toEqual(["decisions"]);
    expect(vendorServesDecisions("systemone")).toBe(true);
    expect(vendorServesDecisions("openai")).toBe(true);
    expect(vendorServesDecisions("anthropic")).toBe(false);

    expect(isDecisionOnlyVendor("systemone")).toBe(true);
    expect(isDecisionOnlyVendor("openai")).toBe(false);
    expect(isDecisionOnlyVendor("some-new-vendor")).toBe(false);
  });

  it("offers System One for LLMs but not for chat set-up", () => {
    expect(getVendorCodes()).toContain("systemone");
    expect(getChatVendorCodes()).not.toContain("systemone");
    expect(getChatVendorCodes()).toContain("openai");
    expect(getVendorDefaultEndpoint("systemone")).toBe("https://api.typesafe.ai");
    // Self-hosts (llama.cpp) may run without a key.
    expect(vendorRequiresAccessDetails("systemone")).toBe(false);
  });

  it("only pairs failover targets that serve a request kind in common", () => {
    expect(failoverCompatible("openai", "anthropic")).toBe(true);
    expect(failoverCompatible("anthropic", "systemone")).toBe(false);
    expect(failoverCompatible("systemone", "anthropic")).toBe(false);
    expect(failoverCompatible("systemone", "openai")).toBe(true);
    expect(failoverCompatible("systemone", "systemone")).toBe(true);
    // OpenAI serves both, so either kind of fallback can help it.
    expect(failoverCompatible("openai", "systemone")).toBe(true);
    // Embeddings alone are not a request kind the gateway fails over.
    expect(failoverCompatible(undefined, "systemone")).toBe(true);
  });

  it("drops decision-only LLMs from chat pickers", () => {
    const llms = [
      { id: 1, attributes: { name: "GPT", vendor: "openai" } },
      { id: 2, attributes: { name: "Jev", vendor: "systemone" } },
      { id: 3, vendor: "anthropic" },
    ];
    expect(chatCapableLLMs(llms).map((l) => l.id)).toEqual([1, 3]);
    expect(chatCapableLLMs(undefined)).toEqual([]);
  });
});
