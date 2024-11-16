package download

import pp "github.com/go-i2p/go-i2p-bt/peerprotocol"

func createDownloadHandler(dm *DownloadManager) pp.Handler {
	return &DownloadHandler{
		NoopHandler: pp.NoopHandler{},
		dm:          dm,
	}
}

type DownloadHandler struct {
	pp.NoopHandler
	dm *DownloadManager
}

func (h *DownloadHandler) OnHandShake(pc *pp.PeerConn) error {
	return nil
}

func (h *DownloadHandler) OnMessage(pc *pp.PeerConn, msg pp.Message) error {
	switch msg.Type {
	case pp.MTypeBitField:
		pc.BitField = msg.BitField
		return nil

	case pp.MTypeHave:
		pc.BitField.Set(msg.Index)
		return nil

	case pp.MTypePiece:
		return h.dm.OnBlock(msg.Index, msg.Begin, msg.Piece)

	default:
		return nil
	}
}
