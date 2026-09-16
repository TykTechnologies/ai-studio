import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import MCPSubmissionFields, { validateMCPPayload } from "./MCPSubmissionFields";

const connections = [
  { id: 1, name: "Prod", effective_mode: "full", direct_create: true, accepts_handoffs: true, rest_to_mcp_supported: true, gateway_tags: [{ tag: "edge-eu", verified: false }] },
  { id: 2, name: "Legacy", effective_mode: "catalogue", direct_create: false, accepts_handoffs: true, rest_to_mcp_supported: false, gateway_tags: [] },
];

describe("validateMCPPayload", () => {
  it("requires the connection, a description and an upstream URL for remote servers", () => {
    const errors = validateMCPPayload({ kind: "remote" }, connections);
    expect(errors.connection_id).toBeTruthy();
    expect(errors.description).toBeTruthy();
    expect(errors.upstream_url).toBeTruthy();
  });

  it("wants a deployment target, or a confirmation, when the connection knows tags", () => {
    const base = { connection_id: 1, description: "d", kind: "remote", upstream_url: "https://w.example.com" };
    expect(validateMCPPayload(base, connections).gateway_tags).toBeTruthy();
    expect(validateMCPPayload({ ...base, confirm_no_gateway_tags: true }, connections)).toEqual({});
    expect(validateMCPPayload({ ...base, gateway_tags: ["edge-eu"] }, connections)).toEqual({});
    // A connection without tags never asks.
    expect(validateMCPPayload({ ...base, connection_id: 2 }, connections)).toEqual({});
  });

  it("refuses REST-to-MCP on a Dashboard that lacks it, keyless without confirmation and OAuth without servers", () => {
    expect(validateMCPPayload({ connection_id: 2, description: "d", kind: "rest_to_mcp", source_api_id: "x" }, connections).kind).toContain("5.15");
    expect(validateMCPPayload({ connection_id: 2, description: "d", kind: "remote", upstream_url: "https://u", consumer_auth: "keyless" }, connections).confirm_keyless).toBeTruthy();
    expect(validateMCPPayload({ connection_id: 2, description: "d", kind: "remote", upstream_url: "https://u", consumer_auth: "oauth21" }, connections).authorization_servers).toBeTruthy();
    expect(validateMCPPayload({ connection_id: 2, description: "d", kind: "remote", upstream_url: "https://u", suggested_listen_path: "/Bad Path" }, connections).suggested_listen_path).toBeTruthy();
    expect(validateMCPPayload({ connection_id: 2, description: "d", kind: "remote", upstream_url: "https://u", upstream_auth_header_name: "X-Token" }, connections).upstream_auth_token).toBeTruthy();
  });
});

describe("MCPSubmissionFields", () => {
  it("writes typed values into the payload and shows the target control only when tags are known", () => {
    const changes = [];
    const onChange = (k, v) => changes.push([k, v]);
    const { rerender } = render(<MCPSubmissionFields payload={{ connection_id: 2 }} onChange={onChange} connections={connections} />);
    expect(screen.queryByTestId("mcp-gateway-tags")).not.toBeInTheDocument();
    expect(screen.getByText(/platform team creates the proxy/)).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("mcp-upstream-url"), { target: { value: "https://w.example.com/mcp" } });
    fireEvent.change(screen.getByTestId("mcp-upstream-token"), { target: { value: "s3cr3t" } });
    expect(changes).toContainEqual(["upstream_url", "https://w.example.com/mcp"]);
    expect(changes).toContainEqual(["upstream_auth_token", "s3cr3t"]);

    rerender(<MCPSubmissionFields payload={{ connection_id: 1, consumer_auth: "keyless" }} onChange={onChange} connections={connections} errors={{ gateway_tags: "Choose a deployment target" }} />);
    expect(screen.getByTestId("mcp-gateway-tags")).toBeInTheDocument();
    expect(screen.getByText("Choose a deployment target")).toBeInTheDocument();
    expect(screen.getByText(/AI Studio creates the proxy/)).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("mcp-confirm-keyless"));
    expect(changes).toContainEqual(["confirm_keyless", true]);
    fireEvent.click(screen.getByTestId("mcp-confirm-no-tags"));
    expect(changes).toContainEqual(["confirm_no_gateway_tags", true]);
  });
});
