import React, { useState } from "react";
import { Alert } from "@mui/material";
import Section from "../common/Section";
import SubSection from "../common/SubSection";
import { useEdition } from "../../context/EditionContext";
import AppGuardrailCapability from "./AppGuardrailCapability";
import AppGuardrailExceptions from "./guardrailExceptions/AppGuardrailExceptions";
import AppPromptTemplates from "./promptTemplates/AppPromptTemplates";

// The App page's Guardrails section (features/GuardrailExceptions.md): what
// the app can do (its attested capability), the guardrail exceptions that
// capability bounds, and its trusted prompt templates.
const AppGuardrails = ({ appId }) => {
  const { isEnterprise } = useEdition();
  const [capability, setCapability] = useState(null);

  return (
    <Section
      title="Guardrails"
      description="How the guardrail filters on this app's LLMs treat its traffic."
      data-testid="app-guardrails"
    >
      {!isEnterprise && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Guardrails act through Enterprise guardrail providers. In the Community Edition capability, exceptions and
          templates can be set, and change nothing.
        </Alert>
      )}
      <SubSection
        title="Capability"
        description="What the app can do. It bounds how far an exception may soften a verdict."
      >
        <AppGuardrailCapability appId={appId} onChange={setCapability} />
      </SubSection>
      <AppGuardrailExceptions appId={appId} capability={capability?.effective || "acting"} />
      <AppPromptTemplates appId={appId} embedded />
    </Section>
  );
};

export default AppGuardrails;
