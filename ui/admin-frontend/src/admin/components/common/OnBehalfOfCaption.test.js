import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import OnBehalfOfCaption from "./OnBehalfOfCaption";

describe("OnBehalfOfCaption", () => {
  it("shows who the call was for and the acting agent", () => {
    render(<OnBehalfOfCaption attributes={{ on_behalf_of: "alice@example.com", acting_agent: "agent-7" }} />);
    expect(screen.getByText("For: alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("Agent: agent-7")).toBeInTheDocument();
  });

  it("shows only what is set", () => {
    render(<OnBehalfOfCaption attributes={{ acting_agent: "agent-7" }} />);
    expect(screen.queryByText(/^For:/)).not.toBeInTheDocument();
    expect(screen.getByText("Agent: agent-7")).toBeInTheDocument();
  });

  it("renders nothing without either", () => {
    const { container } = render(<OnBehalfOfCaption attributes={{ vendor: "openai" }} />);
    expect(container).toBeEmptyDOMElement();
  });
});
