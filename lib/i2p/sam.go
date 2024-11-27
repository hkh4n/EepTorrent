package i2p

/*
A cross-platform I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"fmt"
	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/sam3"
	"os"
	"strconv"
)

// SAMConfig holds the configuration options for SAM sessions.
type SAMConfig struct {
	InboundLength          int
	OutboundLength         int
	InboundQuantity        int
	OutboundQuantity       int
	InboundBackupQuantity  int
	OutboundBackupQuantity int
	InboundLengthVariance  int
	OutboundLengthVariance int
}

// ToOptions converts SAMConfig to a slice of option strings.
func (cfg *SAMConfig) ToOptions() []string {
	return []string{
		fmt.Sprintf("inbound.length=%d", cfg.InboundLength),
		fmt.Sprintf("outbound.length=%d", cfg.OutboundLength),
		fmt.Sprintf("inbound.quantity=%d", cfg.InboundQuantity),
		fmt.Sprintf("outbound.quantity=%d", cfg.OutboundQuantity),
		fmt.Sprintf("inbound.backupQuantity=%d", cfg.InboundBackupQuantity),
		fmt.Sprintf("outbound.backupQuantity=%d", cfg.OutboundBackupQuantity),
		fmt.Sprintf("inbound.lengthVariance=%d", cfg.InboundLengthVariance),
		fmt.Sprintf("outbound.lengthVariance=%d", cfg.OutboundLengthVariance),
	}
}

// DefaultSAMConfig returns the default SAM configuration.
func DefaultSAMConfig() SAMConfig {
	return SAMConfig{
		InboundLength:          1,
		OutboundLength:         1,
		InboundQuantity:        3,
		OutboundQuantity:       3,
		InboundBackupQuantity:  1,
		OutboundBackupQuantity: 1,
		InboundLengthVariance:  0,
		OutboundLengthVariance: 0,
	}
}

var GlobalSAM *sam3.SAM
var GlobalStreamSession *sam3.StreamSession
var GlobalKeys i2pkeys.I2PKeys

func InitSAM(cfg SAMConfig) error {
	var err error
	GlobalSAM, err = sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		return fmt.Errorf("Failed to create global SAM session: %v", err)
	}

	GlobalKeys, err = GlobalSAM.NewKeys()
	if err != nil {
		//return fmt.Errorf("Failed to generate keys for global SAM session: %v", err)
		panic(err)
	}

	options := cfg.ToOptions()
	globalSessionName := fmt.Sprintf("global-session-%d", os.Getpid())
	GlobalStreamSession, err = GlobalSAM.NewStreamSessionWithSignature(
		globalSessionName,
		GlobalKeys,
		options,
		strconv.Itoa(7),
	)
	if err != nil {
		//return fmt.Errorf("Failed to create global SAM stream session: %v", err)
		panic(err)
	}

	return nil
}

func CloseSAM() {
	if GlobalStreamSession != nil {
		GlobalStreamSession.Close()
	}
	if GlobalSAM != nil {
		GlobalSAM.Close()
	}
}

func Cleanup() {
	fmt.Println("Performing cleanup...")
	CloseSAM()
	fmt.Println("SAM session closed")
}
