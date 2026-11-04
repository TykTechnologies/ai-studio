package eventbridge

import "strings"

// GuardrailAlertTopicPrefix prefixes the topics guardrail alerts are raised
// on (guardrail.alert.<name>). Only Studio's alert publisher raises them,
// after checking the compliance event against its own configuration; plugins
// cannot publish them and the bridge drops them when an edge sends them, so
// a webhook subscribed to an alert receives only checked alerts.
const GuardrailAlertTopicPrefix = "guardrail.alert."

// GuardrailOutcomeTopicPrefix prefixes the topics guardrail outcome events
// are raised on (guardrail.outcome.blocked, .quarantined, .judge_softened;
// services.PublishGuardrailOutcomes). They are reserved like alerts.
const GuardrailOutcomeTopicPrefix = "guardrail.outcome."

// IsGuardrailAlertTopic reports a guardrail alert topic.
func IsGuardrailAlertTopic(topic string) bool {
	return strings.HasPrefix(topic, GuardrailAlertTopicPrefix)
}

// IsGuardrailTopic reports a topic only Studio may raise: a guardrail alert
// or outcome event.
func IsGuardrailTopic(topic string) bool {
	return IsGuardrailAlertTopic(topic) || strings.HasPrefix(topic, GuardrailOutcomeTopicPrefix)
}
