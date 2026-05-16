package main

import (
	"errors"
	"strings"
	"testing"
)

// --- formatTestStatus ---

func TestFormatTestStatusSuccess(t *testing.T) {
	msg, ok := formatTestStatus(nil)
	if !ok {
		t.Error("expected success=true for nil error")
	}
	if msg != "Connection successful" {
		t.Errorf("unexpected success message: %q", msg)
	}
}

func TestFormatTestStatusHostKeyChanged(t *testing.T) {
	err := errors.New("WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!")
	msg, ok := formatTestStatus(err)
	if ok {
		t.Error("expected success=false for host key change")
	}
	if !strings.Contains(msg, "Host key mismatch") {
		t.Errorf("expected host key mismatch message, got %q", msg)
	}
}

func TestFormatTestStatusRevokedKey(t *testing.T) {
	err := errors.New("@@@@@@@@@@@@@@@@@@@@@@@\nREVOKED HOST KEY @@@@@")
	msg, ok := formatTestStatus(err)
	if ok {
		t.Error("expected success=false for revoked key")
	}
	if !strings.Contains(strings.ToLower(msg), "revoked") {
		t.Errorf("expected revoked message, got %q", msg)
	}
}

func TestFormatTestStatusUnknownHostKey(t *testing.T) {
	cases := []string{
		"Host key verification failed.",
		"The authenticity of host 'example.com' can't be established.",
		"No RSA host key is known for example.com",
	}
	for _, errMsg := range cases {
		msg, ok := formatTestStatus(errors.New(errMsg))
		if ok {
			t.Errorf("expected failure for %q", errMsg)
		}
		if !strings.Contains(msg, "Host key is unknown") {
			t.Errorf("expected 'Host key is unknown' for input %q, got %q", errMsg, msg)
		}
	}
}

func TestFormatTestStatusGenericError(t *testing.T) {
	errMsg := "Connection refused"
	msg, ok := formatTestStatus(errors.New(errMsg))
	if ok {
		t.Error("expected success=false for generic error")
	}
	if msg != errMsg {
		t.Errorf("expected passthrough of error message, got %q", msg)
	}
}

// --- buildSSHCommand ---

func TestBuildSSHCommandNoPassword(t *testing.T) {
	args := []string{"-l", "root", "example.com"}
	binary, got, env, ok := buildSSHCommand("", args)
	if binary != "ssh" {
		t.Errorf("expected binary=ssh, got %q", binary)
	}
	if len(got) != len(args) {
		t.Errorf("expected args unchanged, got %v", got)
	}
	if len(env) != 0 {
		t.Errorf("expected no extra env, got %v", env)
	}
	if !ok {
		t.Error("expected ok=true when no password")
	}
}

func TestBuildSSHCommandNoSshpass(t *testing.T) {
	// Override PATH so sshpass cannot be found.
	t.Setenv("PATH", t.TempDir())
	args := []string{"example.com"}
	binary, got, _, ok := buildSSHCommand("secret", args)
	if ok {
		t.Error("expected ok=false when sshpass not installed")
	}
	if binary != "ssh" {
		t.Errorf("expected fallback binary=ssh, got %q", binary)
	}
	if len(got) != len(args) {
		t.Errorf("expected original args returned, got %v", got)
	}
}
