import {
  basePath,
  withBase,
  stripBase,
  authMode,
  isHostAuth,
  hostLoginURL,
  csrfTokenHeader,
  csrfTokenURL,
  isChromeless,
  applyChrome,
} from "./runtimeConfig";

describe("runtimeConfig", () => {
  afterEach(() => {
    delete window.__TYK_AI_STUDIO__;
  });

  it("defaults to the root and Studio's own sign-in", () => {
    expect(basePath()).toBe("");
    expect(withBase("/api/v1")).toBe("/api/v1");
    expect(stripBase("/admin")).toBe("/admin");
    expect(authMode()).toBe("local");
    expect(isHostAuth()).toBe(false);
    expect(csrfTokenHeader()).toBe("X-CSRF-Token");
    expect(csrfTokenURL()).toBe("/csrf-token");
  });

  it("puts absolute paths under the injected base path", () => {
    window.__TYK_AI_STUDIO__ = { basePath: "/ai-studio/" };
    expect(basePath()).toBe("/ai-studio");
    expect(withBase("/api/v1/llms")).toBe("/ai-studio/api/v1/llms");
    expect(withBase("/ai-studio/admin")).toBe("/ai-studio/admin");
    expect(withBase("https://example.com/x")).toBe("https://example.com/x");
    expect(withBase("//cdn.example.com/x")).toBe("//cdn.example.com/x");
    expect(withBase("relative/path")).toBe("relative/path");
    expect(csrfTokenURL()).toBe("/ai-studio/csrf-token");
  });

  it("strips the base path from browser paths", () => {
    window.__TYK_AI_STUDIO__ = { basePath: "/ai-studio" };
    expect(stripBase("/ai-studio/admin/llms")).toBe("/admin/llms");
    expect(stripBase("/ai-studio")).toBe("/");
    expect(stripBase("/ai-studiox/admin")).toBe("/ai-studiox/admin");
  });

  it("reads host authentication and CSRF settings", () => {
    window.__TYK_AI_STUDIO__ = {
      authMode: "host",
      loginURL: "/login",
      csrfTokenHeader: "X-Host-CSRF",
      csrfTokenURL: "/host/csrf",
    };
    expect(isHostAuth()).toBe(true);
    expect(hostLoginURL()).toBe("/login");
    expect(csrfTokenHeader()).toBe("X-Host-CSRF");
    expect(csrfTokenURL()).toBe("/host/csrf");
  });

  it("draws its own chrome unless the host asks for pages only", () => {
    const root = document.createElement("div");
    expect(isChromeless()).toBe(false);
    applyChrome(root);
    expect(root.style.getPropertyValue("--studio-header-height")).toBe("");

    window.__TYK_AI_STUDIO__ = { chrome: "none" };
    expect(isChromeless()).toBe(true);
    applyChrome(root);
    expect(root.style.getPropertyValue("--studio-header-height")).toBe("0px");
    expect(root.dataset.studioChrome).toBe("none");
  });
});
