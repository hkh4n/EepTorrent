package util

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
	_, err := rand.Read(peerId[:])
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}
	return peerId
}

func GeneratePeerId() string {
	// Client identifier (8 bytes)
	clientId := "-ET0001-"
	// Generate 12 random bytes for uniqueness
	randomBytes := make([]byte, 12)
	_, err := rand.Read(randomBytes)
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}

	// Combine them into a 20-byte peer ID
	return clientId + string(randomBytes)
}
