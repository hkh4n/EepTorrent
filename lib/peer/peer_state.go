package peer

import "sync"

type PeerState struct {
	RequestPending  bool
	PendingRequests int32
	RequestedBlocks map[uint32]map[uint32]bool // piece index -> offset -> requested
	sync.Mutex
}

func NewPeerState() *PeerState {
	log.Debug("Initializing new PeerState")
	return &PeerState{
		RequestedBlocks: make(map[uint32]map[uint32]bool),
	}
}

// IsBlockRequested checks if a specific block has already been requested
func (ps *PeerState) IsBlockRequested(pieceIndex, offset uint32) bool {
	ps.Lock()
	defer ps.Unlock()
	if _, exists := ps.RequestedBlocks[pieceIndex]; !exists {
		return false
	}
	return ps.RequestedBlocks[pieceIndex][offset]
}

// MarkBlockRequested marks a specific block as requested
func (ps *PeerState) MarkBlockRequested(pieceIndex, offset uint32) {
	ps.Lock()
	defer ps.Unlock()
	if _, exists := ps.RequestedBlocks[pieceIndex]; !exists {
		ps.RequestedBlocks[pieceIndex] = make(map[uint32]bool)
	}
	ps.RequestedBlocks[pieceIndex][offset] = true
}
