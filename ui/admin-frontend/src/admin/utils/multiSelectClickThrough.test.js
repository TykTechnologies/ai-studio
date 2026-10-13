import React, { useState } from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import "@testing-library/jest-dom";
import { FormControl, InputLabel, MenuItem, Select } from "@mui/material";
import { installMultiSelectClickThrough } from "./multiSelectClickThrough";

// #502 E6: with a multi-select open, the next click outside closes it and
// still reaches what was clicked, instead of being swallowed by the menu's
// invisible backdrop.

const Page = ({ multiple, onOutside }) => {
  const [value, setValue] = useState(multiple ? [] : "");
  return (
    <>
      <FormControl>
        <InputLabel id="teams-label">Teams</InputLabel>
        <Select
          labelId="teams-label"
          label="Teams"
          multiple={multiple}
          value={value}
          onChange={(e) => setValue(e.target.value)}
        >
          <MenuItem value="a">Alpha</MenuItem>
          <MenuItem value="b">Beta</MenuItem>
        </Select>
      </FormControl>
      <button type="button" onClick={onOutside}>
        Outside
      </button>
    </>
  );
};

// A click on the backdrop at (5, 5), with what lies beneath it there.
const clickBackdropOver = (beneath) => {
  // The backdrop has no role or label; it is found by MUI's class.
  // eslint-disable-next-line testing-library/no-node-access
  const backdrop = document.querySelector(".MuiMenu-root .MuiBackdrop-invisible");
  expect(backdrop).toBeTruthy();
  document.elementFromPoint = jest.fn(() => beneath);
  const event = new MouseEvent("click", { bubbles: true, cancelable: true, clientX: 5, clientY: 5 });
  act(() => {
    backdrop.dispatchEvent(event);
  });
};

describe("installMultiSelectClickThrough", () => {
  let uninstall;
  beforeEach(() => {
    uninstall = installMultiSelectClickThrough();
  });
  afterEach(() => {
    uninstall();
    delete document.elementFromPoint;
  });

  it("closes an open multi-select and delivers the click to what was beneath", async () => {
    const onOutside = jest.fn();
    render(<Page multiple onOutside={onOutside} />);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Teams" }));
    fireEvent.click(await screen.findByRole("option", { name: "Alpha" }));
    // Still open after a pick: that is how a multi-select works.
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    clickBackdropOver(screen.getByRole("button", { name: "Outside", hidden: true }));
    await waitFor(() => expect(onOutside).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
  });

  it("leaves single-select menus to MUI", async () => {
    const onOutside = jest.fn();
    render(<Page multiple={false} onOutside={onOutside} />);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Teams" }));
    await screen.findByRole("listbox");
    clickBackdropOver(screen.getByRole("button", { name: "Outside", hidden: true }));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(onOutside).not.toHaveBeenCalled();
  });

  it("stops after uninstall", async () => {
    uninstall();
    const onOutside = jest.fn();
    render(<Page multiple onOutside={onOutside} />);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Teams" }));
    await screen.findByRole("listbox");
    clickBackdropOver(screen.getByRole("button", { name: "Outside", hidden: true }));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(onOutside).not.toHaveBeenCalled();
    uninstall = () => {};
  });
});
