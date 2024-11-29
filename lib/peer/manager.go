package peer

import (
	"eeptorrent/lib/download"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const (
	maxUnchoked        = 4 // Maximum number of peers to unchoke normally
	optimisticSlots    = 1 // Number of optimistic unchoke slots
	unchokingInterval  = 10 * time.Second
	optimisticInterval = 30 * time.Second
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
	optimisticPeer  *pp.PeerConn
	lastOptimistic  time.Time
}

type PeerStats struct {
	bytesDownloaded    int64
	bytesUploaded      int64
	lastDownload       time.Time
	lastUpload         time.Time
	downloadRate       float64
	uploadRate         float64
	lastUnchokedUs     time.Time
	unchokedUsDuration time.Duration
	lastWeUnchoked     time.Time
	weUnchokedDuration time.Duration
	isInterested       bool
	hasUnchokedUs      bool
}

func NewPeerManager(dm *download.DownloadManager) *PeerManager {
	pm := &PeerManager{
		Peers:           make(map[*pp.PeerConn]*PeerState),
		downloadManager: dm,
		peerStats:       make(map[*pp.PeerConn]*PeerStats),
		shutdownChan:    make(chan struct{}),
		lastUnchoking:   time.Now(),
		lastOptimistic:  time.Now(),
	}

	// Start periodic unchoking algorithm
	go pm.runChokingAlgorithm()
	return pm
}
func (pm *PeerManager) Shutdown() {
	close(pm.shutdownChan)
	pm.wg.Wait()
}
func (pm *PeerManager) calculatePeerScore(peer *pp.PeerConn) float64 {
	stats := pm.peerStats[peer]
	if stats == nil {
		return 0
	}

	var score float64

	// Base score from download and upload rates
	score += stats.downloadRate * 0.7 // Weight download rate more heavily
	score += stats.uploadRate * 0.3   // Also consider upload rate

	// Bonus for peers who have unchoked us
	if stats.hasUnchokedUs {
		score *= 1.2
	}

	// Bonus for long-term relationships
	if stats.unchokedUsDuration > 5*time.Minute {
		score *= 1.1
	}

	// Bonus for reciprocation
	if time.Since(stats.lastWeUnchoked) < 30*time.Second {
		score *= 1.15
	}

	// Penalty for recent failures or non-contribution
	if stats.downloadRate == 0 && time.Since(stats.lastDownload) > 5*time.Minute {
		score *= 0.8
	}

	return score
}

func (pm *PeerManager) runChokingAlgorithm() {
	pm.wg.Add(1)
	defer pm.wg.Done()

	ticker := time.NewTicker(unchokingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.Mu.Lock()
			now := time.Now()

			// Update peer stats and durations
			for peer, stats := range pm.peerStats {
				if stats.hasUnchokedUs {
					stats.unchokedUsDuration = now.Sub(stats.lastUnchokedUs)
				}
				if !peer.PeerChoked {
					stats.weUnchokedDuration = now.Sub(stats.lastWeUnchoked)
				}
			}

			// Get list of interested peers
			var interestedPeers []*pp.PeerConn
			for peer, stats := range pm.peerStats {
				if stats.isInterested {
					interestedPeers = append(interestedPeers, peer)
				}
			}

			// Sort peers by score
			sort.Slice(interestedPeers, func(i, j int) bool {
				scoreI := pm.calculatePeerScore(interestedPeers[i])
				scoreJ := pm.calculatePeerScore(interestedPeers[j])
				return scoreI > scoreJ
			})

			// Handle regular unchoke slots
			unchokedCount := 0
			for _, peer := range interestedPeers {
				if unchokedCount >= maxUnchoked {
					if !peer.PeerChoked {
						peer.SendChoke()
						log.WithField("peer", peer.RemoteAddr().String()).Debug("Choked peer due to slot limit")
					}
					continue
				}

				if pm.optimisticPeer == peer {
					continue // Skip optimistic peer
				}

				if peer.PeerChoked {
					peer.SendUnchoke()
					stats := pm.peerStats[peer]
					stats.lastWeUnchoked = now
					log.WithField("peer", peer.RemoteAddr().String()).Debug("Unchoked peer based on score")
				}
				unchokedCount++
			}

			// Handle optimistic unchoking
			if now.Sub(pm.lastOptimistic) >= optimisticInterval {
				// Choose a new optimistic peer
				var candidates []*pp.PeerConn
				for peer, stats := range pm.peerStats {
					if stats.isInterested && peer.PeerChoked && peer != pm.optimisticPeer {
						candidates = append(candidates, peer)
					}
				}

				if len(candidates) > 0 {
					// Remove old optimistic peer
					if pm.optimisticPeer != nil && !pm.optimisticPeer.PeerChoked {
						pm.optimisticPeer.SendChoke()
					}

					// Select new optimistic peer
					newOptimistic := candidates[rand.Intn(len(candidates))]
					pm.optimisticPeer = newOptimistic
					newOptimistic.SendUnchoke()
					stats := pm.peerStats[newOptimistic]
					stats.lastWeUnchoked = now
					pm.lastOptimistic = now

					log.WithField("peer", newOptimistic.RemoteAddr().String()).Debug("Selected new optimistic unchoke peer")
				}
			}

			// Special handling for seeding mode
			if pm.downloadManager != nil && pm.downloadManager.IsFinished() {
				pm.handleSeedingMode(now)
			}

			pm.Mu.Unlock()

		case <-pm.shutdownChan:
			log.Info("Shutting down choking algorithm goroutine")
			return
		}
	}
}

