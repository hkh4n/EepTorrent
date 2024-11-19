package util

/*
An I2P-only BitTorrent client.
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
	"crypto/rand"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"log"
	"strings"
)

// Helper function to URL-encode binary data as per BitTorrent protocol
func UrlEncodeBytes(b []byte) string {
	var buf strings.Builder
	for _, c := range b {
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			buf.WriteByte(c)
		} else {
			buf.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return buf.String()
}
func CleanBase32Address(addr string) string {
	// Remove any trailing equals signs
	addr = strings.TrimRight(addr, "=")
	return addr + ".b32.i2p"
}

func GeneratePeerIdMeta() metainfo.Hash {
	var peerId metainfo.Hash

	// Define the prefix (8 bytes)
	prefix := "-ET1337-"
	copy(peerId[:8], prefix)

	// Generate 12 random bytes for uniqueness
	_, err := rand.Read(peerId[8:])
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}

	return peerId
}

func GeneratePeerId() string {
	// Client identifier (8 bytes)
	clientId := "-ET1337-"
	// Generate 12 random bytes for uniqueness
	randomBytes := make([]byte, 12)
	_, err := rand.Read(randomBytes)
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}

	// Combine them into a 20-byte peer ID
	return clientId + string(randomBytes)
}
