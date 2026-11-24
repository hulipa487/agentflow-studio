package main

import (
	"fmt"
	"runtime"
	"strings"
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

func TestDarwinSecretCodecAndCommand(t *testing.T) {
	secrets := []string{
		"tok-initial-0123456789-abcdef",
		"tok-rotated-!@#$%^&*()-unicode-你好",
		`quoted "value" with \backslashes\`,
		"line1\nline2\r\n",
		"",
	}
	for _, secret := range secrets {
		enc := encodeDarwinSecret(secret)
		if !strings.HasPrefix(enc, darwinSecretB64Prefix) {
			t.Fatalf("encodeDarwinSecret(%q) = %q, missing prefix %q", secret, enc, darwinSecretB64Prefix)
		}
		for i := 0; i < len(enc); i++ {
			if enc[i] < 0x20 || enc[i] > 0x7e {
				t.Fatalf("encodeDarwinSecret(%q) contains non-printable-ASCII byte %#x at %d", secret, enc[i], i)
			}
		}
		got, err := decodeDarwinSecret(enc + "\n")
		if err != nil || got != secret {
			t.Fatalf("decodeDarwinSecret(enc(%q)) = (%q, %v), want (%q, nil)", secret, got, err, secret)
		}
	}

	legacy, err := decodeDarwinSecret("legacy-plain-token\r\n")
	if err != nil || legacy != "legacy-plain-token" {
		t.Fatalf("legacy decode = (%q, %v), want (%q, nil)", legacy, err, "legacy-plain-token")
	}

	if _, err := decodeDarwinSecret(darwinSecretB64Prefix + "!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for malformed base64 keychain secret")
	}

	cmd, err := buildDarwinSetCommand(keychainService, `acct"with\slash`, "tok-rotated-!@#$%^&*()-unicode-你好")
	if err != nil {
		t.Fatalf("buildDarwinSetCommand failed: %v", err)
	}
	wantAccount := `acct\"with\\slash`
	if !strings.HasPrefix(cmd, `add-generic-password -U -s "agentflow-studio" -a "`+wantAccount+`" -w "`+darwinSecretB64Prefix) || !strings.HasSuffix(cmd, "\"\n") {
		t.Fatalf("unexpected command line: %q", cmd)
	}

	tooLong := strings.Repeat("a", darwinMaxSecurityLineLen)
	if _, err := buildDarwinSetCommand(keychainService, "prod", tooLong); err == nil {
		t.Fatal("expected length limit error from buildDarwinSetCommand")
	}
}
