package eventbridge

import "strings"

// GuardrailAlertTopicPrefix prefixes the topics guardrail alerts are raised
// on (guardrail.alert.<name>). Only Studio's alert publisher raises them,
// after checking the compliance event against its own configuration; plugins
// cannot publish them and the bridge drops them when an edge sends them, so
// a webhook subscribed to an alert receives only checked alerts.
const GuardrailAlertTopicPrefix = "guardrail.alert."

// IsGuardrailAlertTopic reports a topic only Studio's alert publisher may
// raise.
func IsGuardrailAlertTopic(topic string) bool {
	return strings.HasPrefix(topic, GuardrailAlertTopicPrefix)
}