func (pm *PeerManager) handleSeedingMode(now time.Time) {
	// When seeding, prioritize peers who haven't downloaded much yet
	var seedingPeers []*pp.PeerConn
	for peer, stats := range pm.peerStats {
		if stats.isInterested {
			seedingPeers = append(seedingPeers, peer)
		}
	}

	// Sort by inverse of total bytes downloaded (give priority to peers who have downloaded less)
	sort.Slice(seedingPeers, func(i, j int) bool {
		statsI := pm.peerStats[seedingPeers[i]]
		statsJ := pm.peerStats[seedingPeers[j]]
		return statsI.bytesDownloaded < statsJ.bytesDownloaded
	})

	// Allow more unchoked peers when seeding
	maxSeedingUnchoked := 8
	unchokedCount := 0

	for _, peer := range seedingPeers {
		if unchokedCount >= maxSeedingUnchoked {
			if !peer.PeerChoked {
				peer.SendChoke()
			}
			continue
		}

		if peer.PeerChoked {
			peer.SendUnchoke()
			stats := pm.peerStats[peer]
			stats.lastWeUnchoked = now
		}
		unchokedCount++
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

		// If peer is providing data, show interest
		if !pc.Interested {
			pc.SendInterested()
		}
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
	log.WithFields(logrus.Fields{
		"peer":          pc.RemoteAddr().String(),
		"downloaded":    bytesDownloaded,
		"uploaded":      bytesUploaded,
		"download_rate": stats.downloadRate,
		"upload_rate":   stats.uploadRate,
	}).Debug("Updated peer stats")
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

	stats, exists := pm.peerStats[pc]
	if !exists {
		stats = &PeerStats{}
		pm.peerStats[pc] = stats
	}

	stats.isInterested = true
	pc.PeerInterested = true

	log.WithField("peer", pc.RemoteAddr().String()).Debug("Peer became interested")

	// If we're seeding, consider unchoking immediately
	if pm.downloadManager != nil && pm.downloadManager.IsFinished() {
		if pc.PeerChoked {
			pc.SendUnchoke()
			stats.lastWeUnchoked = time.Now()
			log.WithField("peer", pc.RemoteAddr().String()).Debug("Unchoked interested peer while seeding")
		}
	}
}

func (pm *PeerManager) OnPeerNotInterested(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	_, exists := pm.Peers[pc]
	if !exists {
		log.Warn("peerState does not exist")
		return
	}

	pc.PeerInterested = false

	if !pc.PeerChoked {
		pc.SendChoke()
		log.WithField("peer", pc.RemoteAddr().String()).Info("Choked not interested peer")
	}
}

func (pm *PeerManager) OnPeerChoke(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	stats, exists := pm.peerStats[pc]
	if !exists {
		return
	}

	stats.hasUnchokedUs = false
	log.WithField("peer", pc.RemoteAddr().String()).Debug("Peer choked us")
}
func (pm *PeerManager) OnPeerUnchoke(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	stats, exists := pm.peerStats[pc]
	if !exists {
		return
	}

	stats.hasUnchokedUs = true
	stats.lastUnchokedUs = time.Now()
	log.WithField("peer", pc.RemoteAddr().String()).Debug("Peer unchoked us")
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

func (pm *PeerManager) RemovePeer(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	delete(pm.Peers, pc)
	delete(pm.peerStats, pc)

	if pm.optimisticPeer == pc {
		pm.optimisticPeer = nil
		pm.lastOptimistic = time.Time{} // Reset to trigger new optimistic unchoke
	}
}

func (pm *PeerManager) AddPeer(pc *pp.PeerConn) {
	pm.Mu.Lock()
	defer pm.Mu.Unlock()

	if _, exists := pm.Peers[pc]; !exists {
		pm.Peers[pc] = NewPeerState()
		pm.peerStats[pc] = &PeerStats{
			lastDownload: time.Now(),
			lastUpload:   time.Now(),
		}
	}
}
