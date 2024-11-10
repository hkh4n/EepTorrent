package main

import (
	"fmt"
	"github.com/eyedeekay/sam3"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
)

// Helper function to generate a random 20-byte hash for testing
func generateRandomHash() []byte {
	hash := make([]byte, 20)
	rand.Seed(time.Now().UnixNano())
	rand.Read(hash)
	return hash
}

// http://zzz.i2p/files/trackers.html
// tracker2.postman.i2p/announce.php
func main() {
	// Connect to SAM
	sam, err := sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		panic(err)
	}
	defer sam.Close()

	// Generate keys
	keys, err := sam.NewKeys()
	if err != nil {
		panic(err)
	}

	sessionName := fmt.Sprintf("postman-%d", os.Getpid())
	session, err := sam.NewPrimarySession(sessionName, keys, sam3.Options_Default)
	if err != nil {
		panic(err)
	}
	defer session.Close()

	trackerAddr, err := sam.Lookup("tracker2.postman.i2p")
	if err != nil {
		log.Fatalf("Failed to look up tracker address: %v", err)
	}

	// Create a UDP connection to the tracker's destination and port
	conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", trackerAddr.Base32(), 6881))
	if err != nil {
		log.Fatalf("Failed to establish UDP connection: %v", err)
	}
	defer conn.Close()
	// Create a random info hash and peer ID for testing
	//infoHash := metainfo.NewRandomHash()
	//peerID := metainfo.NewRandomHash()
	// Generate a random info hash and peer ID for the announce request
	infoHash := generateRandomHash()
	peerID := generateRandomHash()

	// Construct the UDP announce payload similar to i2psnark
	// Here, we create a minimal UDP announce packet
	payload := make([]byte, 98)
	copy(payload[16:36], infoHash) // Info hash
	copy(payload[36:56], peerID)   // Peer ID

	// Set the event type (e.g., 2 for started)
	payload[80] = 2

	// Send the announce request to the tracker
	_, err = conn.Write(payload)
	if err != nil {
		log.Fatalf("Failed to send announce packet: %v", err)
	}

	// Read the response from the tracker
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	// Print the response from the tracker
	fmt.Println("Response from tracker:", string(buf[:n]))
}
