package main

import (
	"crypto/sha1"
	"fmt"
	"github.com/zeebo/bencode"
	"os"
)

type TorrentFile struct {
	Announce string `bencode:"announce"`
	Info     struct {
		Name        string `bencode:"name"`
		PieceLength int    `bencode:"piece length"`
		Pieces      string `bencode:"pieces"`
		Length      int    `bencode:"length"`
	} `bencode:"info"`
	InfoBytes []byte `bencode:"-"`
}

func parseTorrentFile(path string) (*TorrentFile, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var torrent TorrentFile
	err = bencode.DecodeBytes(file, &torrent)
	if err != nil {
		return nil, err
	}

	// Extract raw info dict for hash calculation
	var decoded map[string]interface{}
	err = bencode.DecodeBytes(file, &decoded)
	if err != nil {
		return nil, err
	}

	// get the rwa info
	if info, ok := decoded["info"]; ok {
		torrent.InfoBytes, err = bencode.EncodeBytes(info)
		if err != nil {
			return nil, err
		}
	}

	return &torrent, nil
}
func (t *TorrentFile) InfoHash() ([20]byte, error) {
	return sha1.Sum(t.InfoBytes), nil
}
func main() {
	tf, err := parseTorrentFile("../examples/torrent_test.txt.torrent")
	if err != nil {
		fmt.Printf("Error parsing torrent: %v\n", err)
		return
	}

	infoHash, err := tf.InfoHash()
	if err != nil {
		fmt.Printf("error calculating info hash: %v\n", err)
		return
	}

	// Print basic info
	fmt.Printf("Announce: %s\n", tf.Announce)
	fmt.Printf("Name: %s\n", tf.Info.Name)
	fmt.Printf("Piece Length: %d\n", tf.Info.PieceLength)
	fmt.Printf("Total Length: %d\n", tf.Info.Length)
	fmt.Printf("Info Hash: %x\n", infoHash)
	fmt.Printf("Pieces: %d pieces\n", len(tf.Info.Pieces)/20) // Each piece hash is 20 bytes
}
