import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import RequestIdCaption from "./RequestIdCaption";

describe("RequestIdCaption", () => {
  it("shows the gateway's request id and the caller's", () => {
    render(
      <RequestIdCaption attributes={{ request_id: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", client_request_id: "lb-1" }} />
    );
    expect(screen.getByText("tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b")).toBeInTheDocument();
    expect(screen.getByText("Client: lb-1")).toBeInTheDocument();
  });

  it("renders nothing for rows from before request ids were recorded", () => {
    const { container } = render(<RequestIdCaption attributes={{ vendor: "openai" }} />);
    expect(container).toBeEmptyDOMElement();
  });
});
