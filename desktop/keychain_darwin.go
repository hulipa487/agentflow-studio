//go:build darwin

package main

import (
	"bytes"
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

func (s darwinSecretStore) Get(account string) (string, bool, error) {
	if err := validateAccount(account); err != nil {
		return "", false, err
	}
	if !s.Available() {
		return "", false, errKeychainUnavailable
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(macSecurityBin, "find-generic-password", "-s", keychainService, "-a", account, "-w")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isSecItemNotFound(err) {
			return "", false, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", false, fmt.Errorf("%w: %s", err, msg)
		}
		return "", false, err
	}
	secret, err := decodeDarwinSecret(stdout.String())
	if err != nil {
		return "", false, err
	}
	return secret, true, nil
}

func (s darwinSecretStore) Set(account, secret string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	// Pass the secret over stdin (`security -i`) encoded as printable-ASCII base64
	// so it never appears in argv, survives shell quoting, and avoids `security -w`
	// auto-hex-encoding non-ASCII UTF-8 bytes on read.
	line, err := buildDarwinSetCommand(keychainService, account, secret)
	if err != nil {
		return err
	}
	cmd := exec.Command(macSecurityBin, "-i")
	cmd.Stdin = strings.NewReader(line)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	msg := strings.TrimSpace(stderr.String())
	if err != nil {
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	if strings.Contains(msg, "security: ") {
		return errors.New(msg)
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
