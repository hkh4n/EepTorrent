package main

import (
	"fmt"
	"github.com/anacrolix/torrent/metainfo"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run analyze_torrent.go /path/to/torrent/file")
		os.Exit(1)
	}

	torrentPath := os.Args[1]

	// Load the torrent file
	mi, err := metainfo.LoadFromFile(torrentPath)
	if err != nil {
		log.Fatalf("Failed to load torrent file: %v", err)
	}

	// Unmarshal the info dictionary
	info, err := mi.UnmarshalInfo()
	if err != nil {
		log.Fatalf("Failed to unmarshal info dictionary: %v", err)
	}

	// Get the concatenated piece hashes
	pieces := info.Pieces
	pieceLength := 20 // SHA-1 hash size in bytes
	totalPieces := len(pieces) / pieceLength

	fmt.Printf("Torrent Name: %s\n", info.Name)
	fmt.Printf("Total Pieces: %d\n", totalPieces)
	fmt.Printf("Piece Length: %d bytes\n", info.PieceLength)
	fmt.Printf("Total Size: %d bytes\n", info.TotalLength())

	// Iterate over each piece and print the hash
	for i := 0; i < totalPieces; i++ {
		start := i * pieceLength
		end := start + pieceLength
		pieceHash := pieces[start:end]
		fmt.Printf("Piece %d hash: %x\n", i, pieceHash)
	}
}
