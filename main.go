package main

import (
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/peer"
	"eeptorrent/lib/tracker"
	"fmt"
	"github.com/sirupsen/logrus"
	"sync"
	"time"

	"github.com/go-i2p/go-i2p-bt/metainfo"
	"os"
)

var log = logrus.New()

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetLevel(logrus.DebugLevel)
}

/* //Trackers
String convertedurl = url.replace("ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p", "tracker2.postman.i2p")
.replace("w7tpbzncbcocrqtwwm3nezhnnsw4ozadvi2hmvzdhrqzfxfum7wa.b32.i2p", "opentracker.dg2.i2p")
.replace("afuuortfaqejkesne272krqvmafn65mhls6nvcwv3t7l2ic2p4kq.b32.i2p", "lyoko.i2p")
.replace("s5ikrdyjwbcgxmqetxb3nyheizftms7euacuub2hic7defkh3xhq.b32.i2p", "tracker.thebland.i2p")
.replace("nfrjvknwcw47itotkzmk6mdlxmxfxsxhbhlr5ozhlsuavcogv4hq.b32.i2p", "torrfreedom.i2p")
.replace("http://", "");

*/
/*
p6zlufbvhcn426427wiaylzejdwg4hbdlrccst6owijhlvgalb7a.b32.i2p
cpfrxck5c4stxqrrjsp5syqvhfbtmc2jebyiyuv4hwjnhbxopuyq.b32.i2p
6p225yhqnr2t3kjdh5vy3h2bsv5unfip4777dqfk7qv2ihluf6va.b32.i2p+
cofho7nrtwu47mzejuwk6aszk7zj7aox6b5v2ybdhh5ykrz64jka.b32.i2p+

*/

func main() {
	// init download stats
	stats := download.NewDownloadStats()

	//init sam
	err := i2p.InitSAM()
	if err != nil {
		panic(err)
	}
	defer i2p.CloseSAM()

	//http://tracker2.postman.i2p/announce.php
	//ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p <-> tracker2.postman.i2p
	// Load the torrent file
	mi, err := metainfo.LoadFromFile("torrent-i2pify+script.torrent")
	if err != nil {
		log.Fatalf("Failed to load torrent: %v", err)
	}

	info, err := mi.Info()
	if err != nil {
		log.Fatalf("Failed to parse torrent info: %v", err)
	}

	fmt.Printf("Downloading: %s\n", info.Name)
	fmt.Printf("Info Hash: %s\n", mi.InfoHash().HexString())
	fmt.Printf("Total size: %d bytes\n", info.TotalLength())
	fmt.Printf("Piece length: %d bytes\n", info.PieceLength)
	fmt.Printf("Pieces: %d\n", len(info.Pieces))

	// Initialize the file writer
	/*
		outputFile := info.Name
		mode := os.FileMode(0644)
		file, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer file.Close()

	*/
	var outputPath string
	var mode os.FileMode
	if len(info.Files) == 0 {
		// Single-file torrent
		outputPath = info.Name
		mode = 0644
	} else {
		// Multi-file torrent
		outputPath = info.Name
		mode = 0755
		// Create the directory if it doesn't exist
		err := os.MkdirAll(outputPath, mode)
		if err != nil && !os.IsExist(err) {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)
	dm := download.NewDownloadManager(writer, info.TotalLength(), info.PieceLength, len(info.Pieces))
	progressTicker := time.NewTicker(5 * time.Second)
	go func() {
		for range progressTicker.C {
			dm.LogProgress()
		}
	}()
	defer progressTicker.Stop()

	peers, err := tracker.GetPeersFromSimpTracker(&mi)
	if err != nil {
		log.Fatalf("Failed to get peers from tracker: %v", err)
	}
	var wg sync.WaitGroup

	for i, peerHash := range peers {
		wg.Add(1)
		go func(peerHash []byte, index int) {
			defer wg.Done()
			stats.ConnectionStarted()
			defer stats.ConnectionEnded()
			peer.ConnectToPeer(peerHash, index, &mi, dm)
		}(peerHash, i)
	}

	wg.Wait()

	if dm.IsFinished() {
		log.WithFields(logrus.Fields{
			"total_downloaded": dm.Downloaded,
			"elapsed_time":     time.Since(stats.StartTime),
			"avg_speed_kBps":   float64(dm.Downloaded) / time.Since(stats.StartTime).Seconds() / 1024,
			"peak_speed_kBps":  stats.PeakSpeed / 1024,
		}).Info("Download completed?")
	} else {
		fmt.Println("Download incomplete")
	}
}

/*
	func requestNextBlock(pc *pp.PeerConn, dm *downloadManager) error {
		if dm.plength <= 0 {
			dm.pindex++
			if dm.IsFinished() {
				return nil
			}

			dm.poffset = 0
			dm.plength = dm.writer.Info().Piece(int(dm.pindex)).Length()
		}

		// Check if peer has the piece
		if !pc.BitField.IsSet(dm.pindex) {
			// Try next piece
			dm.pindex++
			dm.poffset = 0
			return nil
		}

		// Calculate block size
		length := uint32(BlockSize)
		if length > uint32(dm.plength) {
			length = uint32(dm.plength)
		}

		// Request the block
		err := pc.SendRequest(dm.pindex, dm.poffset, length)
		if err == nil {
			dm.doing = true
			fmt.Printf("\rRequesting piece %d (%d/%d), offset %d, length %d",
				dm.pindex, dm.pindex+1, dm.writer.Info().CountPieces(), dm.poffset, length)
		}
		return err
	}
*/
