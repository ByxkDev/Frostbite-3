package server

const (
	RemoveReasonJoinTimeout         uint32 = 0x0
	RemoveReasonConnLost            uint32 = 0x1
	RemoveReasonBlazeServerConnLost uint32 = 0x2
	RemoveReasonMigrationFailed     uint32 = 0x3
	RemoveReasonGameDestroyed       uint32 = 0x4
	RemoveReasonGameEnded           uint32 = 0x5
	RemoveReasonPlayerLeft          uint32 = 0x6
	RemoveReasonGroupLeft           uint32 = 0x7
	RemoveReasonKicked              uint32 = 0x8
	RemoveReasonKickedWithBan       uint32 = 0x9
	RemoveReasonJoinFromQueueFail   uint32 = 0xA
	RemoveReasonReservationTimeout  uint32 = 0xB
	RemoveReasonHostEjected         uint32 = 0xC
)

const (
	MeshDisconnected uint32 = 0
	MeshConnected    uint32 = 2
)

type DestroyGameRequest struct {
	GID  uint32 `tdf:"GID"`
	REAS uint32 `tdf:"REAS"`
}

type NotifyGameCapacityChange struct {
	CAP  []uint16 `tdf:"CAP"`
	GID  uint32   `tdf:"GID"`
	TCAP uint16   `tdf:"TCAP"`
}

type NotifyGameNameChange struct {
	GID  uint32 `tdf:"GID"`
	GNAM string `tdf:"GNAM"`
}

func (r *Registry) SetCapacityList(id uint32, pcap []uint16) bool {
	return r.update(id, func(g *Game) {
		if len(pcap) == 0 {
			return
		}
		g.Capacity = append([]uint16(nil), pcap...)
		if pcap[0] > 0 {
			g.MaxPlayers = pcap[0]
		}
	})
}

func (r *Registry) CapacityOf(id uint32) []uint16 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.games[id]; ok {
		return capacityList(g)
	}
	return nil
}

func (r *Registry) StateOf(id uint32) (uint32, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.games[id]; ok {
		return g.State, true
	}
	return 0, false
}

func capacityList(g *Game) []uint16 {
	if len(g.Capacity) >= 4 {
		c := append([]uint16(nil), g.Capacity[:4]...)
		c[0] = g.MaxPlayers
		return c
	}
	c := []uint16{g.MaxPlayers, 0, 4, 0}
	copy(c, g.Capacity)
	if len(g.Capacity) > 0 {
		c[0] = g.MaxPlayers
	}
	return c
}
