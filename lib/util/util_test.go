package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanBase32Address(t *testing.T) {
	addr := "abcdef123456===="
	cleaned := CleanBase32Address(addr)
	expected := "abcdef123456.b32.i2p"
	assert.Equal(t, expected, cleaned, "CleanBase32Address should remove padding and append suffix")
}

func TestGeneratePeerIdMeta(t *testing.T) {
	peerId := GeneratePeerIdMeta()
	assert.Len(t, peerId, 20, "Peer ID should be 20 bytes long")
	assert.Equal(t, "-ET0000-", string(peerId[:8]), "Peer ID should start with '-ET0000-'")
}

func TestGeneratePeerId(t *testing.T) {
	peerId := GeneratePeerId()
	assert.Len(t, peerId, 20, "Peer ID should be 20 characters long")
	assert.True(t, len(peerId) >= 8, "Peer ID should contain the client identifier prefix")
	assert.Equal(t, "-ET0000-", peerId[:8], "Peer ID should start with '-ET0000-'")
}
