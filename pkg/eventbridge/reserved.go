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

// IsCredentialTopic reports the credential.changed topic
// (CredentialChangedTopic). Only Studio raises it, from its own credential
// writes: plugins cannot publish it, on Studio or on an edge, and the
// bridge drops it when an edge sends it. Edges only ever drop cached state
// on it, but a payload from anywhere else must not reach them as Studio's.
func IsCredentialTopic(topic string) bool {
	return topic == CredentialChangedTopic
}

// PluginPublishRefusal returns why a plugin may not publish an event on
// topic, or "" when it may: guardrail alerts and outcomes and
// credential.changed are raised by AI Studio only. Both plugin event
// servers (Studio's and the gateway's) refuse with it.
func PluginPublishRefusal(topic string) string {
	switch {
	case IsCredentialTopic(topic):
		return CredentialChangedTopic + " is raised by AI Studio only; plugins cannot publish it"
	case IsGuardrailTopic(topic):
		return "topics under " + GuardrailAlertTopicPrefix + " and " + GuardrailOutcomeTopicPrefix + " are raised by AI Studio only; plugins cannot publish them"
	}
	return ""
}
