import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "./utils/testTheme";
import { FormControl, InputLabel, Select, MenuItem } from "@mui/material";

/**
 * MUI derives a default id of `mui-component-select-{name}` for a Select, and
 * the Playwright suite selects on exactly that (`#mui-component-select-user_id`,
 * `-llm_ids`, `-tool_ids`, `-vendor`). Passing an explicit `id` REPLACES it.
 *
 * The accessibility sweep originally set both `labelId` and `id` on every
 * Select. The `id` was redundant -- `labelId` is what produces the
 * aria-labelledby association -- and it silently removed the default id,
 * breaking every one of those E2E selectors with a click timeout rather than a
 * useful error.
 *
 * These pin both properties at once: the control is programmatically labelled,
 * AND the default id survives.
 */
describe("Select labelling must not clobber MUI's default id", () => {
  const renderSelect = () =>
    render(
      <ThemeProvider theme={testTheme}>
        <FormControl fullWidth>
          <InputLabel id="demo-label">User</InputLabel>
          <Select labelId="demo-label" name="user_id" value="" onChange={() => {}}>
            <MenuItem value="1">Alice</MenuItem>
          </Select>
        </FormControl>
      </ThemeProvider>
    );

  it("keeps the mui-component-select-{name} id the E2E suite selects on", () => {
    const { container } = renderSelect();
    // Querying by DOM id is the point here: the Playwright suite selects on
    // this exact id, and no Testing Library query can assert an id's presence.
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access
    expect(container.querySelector("#mui-component-select-user_id")).toBeInTheDocument();
  });

  it("still gives the control an accessible name from its label", () => {
    renderSelect();
    expect(screen.getByRole("combobox", { name: "User" })).toBeInTheDocument();
  });
});
