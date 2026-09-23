import React, { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import BudgetField from "./BudgetField";

// The Playwright page objects (tests/ui/pom) find the budget field by these
// names; keep them in step with the component.
const Harness = ({ prefix, emptyLabel }) => {
  const [value, setValue] = useState(null);
  return <BudgetField value={value} onChange={setValue} testIdPrefix={prefix} emptyLabel={emptyLabel} />;
};

it.each([["app-budget", "Default"], ["llm-budget", "No limit"]])(
  "%s: combobox named 'Monthly budget…', option 'Fixed amount', then the amount box",
  (prefix, emptyLabel) => {
    render(<Harness prefix={prefix} emptyLabel={emptyLabel} />);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: /^Monthly budget/ }));
    fireEvent.click(screen.getByRole("option", { name: "Fixed amount" }));
    fireEvent.change(screen.getByTestId(`${prefix}-amount`), { target: { value: "10" } });
    expect(screen.getByTestId(`${prefix}-amount`)).toHaveValue(10);
  },
);
