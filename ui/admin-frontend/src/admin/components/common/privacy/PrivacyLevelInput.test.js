import React, { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import PrivacyLevelInput, { PRIVACY_RULE_HELPER } from "./PrivacyLevelInput";

// A stateful harness: the component is controlled, so the tests need the
// value to round-trip through onChange like it does in a form.
const Harness = ({ initial = 0, onChange = () => {}, ...props }) => {
  const [value, setValue] = useState(initial);
  return (
    <PrivacyLevelInput
      value={value}
      onChange={(v) => {
        setValue(v);
        onChange(v);
      }}
      {...props}
    />
  );
};

const score = () => screen.getByRole("spinbutton", { name: "Privacy level score" });
const levelSelect = () => screen.getByTestId("privacy-level-select");
const openLevels = () => fireEvent.mouseDown(screen.getByRole("combobox", { name: /Privacy level/ }));

describe("PrivacyLevelInput", () => {
  it("shows the level for the current score and the one-line rule", () => {
    render(<Harness initial={40} />);
    expect(levelSelect()).toHaveValue("internal");
    expect(screen.getByText("Internal (26–50)")).toBeInTheDocument();
    expect(score()).toHaveValue(40);
    expect(score()).toHaveAttribute("name", "privacy_score");
    expect(screen.getByText(PRIVACY_RULE_HELPER)).toBeInTheDocument();
  });

  it("lists every level with its description", () => {
    render(<Harness initial={0} />);
    openLevels();
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).getByText("Public (0–25)")).toBeInTheDocument();
    expect(within(listbox).getByText("Restricted (76–100)")).toBeInTheDocument();
    expect(within(listbox).getByText("Sensitive data (e.g. financials, strategies)")).toBeInTheDocument();
  });

  it("choosing a level sets the score to that level's default", () => {
    const onChange = jest.fn();
    render(<Harness initial={10} onChange={onChange} />);
    openLevels();
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Confidential (51–75)"));
    expect(onChange).toHaveBeenLastCalledWith(75);
    expect(score()).toHaveValue(75);
    expect(levelSelect()).toHaveValue("confidential");
  });

  it("re-choosing the band the score already sits in leaves the score alone", () => {
    const onChange = jest.fn();
    render(<Harness initial={60} onChange={onChange} />);
    openLevels();
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Confidential (51–75)"));
    // MUI does not fire onChange for the already-selected option, and the
    // component would keep 60 anyway because it is inside the band.
    expect(onChange).not.toHaveBeenCalledWith(75);
    expect(score()).toHaveValue(60);
    expect(levelSelect()).toHaveValue("confidential");
  });

  it("typing a score moves the level to its band and clamps to 0–100", () => {
    const onChange = jest.fn();
    render(<Harness initial={75} onChange={onChange} />);
    fireEvent.change(score(), { target: { value: "10" } });
    expect(onChange).toHaveBeenLastCalledWith(10);
    expect(levelSelect()).toHaveValue("public");
    fireEvent.change(score(), { target: { value: "250" } });
    expect(onChange).toHaveBeenLastCalledWith(100);
    expect(levelSelect()).toHaveValue("restricted");
  });

  it("reports an empty field as '' so the form can flag it, and shows the error", () => {
    const onChange = jest.fn();
    render(<Harness initial={5} onChange={onChange} error helperText="Privacy level must be between 0 and 100" />);
    fireEvent.change(score(), { target: { value: "" } });
    expect(onChange).toHaveBeenLastCalledWith("");
    expect(levelSelect()).toHaveValue("");
    expect(screen.getByText("Privacy level must be between 0 and 100")).toBeInTheDocument();
  });

  it("honours disabled", () => {
    render(<Harness initial={5} disabled />);
    expect(score()).toBeDisabled();
    expect(screen.getByRole("combobox", { name: /Privacy level/ })).toHaveAttribute("aria-disabled", "true");
  });
});
