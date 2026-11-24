package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	keychainService          = "agentflow-studio"
	darwinSecretB64Prefix    = "af-b64:"
	darwinMaxSecurityLineLen = 4095
)

var errKeychainUnavailable = errors.New("os keychain unavailable")

// secretStore persists remembered runtime tokens in the OS credential store
// instead of plaintext localStorage.
type secretStore interface {
	Available() bool
	Backend() string
	Get(account string) (string, bool, error)
	Set(account, secret string) error
	Delete(account string) error
}

func newSecretStore() secretStore {
	return newPlatformSecretStore()
}

func validateAccount(account string) error {
	if account == "" {
		return errors.New("account is required")
	}
	if len(account) > 512 {
		return errors.New("account name is too long")
	}
	if strings.ContainsAny(account, "\r\n\x00") {
		return errors.New("account name contains invalid control characters")
	}
	return nil
}

func escapeSecurityArg(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// encodeDarwinSecret wraps secrets as printable ASCII base64 so macOS
// `security find-generic-password -w` never hex-encodes non-ASCII UTF-8 bytes
// on read.
func encodeDarwinSecret(secret string) string {
	return darwinSecretB64Prefix + base64.StdEncoding.EncodeToString([]byte(secret))
}

func decodeDarwinSecret(raw string) (string, error) {
	trimmed := strings.TrimRight(raw, "\r\n")
	if strings.HasPrefix(trimmed, darwinSecretB64Prefix) {
		dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(trimmed, darwinSecretB64Prefix))
		if err != nil {
			return "", fmt.Errorf("decode macOS keychain secret: %w", err)
		}
		return string(dec), nil
	}
	return trimmed, nil
}

func buildDarwinSetCommand(service, account, secret string) (string, error) {
	line := fmt.Sprintf(
		"add-generic-password -U -s %q -a \"%s\" -w \"%s\"\n",
		service,
		escapeSecurityArg(account),
		encodeDarwinSecret(secret),
	)
	if len(line) > darwinMaxSecurityLineLen {
		return "", fmt.Errorf("secret exceeds macOS security -i command length limit (%d > %d)", len(line), darwinMaxSecurityLineLen)
	}
	return line, nil
}
