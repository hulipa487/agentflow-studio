//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type linuxSecretStore struct{}

func newPlatformSecretStore() secretStore {
	return linuxSecretStore{}
}

func (linuxSecretStore) Available() bool {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return false
	}
	return os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" || os.Getenv("XDG_RUNTIME_DIR") != ""
}

func (s linuxSecretStore) Backend() string {
	if s.Available() {
		return "linux-secret-service"
	}
	return "none"
}

func (s linuxSecretStore) Get(account string) (string, bool, error) {
	if err := validateAccount(account); err != nil {
		return "", false, err
	}
	if !s.Available() {
		return "", false, errKeychainUnavailable
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("secret-tool", "lookup", "service", keychainService, "account", account)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && strings.TrimSpace(stderr.String()) == "" {
			return "", false, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", false, fmt.Errorf("%w: %s", err, msg)
		}
		return "", false, err
	}
	return strings.TrimRight(stdout.String(), "\r\n"), true, nil
}

func (s linuxSecretStore) Set(account, secret string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	var stderr bytes.Buffer
	cmd := exec.Command(
		"secret-tool",
		"store",
		"--label=AgentFlow Studio ("+account+")",
		"service", keychainService,
		"account", account,
	)
	cmd.Stdin = strings.NewReader(secret)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func (s linuxSecretStore) Delete(account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	var stderr bytes.Buffer
	cmd := exec.Command("secret-tool", "clear", "service", keychainService, "account", account)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
