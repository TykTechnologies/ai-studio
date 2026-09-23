import React, { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import BudgetField from "./BudgetField";

// A stateful harness: the field is controlled.
const Harness = ({ initial, onValue, ...props }) => {
  const [value, setValue] = useState(initial);
  return (
    <BudgetField
      value={value}
      onChange={(v) => {
        setValue(v);
        onValue(v);
      }}
      testIdPrefix="b"
      {...props}
    />
  );
};

const chooseMode = (label) => {
  fireEvent.mouseDown(screen.getByRole("combobox"));
  fireEvent.click(within(screen.getByRole("listbox")).getByText(label));
};

describe("BudgetField", () => {
  it("treats no budget as 'No limit' and 0 as a real, blocking budget", () => {
    const onValue = jest.fn();
    render(<Harness initial={null} onValue={onValue} />);
    expect(screen.getByRole("combobox")).toHaveTextContent("No limit");
    expect(screen.queryByTestId("b-amount")).not.toBeInTheDocument();

    chooseMode("Fixed amount");
    expect(onValue).toHaveBeenLastCalledWith(0);
    expect(screen.getByTestId("b-help")).toHaveTextContent("A budget of 0: nothing may be spent");

    fireEvent.change(screen.getByTestId("b-amount"), { target: { value: "12.5" } });
    expect(onValue).toHaveBeenLastCalledWith(12.5);
    expect(screen.getByTestId("b-help")).toHaveTextContent("Requests are refused once");

    chooseMode("No limit");
    expect(onValue).toHaveBeenLastCalledWith(null);
  });

  it("shows an existing 0 as a fixed amount, not as no limit", () => {
    render(<Harness initial={0} onValue={jest.fn()} />);
    expect(screen.getByRole("combobox")).toHaveTextContent("Fixed amount");
    expect(screen.getByTestId("b-amount")).toHaveValue(0);
  });

  it("names the empty choice for new Apps", () => {
    render(<Harness initial={null} onValue={jest.fn()} emptyLabel="Default" emptyHelp="The team's default." />);
    expect(screen.getByRole("combobox")).toHaveTextContent("Default");
    expect(screen.getByTestId("b-help")).toHaveTextContent("The team's default.");
  });
});
