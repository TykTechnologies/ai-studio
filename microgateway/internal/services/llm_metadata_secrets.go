package services

import (
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/rs/zerolog/log"
)

// An LLM's metadata credentials (models.IsCredentialMetadataKey: Bedrock's
// AWS keys) are kept out of database.LLM.Metadata (aa5): their values are
// stored encrypted in MetadataSecretsEncrypted, and Metadata holds
// secrets.RefusedCredential in their place. The hub sends them that way
// (LLMConfig.metadata_secrets_encrypted); a standalone edge's own writes
// split them the same way (splitLLMMetadataSecrets). Only the gateway, when
// it builds the LLM for a vendor client, sees the values
// (overlayLLMMetadataSecrets).

// encryptedLLMMetadataSecrets decodes a MetadataSecretsEncrypted column.
func encryptedLLMMetadataSecrets(column database.JSON) (map[string]string, error) {
	if len(column) == 0 {
		return nil, nil
	}
	var encrypted map[string]string
	if err := json.Unmarshal(column, &encrypted); err != nil {
		return nil, err
	}
	return encrypted, nil
}

// overlayLLMMetadataSecrets is metadata with each credential the column
// holds decrypted in place. A value that does not decrypt (or is empty)
// becomes secrets.RefusedCredential, never the ciphertext or an empty
// credential, which a vendor client would replace with its own. No value is
// logged.
func overlayLLMMetadataSecrets(crypto CryptoServiceInterface, llmID uint, metadata models.JSONMap, column database.JSON) models.JSONMap {
	encrypted, err := encryptedLLMMetadataSecrets(column)
	if err != nil {
		log.Error().Err(err).Uint("llm_id", llmID).Msg("Failed to decode LLM metadata credentials; they are refused")
	}
	if len(encrypted) == 0 && err == nil {
		return metadata
	}
	out := make(models.JSONMap, len(metadata)+len(encrypted))
	for k, v := range metadata {
		out[k] = v
	}
	if err != nil {
		// Whatever the plaintext holds for a credential key is not to be
		// trusted as one either.
		for k := range out {
			if models.IsCredentialMetadataKey(k) {
				out[k] = secrets.RefusedCredential
			}
		}
		return out
	}
	for k, ciphertext := range encrypted {
		value := secrets.RefusedCredential
		if ciphertext != "" {
			if plain, err := crypto.Decrypt(ciphertext); err != nil {
				log.Error().Err(err).Uint("llm_id", llmID).Str("metadata_key", k).Msg("Failed to decrypt LLM metadata credential; it is refused")
			} else if plain != "" {
				value = plain
			}
		}
		out[k] = value
	}
	return out
}

// splitLLMMetadataSecrets is a standalone edge's write of metadata: the
// metadata to store, with secrets.RefusedCredential for each credential
// value, and the column with those values encrypted. A credential sent back
// as models.CredentialRedacted (what the admin API shows) keeps the value
// existing holds. Empty values and references are not credentials and stay
// as written.
func splitLLMMetadataSecrets(crypto CryptoServiceInterface, metadata map[string]interface{}, existing database.JSON) (database.JSON, database.JSON, error) {
	if metadata == nil {
		return nil, nil, nil
	}
	previous, err := encryptedLLMMetadataSecrets(existing)
	if err != nil {
		previous = nil // unreadable: a redacted value then has nothing to keep
	}
	plain := make(map[string]interface{}, len(metadata))
	encrypted := map[string]string{}
	for k, v := range metadata {
		s, isString := v.(string)
		if !isString || !models.IsCredentialMetadataKey(k) || s == "" || secrets.IsReference(s) {
			plain[k] = v
			continue
		}
		if s == models.CredentialRedacted {
			if ciphertext, ok := previous[k]; ok {
				encrypted[k] = ciphertext
				plain[k] = secrets.RefusedCredential
			}
			// Nothing to keep: the key is dropped rather than stored as
			// the placeholder text.
			continue
		}
		ciphertext, err := crypto.Encrypt(s)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to encrypt metadata credential %q: %w", k, err)
		}
		encrypted[k] = ciphertext
		plain[k] = secrets.RefusedCredential
	}
	plainBytes, err := json.Marshal(plain)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if len(encrypted) == 0 {
		return plainBytes, nil, nil
	}
	encryptedBytes, err := json.Marshal(encrypted)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal metadata credentials: %w", err)
	}
	return plainBytes, encryptedBytes, nil
}

// redactLLMMetadataCredentials is an LLM as the admin API shows it: the
// values of credential metadata keys are models.CredentialRedacted, whether
// stored encrypted or (written before 2.3) in Metadata itself.
func redactLLMMetadataCredentials(llm database.LLM) database.LLM {
	if len(llm.Metadata) == 0 {
		return llm
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(llm.Metadata, &metadata); err != nil {
		llm.Metadata = nil
		return llm
	}
	for k, v := range metadata {
		if s, ok := v.(string); ok && s != "" && !secrets.IsReference(s) && models.IsCredentialMetadataKey(k) {
			metadata[k] = models.CredentialRedacted
		}
	}
	if b, err := json.Marshal(metadata); err == nil {
		llm.Metadata = b
	} else {
		llm.Metadata = nil
	}
	return llm
}

// RedactLLMForResponse is llm as the management API returns it, with its
// metadata credentials redacted.
func RedactLLMForResponse(llm *database.LLM) *database.LLM {
	if llm == nil {
		return nil
	}
	redacted := redactLLMMetadataCredentials(*llm)
	return &redacted
}
