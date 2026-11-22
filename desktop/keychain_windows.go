//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errCredNotFound         = syscall.Errno(1168) // ERROR_NOT_FOUND
)

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsSecretStore struct{}

func newPlatformSecretStore() secretStore {
	return windowsSecretStore{}
}

func (windowsSecretStore) Available() bool {
	return advapi32.Load() == nil &&
		procCredReadW.Find() == nil &&
		procCredWriteW.Find() == nil &&
		procCredDeleteW.Find() == nil &&
		procCredFree.Find() == nil
}

func (windowsSecretStore) Backend() string {
	return "windows-credman"
}

func credTargetName(account string) string {
	return "AgentFlow Studio/" + account
}

func (s windowsSecretStore) Get(account string) (string, bool, error) {
	if err := validateAccount(account); err != nil {
		return "", false, err
	}
	if !s.Available() {
		return "", false, errKeychainUnavailable
	}
	targetPtr, err := syscall.UTF16PtrFromString(credTargetName(account))
	if err != nil {
		return "", false, err
	}
	var cred *winCredential
	r, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&cred)),
	)
	runtime.KeepAlive(targetPtr)
	if r == 0 {
		if errors.Is(callErr, errCredNotFound) {
			return "", false, nil
		}
		return "", false, callErr
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))

	if cred == nil || cred.CredentialBlob == nil || cred.CredentialBlobSize == 0 {
		return "", true, nil
	}
	blob := unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)
	return string(blob), true, nil
}

func (s windowsSecretStore) Set(account, secret string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	targetPtr, err := syscall.UTF16PtrFromString(credTargetName(account))
	if err != nil {
		return err
	}
	commentPtr, err := syscall.UTF16PtrFromString("AgentFlow Studio admin token")
	if err != nil {
		return err
	}
	userPtr, err := syscall.UTF16PtrFromString(account)
	if err != nil {
		return err
	}

	blob := []byte(secret)
	var blobPtr *byte
	if len(blob) > 0 {
		blobPtr = &blob[0]
	}

	cred := winCredential{
		Type:               credTypeGeneric,
		TargetName:         targetPtr,
		Comment:            commentPtr,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     blobPtr,
		Persist:            credPersistLocalMachine,
		UserName:           userPtr,
	}

	r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	runtime.KeepAlive(targetPtr)
	runtime.KeepAlive(commentPtr)
	runtime.KeepAlive(userPtr)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(&cred)
	if r == 0 {
		return callErr
	}
	return nil
}

func (s windowsSecretStore) Delete(account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if !s.Available() {
		return errKeychainUnavailable
	}
	targetPtr, err := syscall.UTF16PtrFromString(credTargetName(account))
	if err != nil {
		return err
	}
	r, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credTypeGeneric,
		0,
	)
	runtime.KeepAlive(targetPtr)
	if r == 0 {
		if errors.Is(callErr, errCredNotFound) {
			return nil
		}
		return callErr
	}
	return nil
}
