package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	securitySecretKeyJWT        = "jwt_secret"
	securitySecretKeyAPIKeyHash = "api_key_hash_secret"
	securitySecretReadRetryMax  = 5
	securitySecretReadRetryWait = 10 * time.Millisecond
)

var readRandomBytes = rand.Read

func ensureBootstrapSecrets(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if client == nil {
		return fmt.Errorf("nil ent client")
	}
	if cfg == nil {
		return fmt.Errorf("nil config")
	}

	jwtCreated, err := ensureRuntimeSecret(ctx, client, securitySecretKeyJWT, &cfg.JWT.Secret, "JWT secret", nil)
	if err != nil {
		return err
	}
	if jwtCreated {
		log.Println("Warning: JWT secret auto-generated and persisted to database. Consider rotating to a managed secret for production.")
	}

	apiKeyHashCreated, err := ensureRuntimeSecret(ctx, client, securitySecretKeyAPIKeyHash, &cfg.Security.APIKeyHashSecret, "API key hash secret", config.IsPlaceholderAPIKeyHashSecret)
	if err != nil {
		return err
	}
	if apiKeyHashCreated {
		log.Println("Warning: API key hash secret auto-generated and persisted to database. Consider rotating to a managed secret for production.")
	}
	return nil
}

func ensureRuntimeSecret(ctx context.Context, client *ent.Client, key string, target *string, label string, invalid func(string) bool) (bool, error) {
	if target == nil {
		return false, fmt.Errorf("nil %s target", label)
	}

	configured := strings.TrimSpace(*target)
	if configured != "" && isInvalidRuntimeSecret(invalid, configured) {
		configured = ""
	}
	if configured != "" {
		storedSecret, err := createSecuritySecretIfAbsent(ctx, client, key, configured)
		if err != nil {
			return false, fmt.Errorf("persist %s: %w", label, err)
		}
		if isInvalidRuntimeSecret(invalid, storedSecret) {
			storedSecret, err = replaceSecuritySecretValue(ctx, client, key, storedSecret, configured)
			if err != nil {
				return false, fmt.Errorf("replace invalid %s: %w", label, err)
			}
		}
		if storedSecret != configured {
			log.Printf("Warning: configured %s mismatches persisted value; using persisted secret for cross-instance consistency.", label)
		}
		*target = storedSecret
		return false, nil
	}

	secret, created, err := getOrCreateGeneratedSecuritySecret(ctx, client, key, 32)
	if err != nil {
		return false, fmt.Errorf("ensure %s: %w", label, err)
	}
	if isInvalidRuntimeSecret(invalid, secret) {
		generated, genErr := generateHexSecret(32)
		if genErr != nil {
			return false, fmt.Errorf("generate replacement %s: %w", label, genErr)
		}
		secret, err = replaceSecuritySecretValue(ctx, client, key, secret, generated)
		if err != nil {
			return false, fmt.Errorf("replace invalid %s: %w", label, err)
		}
		created = true
	}
	*target = secret
	return created, nil
}

func isInvalidRuntimeSecret(invalid func(string) bool, value string) bool {
	return invalid != nil && invalid(value)
}

func getOrCreateGeneratedSecuritySecret(ctx context.Context, client *ent.Client, key string, byteLength int) (string, bool, error) {
	existing, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
	if err == nil {
		value := strings.TrimSpace(existing.Value)
		if len([]byte(value)) < 32 {
			return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
		}
		return value, false, nil
	}
	if !ent.IsNotFound(err) {
		return "", false, err
	}

	generated, err := generateHexSecret(byteLength)
	if err != nil {
		return "", false, err
	}

	if err := client.SecuritySecret.Create().
		SetKey(key).
		SetValue(generated).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return "", false, err
		}
	}

	stored, err := querySecuritySecretWithRetry(ctx, client, key)
	if err != nil {
		return "", false, err
	}
	value := strings.TrimSpace(stored.Value)
	if len([]byte(value)) < 32 {
		return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	return value, value == generated, nil
}

func createSecuritySecretIfAbsent(ctx context.Context, client *ent.Client, key, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len([]byte(value)) < 32 {
		return "", fmt.Errorf("secret %q must be at least 32 bytes", key)
	}

	if err := client.SecuritySecret.Create().
		SetKey(key).
		SetValue(value).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		if !isSQLNoRowsError(err) {
			return "", err
		}
	}

	stored, err := querySecuritySecretWithRetry(ctx, client, key)
	if err != nil {
		return "", err
	}
	storedValue := strings.TrimSpace(stored.Value)
	if len([]byte(storedValue)) < 32 {
		return "", fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	return storedValue, nil
}

func replaceSecuritySecretValue(ctx context.Context, client *ent.Client, key, oldValue, newValue string) (string, error) {
	newValue = strings.TrimSpace(newValue)
	if len([]byte(newValue)) < 32 {
		return "", fmt.Errorf("secret %q must be at least 32 bytes", key)
	}

	affected, err := client.SecuritySecret.Update().
		Where(securitysecret.KeyEQ(key), securitysecret.ValueEQ(oldValue)).
		SetValue(newValue).
		Save(ctx)
	if err != nil {
		return "", err
	}
	if affected > 0 {
		return newValue, nil
	}

	stored, err := querySecuritySecretWithRetry(ctx, client, key)
	if err != nil {
		return "", err
	}
	storedValue := strings.TrimSpace(stored.Value)
	if len([]byte(storedValue)) < 32 {
		return "", fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	}
	if storedValue == strings.TrimSpace(oldValue) {
		return "", fmt.Errorf("stored secret %q still has invalid value", key)
	}
	return storedValue, nil
}

func querySecuritySecretWithRetry(ctx context.Context, client *ent.Client, key string) (*ent.SecuritySecret, error) {
	var lastErr error
	for attempt := 0; attempt <= securitySecretReadRetryMax; attempt++ {
		stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
		if err == nil {
			return stored, nil
		}
		if !isSecretNotFoundError(err) {
			return nil, err
		}
		lastErr = err
		if attempt == securitySecretReadRetryMax {
			break
		}

		timer := time.NewTimer(securitySecretReadRetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func isSecretNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return ent.IsNotFound(err) || isSQLNoRowsError(err)
}

func isSQLNoRowsError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set")
}

func generateHexSecret(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}
	buf := make([]byte, byteLength)
	if _, err := readRandomBytes(buf); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
