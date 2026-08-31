import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "./utils/testTheme";
import { TextField } from "@mui/material";

// MUI puts the required asterisk inside the label, so it lands in the
// control's accessible name: a field labelled "Name" announced as "Name *",
// and getByLabelText("Name") resolving to nothing. The theme hides the marker
// from the accessibility tree while leaving it on screen.
it("a required field's accessible name excludes the asterisk", () => {
  render(
    <ThemeProvider theme={testTheme}>
      <TextField label="Catalog Name" required />
    </ThemeProvider>
  );
  // getByRole uses the real accessible-name computation, which honours
  // aria-hidden. (getByLabelText matches a label's textContent instead, so it
  // still sees the asterisk -- that is a Testing Library detail, not what a
  // screen reader announces.)
  expect(
    screen.getByRole("textbox", { name: "Catalog Name" })
  ).toBeInTheDocument();
});

it("the asterisk is still rendered for sighted users", () => {
  const { container } = render(
    <ThemeProvider theme={testTheme}>
      <TextField label="Catalog Name" required />
    </ThemeProvider>
  );
  // The asterisk is deliberately hidden from the accessibility tree, so it has
  // no accessible representation for a Testing Library query to find -- the
  // DOM node is the only way to assert it is still rendered and still hidden.
  // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access
  const asterisk = container.querySelector(".MuiFormLabel-asterisk");
  expect(asterisk).toBeInTheDocument();
  expect(asterisk).toHaveAttribute("aria-hidden", "true");
});
