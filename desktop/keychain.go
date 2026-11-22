package main

import (
	"errors"
	"strings"
)

const keychainService = "agentflow-studio"

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
