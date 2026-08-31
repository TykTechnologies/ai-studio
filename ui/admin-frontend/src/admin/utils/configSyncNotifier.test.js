import {
  affectsGatewayConfig,
  notifyConfigChanged,
  registerSyncStatusRefresh,
  __resetForTests,
} from "./configSyncNotifier";

describe("affectsGatewayConfig", () => {
  it("matches mutating requests to gateway-affecting resources", () => {
    expect(affectsGatewayConfig("post", "/filters")).toBe(true);
    expect(affectsGatewayConfig("patch", "/llms/3")).toBe(true);
    expect(affectsGatewayConfig("delete", "/tools/7")).toBe(true);
    expect(affectsGatewayConfig("PATCH", "/apps/1?x=1")).toBe(true);
  });

  it("ignores reads", () => {
    expect(affectsGatewayConfig("get", "/llms")).toBe(false);
  });

  it("ignores resources the gateways do not consume", () => {
    expect(affectsGatewayConfig("post", "/users")).toBe(false);
    expect(affectsGatewayConfig("patch", "/branding")).toBe(false);
  });

  it("does not treat pushing configuration as a config change", () => {
    // Otherwise a push would schedule a refresh that races its own result.
    expect(affectsGatewayConfig("post", "/edges/reload-all")).toBe(false);
  });
});

describe("notifyConfigChanged", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    __resetForTests();
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  it("does nothing when no refresh is registered", () => {
    expect(() => notifyConfigChanged()).not.toThrow();
    jest.runAllTimers();
  });

  it("coalesces a burst of saves into one refresh", () => {
    // A catalog form issues one request per member; that should not produce
    // one sync-status fetch per member.
    const refresh = jest.fn();
    registerSyncStatusRefresh(refresh);

    notifyConfigChanged();
    notifyConfigChanged();
    notifyConfigChanged();
    jest.runAllTimers();

    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("stops calling a refresh that has been unregistered", () => {
    const refresh = jest.fn();
    const unregister = registerSyncStatusRefresh(refresh);
    unregister();

    notifyConfigChanged();
    jest.runAllTimers();

    expect(refresh).not.toHaveBeenCalled();
  });
});
