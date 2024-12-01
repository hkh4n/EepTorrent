package util

/*
A cross-platform I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

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
