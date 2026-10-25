import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import AllowInternalHostSwitch from "./AllowInternalHostSwitch";
import { usePermissions } from "../../context/PermissionsContext";

jest.mock("../../context/PermissionsContext", () => ({ usePermissions: jest.fn() }));

describe("AllowInternalHostSwitch", () => {
  it("lets an administrator switch it", () => {
    usePermissions.mockReturnValue({ isFullAdmin: true });
    const onChange = jest.fn();
    render(<AllowInternalHostSwitch checked={false} onChange={onChange} target="this tool's API host" />);
    const toggle = screen.getByRole("checkbox", { name: "Allow internal host" });
    expect(toggle).toBeEnabled();
    fireEvent.click(toggle);
    expect(onChange).toHaveBeenCalledWith(true);
    expect(screen.getByText(/this tool's API host/)).toBeInTheDocument();
  });

  it("is read-only for anyone else", () => {
    usePermissions.mockReturnValue({ isFullAdmin: false });
    render(<AllowInternalHostSwitch checked onChange={jest.fn()} />);
    const toggle = screen.getByRole("checkbox", { name: "Allow internal host" });
    expect(toggle).toBeDisabled();
    expect(toggle).toBeChecked();
    expect(screen.getByText(/Only an administrator can change this/)).toBeInTheDocument();
  });
});
