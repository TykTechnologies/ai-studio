import React from "react";
import { render, screen } from "@testing-library/react";
import AppGovernanceFlags from "./AppGovernanceFlags";

describe("AppGovernanceFlags", () => {
  it("renders nothing without flags", () => {
    const { container } = render(<AppGovernanceFlags metadata={{ team: "ops" }} />);
    expect(container).toBeEmptyDOMElement();
    const { container: empty } = render(<AppGovernanceFlags metadata={{ governance_flags: {} }} />);
    expect(empty).toBeEmptyDOMElement();
  });

  it("lists each flag with its value", () => {
    render(
      <AppGovernanceFlags
        metadata={{
          governance_flags: {
            review_lapsed: { value: "2026-09-01", reason: "review lapsed", set_by: "plugin:3", at: "2026-09-15T06:00:00Z" },
            governed_by_asset: { value: "ast_1" },
          },
        }}
      />
    );
    expect(screen.getByText("Review lapsed: 2026-09-01")).toBeInTheDocument();
    expect(screen.getByText("Governed by asset: ast_1")).toBeInTheDocument();
  });
});
