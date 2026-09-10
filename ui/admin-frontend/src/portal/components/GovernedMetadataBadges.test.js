import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import GovernedMetadataBadges from "./GovernedMetadataBadges";

describe("GovernedMetadataBadges", () => {
  it("renders label: value chips, joining arrays and formatting booleans", () => {
    render(
      <GovernedMetadataBadges
        items={[
          { key: "data_classification", label: "Data classification", type: "vocabulary", value: "Confidential" },
          { key: "regulatory_applicability", label: "Regulations", type: "multi_vocabulary", value: ["GDPR", "HIPAA"] },
          { key: "approved", label: "Approved", type: "boolean", value: true },
        ]}
      />
    );
    expect(screen.getByText("Data classification: Confidential")).toBeInTheDocument();
    expect(screen.getByText("Regulations: GDPR, HIPAA")).toBeInTheDocument();
    expect(screen.getByText("Approved: Yes")).toBeInTheDocument();
  });

  it("renders nothing for empty or missing items", () => {
    const { container } = render(<GovernedMetadataBadges items={[]} />);
    expect(container).toBeEmptyDOMElement();
    const { container: c2 } = render(<GovernedMetadataBadges />);
    expect(c2).toBeEmptyDOMElement();
  });
});
