package guardrails

// The catalogue describes every provider the platform knows about. It is
// registered in every build so the API and the admin UI can list providers,
// their detectors and their connection fields; Spec.Available says whether
// this build implements one.

const (
	CategorySecrets    = "secrets"
	CategoryPII        = "pii"
	CategoryInjection  = "injection"
	CategoryLeak       = "leak"
	CategoryModeration = "moderation"
	CategoryTopic      = "topic"
	CategoryOther      = "other"
)

func init() {
	RegisterSpec(Spec{
		Name:        ProviderBuiltin,
		DisplayName: "Built-in pattern library",
		Description: "Runs in-process with no network call. Validated patterns for credentials and API keys, personal data (Luhn-checked cards, mod-97 IBANs), prompt-injection heuristics and response leakage. Enable whole categories and exclude individual patterns.",
		Redacts:     true,
		Local:       true,
		Detectors: []DetectorSpec{
			{Name: CategorySecrets, Category: CategorySecrets, Label: "Credentials and API keys", Description: "Cloud and SaaS API keys, tokens, private keys, connection strings with passwords, generic high-entropy secrets."},
			{Name: CategoryPII, Category: CategoryPII, Label: "Personal data", Description: "Email, phone, national identifiers, payment cards, bank accounts, IP and MAC addresses, dates of birth, addresses."},
			{Name: CategoryInjection, Category: CategoryInjection, Label: "Prompt injection heuristics", Description: "Instruction override, role hijack, system-prompt exfiltration, pseudo chat-template tags, encoding tricks and exfiltration links. Heuristic: pair with a classifier provider for real detection."},
			{Name: CategoryLeak, Category: CategoryLeak, Label: "Response leakage", Description: "System-prompt disclosure markers and markdown exfiltration links in model output. Combine with secrets and pii on a response filter."},
		},
		DocsURL: "/docs/guardrails#built-in",
	})

	RegisterSpec(Spec{
		Name:          ProviderHTTP,
		DisplayName:   "HTTP classifier (generic)",
		Description:   "Any classifier or NER service behind the documented JSON contract: POST {text, direction, detectors} → {flagged, findings[], rewritten?}. Wraps self-hosted models such as Llama Prompt Guard 2 or LLM Guard.",
		Redacts:       true,
		Detectors:     []DetectorSpec{},
		OpenDetectors: true,
		ConnectionFields: []ConnectionField{
			{Name: "endpoint", Label: "Endpoint URL", Description: "Full URL the request is POSTed to.", Required: true, Example: "https://classifier.internal/v1/classify"},
			{Name: "api_key", Label: "API key", Description: "Sent as Authorization: Bearer. Use a $SECRET/ reference.", Secret: true, Example: "$SECRET/CLASSIFIER_KEY"},
			{Name: "auth_header", Label: "Auth header", Description: "Header name to carry the key in when it is not Authorization: Bearer.", Example: "X-API-Key"},
		},
		DocsURL: "/docs/guardrails#http",
	})

	RegisterSpec(Spec{
		Name:        ProviderPresidio,
		DisplayName: "Microsoft Presidio (NER)",
		Description: "Self-hosted named-entity PII detection and anonymisation. Detectors are Presidio entity types; thresholds are the 0..1 recogniser score.",
		Redacts:     true,
		Detectors: presidioDetectors(
			"PERSON", "EMAIL_ADDRESS", "PHONE_NUMBER", "CREDIT_CARD", "IBAN_CODE", "US_SSN", "US_BANK_NUMBER",
			"US_DRIVER_LICENSE", "US_PASSPORT", "US_ITIN", "UK_NHS", "IP_ADDRESS", "LOCATION", "DATE_TIME",
			"NRP", "MEDICAL_LICENSE", "URL", "CRYPTO",
		),
		OpenDetectors: true,
		ConnectionFields: []ConnectionField{
			{Name: "analyzer_url", Label: "Analyzer URL", Description: "Base URL of presidio-analyzer.", Required: true, Example: "http://presidio-analyzer:3000"},
			{Name: "anonymizer_url", Label: "Anonymizer URL", Description: "Base URL of presidio-anonymizer. Optional: without it, redaction is applied locally from the analyzer spans.", Example: "http://presidio-anonymizer:3000"},
			{Name: "language", Label: "Language", Description: "ISO 639-1 code passed to the analyzer. Default en.", Example: "en"},
		},
		DocsURL: "/docs/guardrails#presidio",
	})

	RegisterSpec(Spec{
		Name:        ProviderLakera,
		DisplayName: "Lakera Guard",
		Description: "Lakera Guard v2 screening: prompt attacks, PII, moderated content, unknown links and custom detectors, as configured in the Lakera project policy. PII findings carry spans, so redact is available for them.",
		Redacts:     true,
		Detectors: []DetectorSpec{
			{Name: "prompt_attack", Category: CategoryInjection, Label: "Prompt attack", Description: "Jailbreak and prompt injection."},
			{Name: "pii", Category: CategoryPII, Label: "PII", Description: "Any pii/* detector in the policy. Name a subtype (pii/email) to select just that one."},
			{Name: "moderated_content", Category: CategoryModeration, Label: "Moderated content", Description: "Any moderated_content/* detector. Name a subtype to select just that one."},
			{Name: "unknown_links", Category: CategoryOther, Label: "Unknown links", Description: "Links outside the allow-list."},
			{Name: "custom", Category: CategoryOther, Label: "Custom detectors", Description: "Regex and custom detectors defined in the policy."},
		},
		OpenDetectors: true,
		ConnectionFields: []ConnectionField{
			{Name: "api_key", Label: "API key", Description: "Lakera Guard API key. Not needed for a self-hosted Guard.", Secret: true, Example: "$SECRET/LAKERA_KEY"},
			{Name: "endpoint", Label: "Endpoint", Description: "Guard endpoint. Default https://api.lakera.ai/v2/guard; use the EU or Asia host, or your self-hosted URL.", Example: "https://eu.api.lakera.ai/v2/guard"},
			{Name: "project_id", Label: "Project ID", Description: "Lakera project whose policy applies.", Example: "project-1234567890"},
		},
		RequireAnyConnection: true,
		DocsURL:              "/docs/guardrails#lakera",
	})

	RegisterSpec(Spec{
		Name:        ProviderAzureContentSafety,
		DisplayName: "Azure AI Content Safety",
		Description: "Prompt Shields for prompt attacks and the text moderation categories with severity thresholds (0..6, default 4). Optional blocklists.",
		Detectors: []DetectorSpec{
			{Name: "prompt_attack", Category: CategoryInjection, Label: "Prompt Shields", Description: "User prompt attack detection (text:shieldPrompt)."},
			{Name: "Hate", Category: CategoryModeration, Label: "Hate", HasThreshold: true, ThresholdHint: "severity 0..6, flag at or above; default 4"},
			{Name: "Sexual", Category: CategoryModeration, Label: "Sexual", HasThreshold: true, ThresholdHint: "severity 0..6, flag at or above; default 4"},
			{Name: "SelfHarm", Category: CategoryModeration, Label: "Self-harm", HasThreshold: true, ThresholdHint: "severity 0..6, flag at or above; default 4"},
			{Name: "Violence", Category: CategoryModeration, Label: "Violence", HasThreshold: true, ThresholdHint: "severity 0..6, flag at or above; default 4"},
		},
		ConnectionFields: []ConnectionField{
			{Name: "endpoint", Label: "Endpoint", Description: "Content Safety resource endpoint.", Required: true, Example: "https://my-resource.cognitiveservices.azure.com"},
			{Name: "api_key", Label: "Subscription key", Description: "Sent as Ocp-Apim-Subscription-Key.", Required: true, Secret: true, Example: "$SECRET/AZURE_CONTENT_SAFETY_KEY"},
			{Name: "blocklist_names", Label: "Blocklists", Description: "Comma-separated blocklist names to apply.", Example: "banned-terms"},
		},
		DocsURL: "/docs/guardrails#azure-content-safety",
	})

	RegisterSpec(Spec{
		Name:        ProviderAzurePII,
		DisplayName: "Azure AI Language PII",
		Description: "Named-entity PII detection with server-side redaction. Detectors are Azure PII categories; name specific categories or 'all'. Note that 'all' includes PersonType, Organization and DateTime, which match ordinary prose such as job titles and dates, so prefer an explicit list for blocking policies.",
		Redacts:     true,
		Detectors: azurePIIDetectors(
			"all", "Person", "PersonType", "PhoneNumber", "Organization", "Address", "Email", "URL", "IPAddress",
			"DateTime", "Quantity", "Age", "CreditCardNumber", "InternationalBankingAccountNumber", "SWIFTCode",
			"USSocialSecurityNumber", "USDriversLicenseNumber", "USPassportNumber", "UKNationalInsuranceNumber",
			"UKNationalHealthNumber", "EUPassportNumber", "EUDriversLicenseNumber",
		),
		OpenDetectors: true,
		ConnectionFields: []ConnectionField{
			{Name: "endpoint", Label: "Endpoint", Description: "Language resource endpoint (cloud or container).", Required: true, Example: "https://my-language.cognitiveservices.azure.com"},
			{Name: "api_key", Label: "Subscription key", Description: "Sent as Ocp-Apim-Subscription-Key.", Required: true, Secret: true, Example: "$SECRET/AZURE_LANGUAGE_KEY"},
			{Name: "language", Label: "Language", Description: "Document language. Default en.", Example: "en"},
			{Name: "domain", Label: "Domain", Description: "Set to phi for protected health information.", Example: "phi"},
		},
		DocsURL: "/docs/guardrails#azure-pii",
	})

	RegisterSpec(Spec{
		Name:        ProviderBedrockGuardrails,
		DisplayName: "Amazon Bedrock Guardrails",
		Description: "ApplyGuardrail against a guardrail you define in Bedrock: denied topics, content filters including prompt attacks, word lists, PII (with masking) and regexes. Works with any model, not only Bedrock-hosted ones.",
		Redacts:     true,
		Detectors: []DetectorSpec{
			{Name: "content", Category: CategoryModeration, Label: "Content filters", Description: "Hate, insults, sexual, violence, misconduct and prompt attack, at the strengths set on the guardrail."},
			{Name: "topic", Category: CategoryTopic, Label: "Denied topics"},
			{Name: "word", Category: CategoryOther, Label: "Word filters"},
			{Name: "sensitive_information", Category: CategoryPII, Label: "Sensitive information", Description: "PII entities and custom regexes; anonymised entities are returned as masked text."},
			{Name: "grounding", Category: CategoryOther, Label: "Contextual grounding"},
		},
		ConnectionFields: []ConnectionField{
			{Name: "guardrail_id", Label: "Guardrail ID", Required: true, Example: "abc123def456"},
			{Name: "guardrail_version", Label: "Guardrail version", Description: "A version number or DRAFT.", Required: true, Example: "DRAFT"},
			{Name: "region", Label: "AWS region", Required: true, Example: "us-east-1"},
			{Name: "access_key_id", Label: "Access key ID", Description: "Leave empty to use the gateway's ambient AWS credentials.", Secret: true, Example: "$SECRET/AWS_ACCESS_KEY_ID"},
			{Name: "secret_access_key", Label: "Secret access key", Secret: true, Example: "$SECRET/AWS_SECRET_ACCESS_KEY"},
		},
		DocsURL: "/docs/guardrails#bedrock",
	})
}

func presidioDetectors(names ...string) []DetectorSpec {
	out := make([]DetectorSpec, 0, len(names))
	for _, n := range names {
		out = append(out, DetectorSpec{Name: n, Category: CategoryPII, Label: n, HasThreshold: true, ThresholdHint: "recogniser score 0..1; default 0.5"})
	}
	return out
}

func azurePIIDetectors(names ...string) []DetectorSpec {
	out := make([]DetectorSpec, 0, len(names))
	for _, n := range names {
		out = append(out, DetectorSpec{Name: n, Category: CategoryPII, Label: n, HasThreshold: true, ThresholdHint: "confidence 0..1; default 0.5"})
	}
	return out
}
