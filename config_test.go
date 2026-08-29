// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestLoadConfigRejectsMissingOrInvalidMasterKey(t *testing.T) {
	t.Setenv("UNITED_STATE_MASTER_KEY", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want error for a missing master key")
	}

	t.Setenv("UNITED_STATE_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 31)))
	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want error for a non-32-byte master key")
	}
}

func TestLoadConfigDecodes32ByteMasterKey(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	t.Setenv("UNITED_STATE_MASTER_KEY", base64.StdEncoding.EncodeToString(key))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !bytes.Equal(cfg.StateMasterKey, key) {
		t.Errorf("LoadConfig().StateMasterKey = %x, want %x", cfg.StateMasterKey, key)
	}
}

func TestNewAppConstructsPocketBaseApplication(t *testing.T) {
	app := NewApp(Config{StateMasterKey: bytes.Repeat([]byte{1}, 32)})
	if app == nil {
		t.Fatal("NewApp() = nil, want PocketBase application")
	}
}
