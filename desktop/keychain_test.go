package main

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestSecretStoreRoundTrip(t *testing.T) {
	store := newSecretStore()
	if !store.Available() {
		if runtime.GOOS == "windows" {
			t.Fatalf("expected Windows Credential Manager to be available, backend=%q", store.Backend())
		}
		t.Skipf("os keychain unavailable in this environment (backend=%q)", store.Backend())
	}

	account := fmt.Sprintf("af-studio-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = store.Delete(account)
	})

	if _, found, err := store.Get(account); err != nil || found {
		t.Fatalf("initial Get = (found=%v, err=%v), want (false, nil)", found, err)
	}

	const firstSecret = "tok-initial-0123456789-abcdef"
	if err := store.Set(account, firstSecret); err != nil {
		t.Fatalf("Set(firstSecret) failed: %v", err)
	}
	got, found, err := store.Get(account)
	if err != nil || !found || got != firstSecret {
		t.Fatalf("Get after Set = (%q, found=%v, err=%v), want (%q, true, nil)", got, found, err, firstSecret)
	}

	const rotatedSecret = "tok-rotated-!@#$%^&*()-unicode-你好"
	if err := store.Set(account, rotatedSecret); err != nil {
		t.Fatalf("Set(rotatedSecret) failed: %v", err)
	}
	got, found, err = store.Get(account)
	if err != nil || !found || got != rotatedSecret {
		t.Fatalf("Get after overwrite = (%q, found=%v, err=%v), want (%q, true, nil)", got, found, err, rotatedSecret)
	}

	if err := store.Delete(account); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, found, err := store.Get(account); err != nil || found {
		t.Fatalf("Get after Delete = (found=%v, err=%v), want (false, nil)", found, err)
	}
	if err := store.Delete(account); err != nil {
		t.Fatalf("idempotent second Delete failed: %v", err)
	}
}
