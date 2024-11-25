package tracker

import (
	"eeptorrent/lib/i2p"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/sirupsen/logrus"
	"os"
	"testing"
	"time"
)

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func TestTrackers(t *testing.T) {
	// Setup logging
	logrus.SetLevel(logrus.DebugLevel)

	torrentFile := "/tmp/test.torrent"

	if !fileExists(torrentFile) {
		t.Skip("No torrent file set for tracker_test go (/tmp/test.torrent), so we're skipping.")
	}

	mi, err := metainfo.LoadFromFile(torrentFile)
	if err != nil {
		t.Fatalf("Failed to load test torrent: %v", err)
	}

	// Initialize SAM connection
	err = i2p.InitSAM(i2p.DefaultSAMConfig())
	if err != nil {
		t.Fatalf("Failed to initialize SAM: %v", err)
	}
	defer i2p.CloseSAM()

	// Define test cases
	trackerTests := []struct {
		name    string
		getFn   func(*metainfo.MetaInfo) ([][]byte, error)
		timeout time.Duration
	}{
		{
			name:    "PostmanTracker",
			getFn:   GetPeersFromPostmanTracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "SimpTracker",
			getFn:   GetPeersFromSimpTracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "Dg2Tracker",
			getFn:   GetPeersFromDg2Tracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "SkankTracker",
			getFn:   GetPeersFromSkankTracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "OmitTracker",
			getFn:   GetPeersFromOmitTracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "6kw6Tracker",
			getFn:   GetPeersFrom6kw6Tracker,
			timeout: 30 * time.Second,
		},
		{
			name:    "EepTorrentTracker",
			getFn:   GetPeersFromEepTorrentTracker,
			timeout: 30 * time.Second,
		},
	}

	for _, tt := range trackerTests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan bool)
			var peers [][]byte
			var testErr error

			go func() {
				peers, testErr = tt.getFn(&mi)
				done <- true
			}()

			select {
			case <-done:
				if testErr != nil {
					t.Errorf("Failed to get peers from %s: %v", tt.name, testErr)
				} else if len(peers) == 0 {
					t.Logf("%s returned no peers", tt.name)
				} else {
					t.Logf("%s successfully returned %d peers", tt.name, len(peers))

					for i := 0; i < min(3, len(peers)); i++ {
						t.Logf("Peer %d hash: %x", i, peers[i])
					}
				}
			case <-time.After(tt.timeout):
				t.Errorf("%s test timed out after %v", tt.name, tt.timeout)
			}
		})

		// Add delay between tracker tests to avoid overwhelming I2P
		time.Sleep(2 * time.Second)
	}
}
