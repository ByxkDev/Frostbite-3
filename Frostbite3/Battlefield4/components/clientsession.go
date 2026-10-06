package components

import (
	"sort"
	"sync"

	"bf4/logger"
	"bf4/server"
)

type ClientSession struct {
	ConnID uint64
	Push   func(b []byte) error

	OnlineID  string
	Auth      string
	Exti      uint64
	Extb      []byte
	PersonaID uint64
	Address   *ipPairAddress
	UserAttr  uint64
}

var (
	clientMu sync.Mutex

	sessionsMu        sync.RWMutex
	sessionsByPersona = map[uint64]*ClientSession{}
	sessionData       = map[*ClientSession]ClientSession{} 
)

func newClientSession(connID uint64, push func([]byte) error) *ClientSession {
	return &ClientSession{ConnID: connID, Push: push, UserAttr: userAttributes}
}

func handleClientPacket(s *ClientSession, data []byte) []byte {
	clientMu.Lock()
	defer clientMu.Unlock()

	s.activate()
	defer s.capture()
	return HandlePacket(data)
}

func (s *ClientSession) activate() {
	sessionsMu.RLock()
	saved, ok := sessionData[s]
	sessionsMu.RUnlock()
	if !ok {
		saved = *s
	}

	component35Mu.Lock()
	component35OnlineID = saved.OnlineID
	component35Auth = saved.Auth
	component35Exti = saved.Exti
	component35Extb = append([]byte(nil), saved.Extb...)
	component35PersonaID = saved.PersonaID
	component35Mu.Unlock()

	userStateMu.Lock()
	lastAddress = saved.Address
	userAttrValue = saved.UserAttr
	userStateMu.Unlock()

	pushMu.Lock()
	pushToken++
	PushNotification = s.Push
	pushMu.Unlock()
}

func (s *ClientSession) capture() {
	var saved ClientSession
	saved.ConnID, saved.Push = s.ConnID, s.Push

	component35Mu.RLock()
	saved.OnlineID = component35OnlineID
	saved.Auth = component35Auth
	saved.Exti = component35Exti
	saved.Extb = append([]byte(nil), component35Extb...)
	saved.PersonaID = component35PersonaID
	component35Mu.RUnlock()

	userStateMu.Lock()
	if lastAddress != nil {
		a := *lastAddress
		saved.Address = &a
	}
	saved.UserAttr = userAttrValue
	userStateMu.Unlock()

	sessionsMu.Lock()
	prev := sessionData[s]
	sessionData[s] = saved
	if prev.PersonaID != 0 && prev.PersonaID != saved.PersonaID && sessionsByPersona[prev.PersonaID] == s {
		delete(sessionsByPersona, prev.PersonaID)
	}
	if saved.PersonaID != 0 {
		if old, ok := sessionsByPersona[saved.PersonaID]; ok && old != s {
			logger.Info("SESSIONS: %q (%d) logged in again on connection %d, replacing connection %d",
				saved.OnlineID, saved.PersonaID, saved.ConnID, sessionData[old].ConnID)
		} else if !ok {
			logger.Info("SESSIONS: %q (%d) online on connection %d (%d PS3s online)",
				saved.OnlineID, saved.PersonaID, saved.ConnID, len(sessionsByPersona)+1)
		}
		sessionsByPersona[saved.PersonaID] = s
	}
	sessionsMu.Unlock()
}

func (s *ClientSession) Snapshot() ClientSession {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	if d, ok := sessionData[s]; ok {
		return d
	}
	return *s
}

func closeClientSession(s *ClientSession) {
	d := s.Snapshot()

	sessionsMu.Lock()
	delete(sessionData, s)
	if sessionsByPersona[d.PersonaID] == s {
		delete(sessionsByPersona, d.PersonaID)
	}
	online := len(sessionsByPersona)
	sessionsMu.Unlock()

	if d.PersonaID == 0 {
		return
	}
	closePeerGamesOf(d.PersonaID, d.OnlineID)
	for _, gid := range server.Games.LeaveAll(d.PersonaID) {
		logger.Info("SESSIONS: %q left game %d (disconnected)", d.OnlineID, gid)
		notifyPlayersOfRemoval(gid, d.PersonaID)
	}
	logger.Info("SESSIONS: %q (%d) offline (%d PS3s online)", d.OnlineID, d.PersonaID, online)
}

func SessionByPersona(personaID uint64) *ClientSession {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	return sessionsByPersona[personaID]
}

func OnlinePlayers() []ClientSession {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	out := make([]ClientSession, 0, len(sessionsByPersona))
	for _, s := range sessionsByPersona {
		out = append(out, sessionData[s])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OnlineID < out[j].OnlineID })
	return out
}

func pushToPersona(personaID uint64, b []byte) {
	if len(b) == 0 {
		return
	}
	s := SessionByPersona(personaID)
	if s == nil || s.Push == nil {
		return
	}
	if err := s.Push(b); err != nil {
		logger.Warn("SESSIONS: push to %d failed: %v", personaID, err)
	}
}

func attrsFor(personaID uint64) uint64 {
	if personaID != 0 && personaID == currentPersonaID() {
		return currentUserAttributes()
	}
	if s := SessionByPersona(personaID); s != nil {
		return s.Snapshot().UserAttr
	}
	return userAttributes
}

func addressFor(personaID uint64) *ipPairAddress {
	if personaID != 0 && personaID == currentPersonaID() {
		userStateMu.Lock()
		defer userStateMu.Unlock()
		if lastAddress != nil {
			a := *lastAddress
			return &a
		}
		return nil
	}
	if s := SessionByPersona(personaID); s != nil {
		return s.Snapshot().Address
	}
	return nil
}

func notifyPlayersOfRemoval(gid uint32, personaID uint64) {
	b := gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{GID: gid, PID: personaID, REAS: removeReasonLeft})
	for _, pid := range server.Games.PlayerIDs(gid) {
		pushToPersona(pid, b)
	}
	if push := server.Games.HostPushFor(gid); push != nil {
		_ = push(b)
	}
}
