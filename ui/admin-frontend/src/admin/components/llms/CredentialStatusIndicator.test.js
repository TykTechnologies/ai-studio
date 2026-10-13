import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import {
  CredentialStatusDot,
  CredentialStatusNotice,
  credentialPresentation,
} from "./CredentialStatusIndicator";

const renderWithTheme = (ui) =>
  render(<ThemeProvider theme={testTheme}>{ui}</ThemeProvider>);

describe("credentialPresentation", () => {
  // The fresh-instance case: the seeded providers point at bootstrap secrets
  // that were created with empty values, and the list showed them as healthy.
  it("treats an unresolved secret as a warning and names the secret", () => {
    const p = credentialPresentation("unresolved", "OPENAI_KEY");
    expect(p.severity).toBe("warning");
    expect(p.detail).toContain("OPENAI_KEY");
  });

  it("words an unresolved $ENV/ reference as an unset environment variable, not an empty secret", () => {
    const env = credentialPresentation("unresolved", "OPENAI_API_KEY", "env");
    expect(env.severity).toBe("warning");
    expect(env.detail).toContain("environment variable OPENAI_API_KEY, which is not set on the server");
    expect(env.detail).not.toMatch(/secret/i);

    const secret = credentialPresentation("unresolved", "OPENAI_KEY", "secret");
    expect(secret.detail).toContain("points at the secret OPENAI_KEY");
  });

  it("describes a resolved $ENV/ key as coming from the environment, not the vault", () => {
    const env = credentialPresentation("secret", "OPENAI_API_KEY", "env");
    expect(env.severity).toBe("ok");
    expect(env.label).toBe("Key from environment: OPENAI_API_KEY");
    expect(env.detail).toContain("environment variable OPENAI_API_KEY");
    expect(env.label + env.detail).not.toMatch(/vault|secret/i);
    expect(credentialPresentation("secret", "OPENAI_KEY", "secret").label).toBe("Key from vault: OPENAI_KEY");
  });

  it("shows the environment wording on the detail notice", () => {
    renderWithTheme(<CredentialStatusNotice status="unresolved" reference="OPENAI_API_KEY" origin="env" />);
    expect(screen.getByText(/environment variable OPENAI_API_KEY/)).toBeInTheDocument();
  });

  it("treats a missing key as a warning", () => {
    expect(credentialPresentation("unset").severity).toBe("warning");
  });

  it("distinguishes a vault-backed key from an inline one", () => {
    const vault = credentialPresentation("secret", "OPENAI_KEY");
    const inline = credentialPresentation("inline");
    expect(vault.severity).toBe("ok");
    expect(vault.label).toContain("OPENAI_KEY");
    expect(inline.severity).toBe("ok");
    expect(inline.label).toBe("Inline key");
    expect(inline.detail).toContain("not in the vault");
  });
});

describe("CredentialStatusDot", () => {
  it("warns on an active provider whose credential cannot resolve", () => {
    // Active AND broken is the exact combination that misled people.
    renderWithTheme(
      <CredentialStatusDot active status="unresolved" reference="OPENAI_KEY" />
    );
    expect(screen.getByTitle("Active")).toBeInTheDocument();
    expect(screen.getByTitle("Credential unresolved")).toBeInTheDocument();
  });

  it("shows no warning when the credential resolves", () => {
    renderWithTheme(
      <CredentialStatusDot active status="secret" reference="OPENAI_KEY" />
    );
    expect(screen.queryByTitle("Credential unresolved")).not.toBeInTheDocument();
  });
});

describe("CredentialStatusNotice", () => {
  it("names the secret that needs a value", () => {
    renderWithTheme(
      <CredentialStatusNotice status="unresolved" reference="ANTHROPIC_KEY" />
    );
    expect(screen.getByText("Credential unresolved")).toBeInTheDocument();
    expect(screen.getByText(/ANTHROPIC_KEY/)).toBeInTheDocument();
  });
});
