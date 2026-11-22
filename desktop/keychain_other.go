//go:build !windows && !darwin && !linux

package main

type unsupportedSecretStore struct{}

func newPlatformSecretStore() secretStore {
	return unsupportedSecretStore{}
}

func (unsupportedSecretStore) Available() bool                       { return false }
func (unsupportedSecretStore) Backend() string                       { return "none" }
func (unsupportedSecretStore) Get(string) (string, bool, error)      { return "", false, errKeychainUnavailable }
func (unsupportedSecretStore) Set(string, string) error              { return errKeychainUnavailable }
func (unsupportedSecretStore) Delete(string) error                   { return errKeychainUnavailable }
