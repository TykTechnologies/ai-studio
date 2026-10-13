import React, { useState } from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { FormControl, InputLabel, MenuItem, Select, TextField } from "@mui/material";
import focusFirstInvalid, { focusFirstInvalidNow, focusTargetFor } from "./focusFirstInvalid";

// #502 E8: a failed submit brings the first invalid control into view and
// focuses it, from what the form rendered as invalid.

beforeEach(() => {
  Element.prototype.scrollIntoView = jest.fn();
});

// A long form: a valid field, then an invalid select, then an invalid text
// field. The select comes first, so it is the one to fix first.
const LongForm = () => {
  const [errors, setErrors] = useState({});
  const onSubmit = (e) => {
    e.preventDefault();
    setErrors({ vendor: "Pick a vendor", name: "Name is required" });
    focusFirstInvalid(e.currentTarget);
  };
  return (
    <>
      <TextField label="Outside the form" error />
      <form onSubmit={onSubmit}>
        <TextField label="Description" />
        <FormControl error={!!errors.vendor}>
          <InputLabel id="vendor-label">Vendor</InputLabel>
          <Select labelId="vendor-label" label="Vendor" value="">
            <MenuItem value="a">A</MenuItem>
          </Select>
        </FormControl>
        <TextField label="Name" error={!!errors.name} />
        <button type="submit">Save</button>
      </form>
    </>
  );
};

describe("focusFirstInvalid", () => {
  it("focuses the first invalid control in the submitted form and scrolls it into view", async () => {
    render(<LongForm />);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const vendor = screen.getByRole("combobox", { name: "Vendor" });
    await waitFor(() => expect(vendor).toHaveFocus());
    expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "center" });
  });

  it("does nothing when nothing is invalid", () => {
    render(<TextField label="Fine" />);
    expect(focusFirstInvalidNow(document)).toBe(false);
    expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
  });

  it("focuses a text field marked aria-invalid directly", () => {
    render(<TextField label="Email" error />);
    expect(focusFirstInvalidNow(document)).toBe(true);
    expect(screen.getByRole("textbox", { name: "Email" })).toHaveFocus();
  });

  it("maps an errored Select's hidden native input to its visible combobox", () => {
    render(
      <FormControl error>
        <InputLabel id="v-label">Vendor</InputLabel>
        <Select labelId="v-label" label="Vendor" value="">
          <MenuItem value="a">A</MenuItem>
        </Select>
      </FormControl>
    );
    // MUI marks the hidden native input, which cannot take focus usefully.
    // eslint-disable-next-line testing-library/no-node-access
    const native = document.querySelector(".MuiSelect-nativeInput");
    expect(native).toBeTruthy();
    expect(focusTargetFor(native)).toBe(screen.getByRole("combobox", { name: "Vendor" }));
  });

  it("maps a custom picker's .Mui-error control to its first focusable element", () => {
    render(
      <div className="MuiFormControl-root Mui-error" data-testid="picker">
        <span>Embedder</span>
        <button type="button">Choose</button>
      </div>
    );
    expect(focusTargetFor(screen.getByTestId("picker"))).toBe(screen.getByRole("button", { name: "Choose" }));
  });
});
