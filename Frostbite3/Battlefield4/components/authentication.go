package components

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/psn"
)

const (
	ComponentAuthentication uint16 = 1
	CommandPs3Login         uint16 = 0x0098
	CommandSilentLogin      uint16 = 0x00C7
	defaultEmail                   = "email@gmail.com"
	fixedBlazeUserID        int64  = 1000000001
	externalRefTypePS3      uint32 = 2
	personaStatusActive     uint32 = 2
)

var (
	authFactory = blaze.NewTdfFactory()
	authEncoder = authFactory.CreateEncoder(false)
	authDecoder = authFactory.CreateDecoder(false)
)

type authPersonaWire struct {
	DSNM string `tdf:"DSNM"`
	LAST uint32 `tdf:"LAST"`
	PID  int64  `tdf:"PID"`
	STAS uint32 `tdf:"STAS"`
	XREF uint64 `tdf:"XREF"`
	XTYP uint32 `tdf:"XTYP"`
}

type authSessionWire struct {
	BUID int64           `tdf:"BUID"`
	FRST bool            `tdf:"FRST"`
	KEY  string          `tdf:"KEY"`
	LLOG int64           `tdf:"LLOG"`
	MAIL string          `tdf:"MAIL"`
	PDTL authPersonaWire `tdf:"PDTL"`
	UID  int64           `tdf:"UID"`
}

type fullLoginResponse struct {
	AGUP bool            `tdf:"AGUP"`
	LDHT string          `tdf:"LDHT"`
	NTOS bool            `tdf:"NTOS"`
	PCTK string          `tdf:"PCTK"`
	PRIV string          `tdf:"PRIV"`
	SESS authSessionWire `tdf:"SESS"`
	SPAM bool            `tdf:"SPAM"`
	THST string          `tdf:"THST"`
	TSUI string          `tdf:"TSUI"`
	TURI string          `tdf:"TURI"`
}

type silentLoginRequest struct {
	AUTH string `tdf:"AUTH"`
	PID  int64  `tdf:"PID"`
	TYPE int64  `tdf:"TYPE"`
}

type PersonaDetails struct {
	DisplayName       string
	ExtId             uint64
	ExtType           int32
	LastAuthenticated uint32
	PersonaId         int64
	Status            int32
}

type SessionInfo struct {
	BlazeUserId       int64
	Email             string
	IsFirstLogin      bool
	LastLoginDateTime int64
	PersonaDetails    PersonaDetails
	SessionKey        string
	UserId            int64
}

type ConsoleCreateAccountResponse struct {
	CreateResult int32
	SessionInfo  SessionInfo
}

type sessionInfo struct {
	BlazeUserId       int64
	Email             string
	IsFirstLogin      bool
	LastLoginDateTime int64
	PersonaDetails    PersonaDetails
	SessionKey        string
	UserId            int64
}

var (
	mu        sync.Mutex
	session   *sessionInfo
	psnTicket []byte
	xi5Ticket *xi5.Ticket
)

func SetPsnTicket(ticket []byte) {
	if len(ticket) == 0 {
		return
	}

	cp := make([]byte, len(ticket))
	copy(cp, ticket)

	parsed, err := xi5.NewTicket(cp)
	if err != nil {
		return
	}

	mu.Lock()
	psnTicket = cp
	xi5Ticket = parsed
	mu.Unlock()
}

func SetPsnTicketString(ticket string) {
	if strings.TrimSpace(ticket) == "" {
		return
	}

	b, err := base64.StdEncoding.DecodeString(ticket)
	if err != nil {
		b = []byte(ticket)
	}

	SetPsnTicket(b)
}

func GetPsnTicket() []byte {
	mu.Lock()
	defer mu.Unlock()

	if len(psnTicket) == 0 {
		return nil
	}

	cp := make([]byte, len(psnTicket))
	copy(cp, psnTicket)
	return cp
}

func getXi5Ticket() *xi5.Ticket {
	mu.Lock()
	defer mu.Unlock()
	return xi5Ticket
}

func newSessionKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func buildSession(t *xi5.Ticket, requestedPersonaID int64, email string, now int64, existing *sessionInfo) (*sessionInfo, error) {
	if t == nil {
		return nil, errors.New("XI5 ticket is required to build the session")
	}

	if t.UserID == 0 {
		return nil, errors.New("XI5 ticket contains an invalid UserId")
	}

	if strings.TrimSpace(t.OnlineID) == "" {
		return nil, errors.New("XI5 ticket contains an empty OnlineId")
	}

	if strings.TrimSpace(email) == "" && existing != nil {
		email = existing.Email
	}

	if strings.TrimSpace(email) == "" {
		email = defaultEmail
	}

	maxInt64 := uint64(^uint64(0) >> 1)

	if t.UserID > maxInt64 {
		return nil, errors.New("XI5 UserId does not fit in int64")
	}

	var personaID int64

	switch {
	case requestedPersonaID != 0:
		personaID = requestedPersonaID
	case existing != nil && existing.PersonaDetails.PersonaId != 0:
		personaID = existing.PersonaDetails.PersonaId
	default:
		personaID = int64(t.UserID)
	}

	if now < 0 {
		return nil, errors.New("timestamp cannot be negative")
	}

	if now > int64(^uint32(0)) {
		return nil, errors.New("timestamp does not fit in uint32")
	}

	key := newSessionKey()

	if existing != nil && strings.TrimSpace(existing.SessionKey) != "" {
		key = existing.SessionKey
	}

	persona := PersonaDetails{
		DisplayName:       t.OnlineID,
		ExtId:             uint64(t.UserID),
		ExtType:           int32(externalRefTypePS3),
		LastAuthenticated: uint32(now),
		PersonaId:         personaID,
		Status:            int32(personaStatusActive),
	}

	return &sessionInfo{
		BlazeUserId:       fixedBlazeUserID,
		Email:             email,
		IsFirstLogin:      false,
		LastLoginDateTime: now,
		PersonaDetails:    persona,
		SessionKey:        key,
		UserId:            fixedBlazeUserID,
	}, nil
}

func sessionToWire(s *sessionInfo) authSessionWire {
	return authSessionWire{
		BUID: s.BlazeUserId,
		FRST: s.IsFirstLogin,
		KEY:  s.SessionKey,
		LLOG: s.LastLoginDateTime,
		MAIL: s.Email,
		PDTL: authPersonaWire{
			DSNM: s.PersonaDetails.DisplayName,
			LAST: s.PersonaDetails.LastAuthenticated,
			PID:  s.PersonaDetails.PersonaId,
			STAS: uint32(s.PersonaDetails.Status),
			XREF: s.PersonaDetails.ExtId,
			XTYP: uint32(s.PersonaDetails.ExtType),
		},
		UID: s.UserId,
	}
}

func buildFullLoginPayload(s *sessionInfo) ([]byte, error) {
	return authEncoder.Encode(&fullLoginResponse{
		AGUP: false,
		NTOS: false,
		SESS: sessionToWire(s),
		SPAM: true,
	})
}

func respond(name string, command uint16, messageID uint32, payload []byte) []byte {
	return blaze.EncodePacket(ComponentAuthentication, command, blaze.PacketTypeResponse, messageID, payload)
}

func HandleSilentLogin(p blaze.Packet) []byte {
	var req silentLoginRequest

	if err := authDecoder.Decode(p.Payload, &req); err != nil {
	}

	auth := req.AUTH

	if strings.TrimSpace(auth) == "" {
		auth = "<empty>"
	}

	requestedPID := req.PID
	ticket := getXi5Ticket()

	if ticket == nil {
		return nil
	}

	var existing *sessionInfo

	mu.Lock()

	if session != nil {
		c := *session
		existing = &c
	}

	mu.Unlock()

	if requestedPID == 0 && existing != nil {
		requestedPID = existing.PersonaDetails.PersonaId
	}

	email := defaultEmail

	if existing != nil {
		email = existing.Email
	}

	s, err := buildSession(ticket, requestedPID, email, time.Now().Unix(), existing)

	if err != nil {
		return nil
	}

	payload, err := buildFullLoginPayload(s)

	if err != nil {
		return nil
	}

	mu.Lock()
	session = s
	mu.Unlock()

	return respond("SilentLogin", p.Command, p.MessageId, payload)
}

type ticketLoginRequest struct {
	TCKT []byte `tdf:"TCKT"`
}

func HandleTicketLogin(p blaze.Packet) []byte {
	var req ticketLoginRequest

	if err := authDecoder.Decode(p.Payload, &req); err != nil {
	}

	if len(req.TCKT) > 0 {
		SetPsnTicket(req.TCKT)
	} else if getXi5Ticket() == nil {
		return nil
	}

	return HandleSilentLogin(p)
}
