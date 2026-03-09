import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import { createTheme } from "@mui/material";
import SecretForm from "./SecretForm";

// Mock apiClient
jest.mock("../../utils/apiClient", () => {
  const mockClient = {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() },
    },
  };
  return {
    __esModule: true,
    default: mockClient,
  };
});

// Mock useNavigate — default to create mode
const mockNavigate = jest.fn();
let mockId;
jest.mock("react-router-dom", () => ({
  ...jest.requireActual("react-router-dom"),
  useNavigate: () => mockNavigate,
  useParams: () => ({ id: mockId }),
}));

describe("SecretForm Component", () => {
  const theme = createTheme({
    palette: {
      text: {
        primary: "#ffffff",
        defaultSubdued: "rgba(255, 255, 255, 0.6)",
      },
      background: {
        buttonPrimaryDefault: "#007bff",
        buttonPrimaryDefaultHover: "#0069d9",
      },
      custom: { white: "#ffffff" },
      primary: { main: "#7b68ee" },
      error: { main: "#dc3545" },
      border: { neutralDefault: "#e0e0e0" },
    },
    typography: {
      bodyLargeMedium: { fontSize: "1rem", fontWeight: 500 },
      headingXLarge: { fontSize: "2rem", fontWeight: "bold" },
    },
  });

  const Wrapper = ({ children }) => (
    <ThemeProvider theme={theme}>
      <MemoryRouter>
        <Routes>
          <Route path="*" element={children} />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );

  let apiClient;

  beforeEach(() => {
    jest.clearAllMocks();
    mockId = undefined;
    apiClient = require("../../utils/apiClient").default;
    apiClient.get.mockReset();
    apiClient.post.mockReset();
    apiClient.patch.mockReset();
    apiClient.post.mockResolvedValue({ data: { data: { id: "1" } } });
    apiClient.patch.mockResolvedValue({ data: { data: { id: "1" } } });
  });

  test("create mode sends value in request", async () => {
    mockId = undefined;

    render(<SecretForm />, { wrapper: Wrapper });

    fireEvent.change(screen.getByLabelText(/Variable Name/i), {
      target: { value: "MY_KEY" },
    });
    fireEvent.change(screen.getByLabelText(/Secret Value/i), {
      target: { value: "my-secret-123" },
    });

    fireEvent.click(screen.getByRole("button", { name: /Add secret/i }));

    await waitFor(() => {
      expect(apiClient.post).toHaveBeenCalledWith("/secrets", {
        data: {
          type: "secrets",
          attributes: {
            var_name: "MY_KEY",
            value: "my-secret-123",
          },
        },
      });
    });
  });

  test("edit mode does NOT send value when user does not modify it", async () => {
    mockId = "42";

    apiClient.get.mockResolvedValue({
      data: {
        data: {
          attributes: {
            var_name: "EXISTING_KEY",
            value: "$SECRET/EXISTING_KEY",
          },
        },
      },
    });

    render(<SecretForm />, { wrapper: Wrapper });

    // Wait for the secret data to load
    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledWith("/secrets/42");
    });

    // Only change the var_name, leave value untouched
    fireEvent.change(screen.getByLabelText(/Variable Name/i), {
      target: { value: "RENAMED_KEY" },
    });

    fireEvent.click(screen.getByRole("button", { name: /Update secret/i }));

    await waitFor(() => {
      expect(apiClient.patch).toHaveBeenCalledWith("/secrets/42", {
        data: {
          type: "secrets",
          attributes: {
            var_name: "RENAMED_KEY",
            // value should NOT be included
          },
        },
      });
    });

    // Verify value was NOT sent
    const patchCall = apiClient.patch.mock.calls[0][1];
    expect(patchCall.data.attributes).not.toHaveProperty("value");
  });

  test("edit mode sends value when user modifies it", async () => {
    mockId = "42";

    apiClient.get.mockResolvedValue({
      data: {
        data: {
          attributes: {
            var_name: "EXISTING_KEY",
            value: "$SECRET/EXISTING_KEY",
          },
        },
      },
    });

    render(<SecretForm />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(apiClient.get).toHaveBeenCalledWith("/secrets/42");
    });

    // Change the value field
    fireEvent.change(screen.getByLabelText(/Secret Value/i), {
      target: { value: "brand-new-secret" },
    });

    fireEvent.click(screen.getByRole("button", { name: /Update secret/i }));

    await waitFor(() => {
      expect(apiClient.patch).toHaveBeenCalledWith("/secrets/42", {
        data: {
          type: "secrets",
          attributes: {
            var_name: "EXISTING_KEY",
            value: "brand-new-secret",
          },
        },
      });
    });
  });

  test("edit mode submit button is not disabled when value is unchanged", async () => {
    mockId = "42";

    apiClient.get.mockResolvedValue({
      data: {
        data: {
          attributes: {
            var_name: "EXISTING_KEY",
            value: "$SECRET/EXISTING_KEY",
          },
        },
      },
    });

    render(<SecretForm />, { wrapper: Wrapper });

    await waitFor(() => {
      expect(screen.getByLabelText(/Variable Name/i)).toHaveValue(
        "EXISTING_KEY"
      );
    });

    const submitButton = screen.getByRole("button", {
      name: /Update secret/i,
    });
    expect(submitButton).not.toBeDisabled();
  });
});
