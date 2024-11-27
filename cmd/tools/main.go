package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"os"
)

func main() {
	// Define subcommands
	analyzeCmd := flag.NewFlagSet("analyze", flag.ExitOnError)
	verifyCmd := flag.NewFlagSet("verify", flag.ExitOnError)

	// Check if a subcommand was provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: EepTorrent-tools <command> [arguments]")
		fmt.Println("\nCommands:")
		fmt.Println("  analyze <torrent-file>    - Analyze a torrent file in detail")
		fmt.Println("  verify <torrent-file>     - Verify downloaded files against torrent")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "analyze":
		analyzeCmd.Parse(os.Args[2:])
		if analyzeCmd.NArg() == 0 {
			fmt.Println("Please specify a torrent file to analyze")
			os.Exit(1)
		}
		analyzeTorrent(analyzeCmd.Arg(0))

	case "verify":
		verifyCmd.Parse(os.Args[2:])
		if verifyCmd.NArg() == 0 {
			fmt.Println("Please specify a torrent file to verify")
			os.Exit(1)
		}
		verifyTorrent(verifyCmd.Arg(0))

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func analyzeTorrent(path string) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		fmt.Printf("Error loading torrent file: %v\n", err)
		os.Exit(1)
	}

	info, err := mi.Info()
	if err != nil {
		fmt.Printf("Error getting torrent info: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Torrent Information ===")
	fmt.Printf("Info Hash: %x\n", mi.InfoHash())
	if mi.Announce != "" {
		fmt.Printf("Primary Tracker: %s\n", mi.Announce)
	}

	if len(mi.AnnounceList) > 0 {
		fmt.Println("\nTracker List:")
		for i, trackers := range mi.AnnounceList {
			fmt.Printf("Tier %d:\n", i+1)
			for _, tracker := range trackers {
				fmt.Printf("  - %s\n", tracker)
			}
		}
	}

	fmt.Printf("\n=== Content Information ===\n")
	fmt.Printf("Piece Length: %d bytes\n", info.PieceLength)
	fmt.Printf("Total Pieces: %d\n", info.CountPieces())
	fmt.Printf("Total Size: %d bytes\n", info.TotalLength())

	if len(info.Files) == 0 {
		fmt.Printf("\nSingle File Torrent:\n")
		fmt.Printf("Name: %s\n", info.Name)
		fmt.Printf("Size: %d bytes\n", info.Length)
	} else {
		fmt.Printf("\nMulti-file Torrent:\n")
		fmt.Printf("Directory Name: %s\n", info.Name)
		fmt.Printf("Number of Files: %d\n", len(info.Files))

		fmt.Printf("\nFile List:\n")
		for i, file := range info.Files {
			//fmt.Printf("\n%d. Path: %s\n", i+1, filepath.Join(file.Path...))
			fmt.Printf("%v:\tSize: %d bytes\n", i, file.Length)
		}
	}

	fmt.Printf("\n=== Piece Hashes ===\n")
	for i := 0; i < info.CountPieces(); i++ {
		pieceHash := info.Pieces[i]
		fmt.Printf("Piece %d:\n", i)
		fmt.Printf("  Hex:      %s\n", hex.EncodeToString(pieceHash.Bytes()))
		fmt.Printf("  Raw Bytes: [")
		for _, b := range pieceHash {
			fmt.Printf("%d ", b)
		}
		fmt.Printf("]\n")
	}
}

func verifyTorrent(path string) {
	// TODO: Implement verification logic
	fmt.Println("Verification feature coming soon!")
}
