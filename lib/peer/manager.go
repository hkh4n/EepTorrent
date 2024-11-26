package peer

import (
	"eeptorrent/lib/download"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// PeerManager handles peer choking/unchoking and optimistic unchoking
type PeerManager struct {
	Mu              sync.Mutex
	Peers           map[*pp.PeerConn]*PeerState
	downloadManager *download.DownloadManager
	lastUnchoking   time.Time
	peerStats       map[*pp.PeerConn]*PeerStats
	shutdownChan    chan struct{}
	wg              sync.WaitGroup
}

type PeerStats struct {
	bytesDownloaded int64
	bytesUploaded   int64
	lastDownload    time.Time
	lastUpload      time.Time
	downloadRate    float64
	uploadRate      float64
}

func NewPeerManager(dm *download.DownloadManager) *PeerManager {
	pm := &PeerManager{
		Peers:           make(map[*pp.PeerConn]*PeerState),
		downloadManager: dm,
		peerStats:       make(map[*pp.PeerConn]*PeerStats),
		shutdownChan:    make(chan struct{}),
	}

	// Start periodic unchoking algorithm
	go pm.runChokingAlgorithm()
	return pm
}
func (pm *PeerManager) Shutdown() {
	close(pm.shutdownChan)
	pm.wg.Wait()
}
func (pm *PeerManager) runChokingAlgorithm() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pm.Mu.Lock()
		now := time.Now()

		// Sort peers by download rate
		var peerList []*pp.PeerConn
		for peer := range pm.Peers {
			peerList = append(peerList, peer)
		}

		sort.Slice(peerList, func(i, j int) bool {
			statsI := pm.peerStats[peerList[i]]
			statsJ := pm.peerStats[peerList[j]]
			return statsI.downloadRate > statsJ.downloadRate
		})

		// Unchoke top 4 downloading peers
		for i, peer := range peerList {
			if i < 4 {
				if peer.Choked {
					peer.SendUnchoke()
				}
			} else {
				if !peer.Choked { // Choked vs PeerChoked
					peer.SendChoke()
				}
			}
		}

		// Optimistic unchoking: randomly unchoke one additional peer every 30 seconds
		if now.Sub(pm.lastUnchoking) > 30*time.Second {
			if len(peerList) > 4 {
				randomIndex := 4 + rand.Intn(len(peerList)-4)
				peer := peerList[randomIndex]
				if peer.Choked {
					peer.SendUnchoke()
				}
			}
			pm.lastUnchoking = now
		}

		pm.Mu.Unlock()
	}
}

func (pm *PeerManager) UpdatePeerStats(pc *pp.PeerConn, bytesDownloaded, bytesUploaded int64) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	stats, exists := pm.peerStats[pc]
	if !exists {
		stats = &PeerStats{}
		pm.peerStats[pc] = stats
	}

	now := time.Now()

	// Update download stats
	if bytesDownloaded > 0 {
		duration := now.Sub(stats.lastDownload).Seconds()
		if duration > 0 {
			stats.downloadRate = float64(bytesDownloaded) / duration
		}
		stats.bytesDownloaded += bytesDownloaded
		stats.lastDownload = now
	}

	// Update upload stats
	if bytesUploaded > 0 {
		duration := now.Sub(stats.lastUpload).Seconds()
		if duration > 0 {
			stats.uploadRate = float64(bytesUploaded) / duration
		}
		stats.bytesUploaded += bytesUploaded
		stats.lastUpload = now
	}
}

// Call this when receiving data from a peer
func (pm *PeerManager) OnDownload(pc *pp.PeerConn, bytes int64) {
	// If we're downloading from a peer, express interest and try to reciprocate
	if !pc.Interested {
		pc.SendInterested()
	}
	pm.UpdatePeerStats(pc, bytes, 0)
}

// Call this when uploading data to a peer
func (pm *PeerManager) OnUpload(pc *pp.PeerConn, bytes int64) {
	pm.UpdatePeerStats(pc, 0, bytes)
}

func (pm *PeerManager) OnPeerInterested(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	pc.PeerInterested = true

	if pm.downloadManager.IsFinished() {
		if pc.PeerChoked {
			pc.SendUnchoke()
			log.WithField("peer", pc.RemoteAddr().String()).Info("Unchoked interested peer while seeding")
		}
	}
}

func (pm *PeerManager) OnPeerNotInterested(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	pc.PeerInterested = false

	if !pc.PeerChoked {
		pc.SendChoke()
		log.WithField("peer", pc.RemoteAddr().String()).Info("Choked not interested peer")
	}
}

func (pm *PeerManager) HandleSeeding() {
	// Modify the choking algorithm for seeding mode
	if !pm.downloadManager.IsFinished() {
		return
	}

	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	// Sort peers by upload rate
	var peerList []*pp.PeerConn
	for peer := range pm.Peers {
		if peer.PeerInterested {
			peerList = append(peerList, peer)
		}
	}

	sort.Slice(peerList, func(i, j int) bool {
		statsI := pm.peerStats[peerList[i]]
		statsJ := pm.peerStats[peerList[j]]
		return statsI.uploadRate > statsJ.uploadRate
	})

	// When seeding, we can be more generous with unchoking
	maxUnchoked := 8 // Allow more concurrent uploads when seeding

	// Unchoke the top uploaders
	for i, peer := range peerList {
		if i < maxUnchoked {
			if peer.PeerChoked {
				peer.SendUnchoke()
				log.WithField("peer", peer.RemoteAddr().String()).Info("Unchoked high-performing peer while seeding")
			}
		} else {
			if !peer.PeerChoked {
				peer.SendChoke()
			}
		}
	}
}
