//go:build darwin

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const macSecurityBin = "/usr/bin/security"

type darwinSecretStore struct{}

func newPlatformSecretStore() secretStore {
	return darwinSecretStore{}
}

func (darwinSecretStore) Available() bool {
	_, err := exec.LookPath(macSecurityBin)
	return err == nil
}

func (darwinSecretStore) Backend() string {
	return "macos-keychain"
}

func isSecItemNotFound(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 44
}

func escapeSecurityArg(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func (s darwinSecretStore) Get(account string) (string, bool, error) {
	if err := validateAccount(account); err != nil {
		return "", false, err
	}
	if !s.Available() {
		return "", false, errKeychainUnavailable
	}
	out, err := exec.Command(macSecurityBin, "find-generic-password", "-s", keychainService, "-a", account, "-w").Output()
	if err != nil {
		if isSecItemNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}

func (s darwinSecretStore) Set(account, secret string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	// Pass the secret over stdin (`security -i`) as hex so it never appears in
	// argv or trips shell quoting.
	line := fmt.Sprintf(
		"add-generic-password -U -s %q -a \"%s\" -X %s\n",
		keychainService,
		escapeSecurityArg(account),
		hex.EncodeToString([]byte(secret)),
	)
	cmd := exec.Command(macSecurityBin, "-i")
	cmd.Stdin = strings.NewReader(line)
	var stderr bytes.Buffer
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

func (s darwinSecretStore) Delete(account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	var stderr bytes.Buffer
	cmd := exec.Command(macSecurityBin, "delete-generic-password", "-s", keychainService, "-a", account)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isSecItemNotFound(err) {
			return nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
