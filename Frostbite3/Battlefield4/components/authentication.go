package components

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/logger"
	"bf4/psn"
)

const (
	ComponentAuthentication uint16 = 1

	CommandPs3Login    uint16 = 0x0098 
	CommandSilentLogin uint16 = 0x00C7 

	defaultEmail = "email@gmail.com"

	fixedBlazeUserID int64 = 1000000001

	externalRefTypePS3  uint32 = 2 
	personaStatusActive uint32 = 1 
)

type personaDetails struct {
	DisplayName       string
	ExtID             uint64
	ExtType           uint32
	LastAuthenticated uint32
	PersonaID         int64
	Status            uint32
}

type sessionInfo struct {
	BlazeUserID       int64
	Email             string
	IsFirstLogin      bool
	LastLoginDateTime int64
	Persona           personaDetails
	SessionKey        string
	UserID            int64
}

var (
	mu        sync.Mutex
	session   *sessionInfo
	psnTicket []byte
	xi5Ticket *xi5.Ticket
)

func SetPsnTicket(ticket []byte) {
	if len(ticket) == 0 {
		logger.Warn("AUTH: SetPsnTicket called with an empty byte[] ticket")
		return
	}

	cp := make([]byte, len(ticket))
	copy(cp, ticket)

	parsed, err := xi5.NewTicket(cp)
	if err != nil {
		logger.Error("AUTH: failed to parse XI5 ticket in SetPsnTicket: %v", err)
		return
	}

	mu.Lock()
	psnTicket = cp
	xi5Ticket = parsed
	mu.Unlock()

	logger.Info("AUTH: PSN/XI5 ticket stored. Length=%d UserId=%d OnlineId=%q",
		len(cp), parsed.UserID, parsed.OnlineID)
}

func SetPsnTicketString(ticket string) {
	if strings.TrimSpace(ticket) == "" {
		logger.Warn("AUTH: SetPsnTicket called with an empty string ticket")
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

	var personaID int64
	switch {
	case requestedPersonaID != 0:
		personaID = requestedPersonaID
	case existing != nil && existing.Persona.PersonaID != 0:
		personaID = existing.Persona.PersonaID
	default:
		if t.UserID > math.MaxInt64 {
			return nil, errors.New("XI5 UserId does not fit in int64")
		}
		personaID = int64(t.UserID)
	}

	if now < 0 || now > math.MaxUint32 {
		return nil, errors.New("timestamp does not fit in uint32")
	}

	key := newSessionKey()
	if existing != nil && strings.TrimSpace(existing.SessionKey) != "" {
		key = existing.SessionKey
	}

	return &sessionInfo{
		BlazeUserID:       fixedBlazeUserID,
		Email:             email,
		IsFirstLogin:      false,
		LastLoginDateTime: now,
		Persona: personaDetails{
			DisplayName:       t.OnlineID,
			ExtID:             t.UserID,
			ExtType:           externalRefTypePS3,
			LastAuthenticated: uint32(now),
			PersonaID:         personaID,
			Status:            personaStatusActive,
		},
		SessionKey: key,
		UserID:     fixedBlazeUserID,
	}, nil
}

func findField(fields []blaze.TDF, tag string) (blaze.TDF, bool) {
	tag = strings.TrimSpace(tag)
	for _, f := range fields {
		if strings.TrimSpace(f.Tag) == tag {
			return f, true
		}
	}
	return blaze.TDF{}, false
}

func fieldInt(fields []blaze.TDF, tag string) int64 {
	if f, ok := findField(fields, tag); ok {
		if v, ok := f.Value.(int64); ok {
			return v
		}
	}
	return 0
}

func fieldString(fields []blaze.TDF, tag string) string {
	if f, ok := findField(fields, tag); ok {
		if v, ok := f.Value.(string); ok {
			return v
		}
	}
	return ""
}

func fieldBlob(fields []blaze.TDF, tag string) []byte {
	if f, ok := findField(fields, tag); ok {
		if v, ok := f.Value.([]byte); ok {
			return v
		}
	}
	return nil
}

func encodeSession(s *sessionInfo) []byte {
	var pdtl bytes.Buffer
	blaze.WriteTDF(&pdtl, "DSNM", s.Persona.DisplayName)
	blaze.WriteUInt32(&pdtl, "LAST", s.Persona.LastAuthenticated)
	blaze.WriteInt64(&pdtl, "PID", s.Persona.PersonaID)
	blaze.WriteUInt32(&pdtl, "STAS", s.Persona.Status)
	blaze.WriteUInt64(&pdtl, "XREF", s.Persona.ExtID)
	blaze.WriteUInt32(&pdtl, "XTYP", s.Persona.ExtType)

	var sess bytes.Buffer
	blaze.WriteInt64(&sess, "BUID", s.BlazeUserID)
	blaze.WriteBool(&sess, "FRST", s.IsFirstLogin)
	blaze.WriteTDF(&sess, "KEY", s.SessionKey)
	blaze.WriteInt64(&sess, "LLOG", s.LastLoginDateTime)
	blaze.WriteTDF(&sess, "MAIL", s.Email)
	blaze.WriteStruct(&sess, "PDTL", pdtl.Bytes())
	blaze.WriteInt64(&sess, "UID", s.UserID)

	return sess.Bytes()
}

func buildConsoleLoginPayload(s *sessionInfo) []byte {
	var b bytes.Buffer
	blaze.WriteBool(&b, "AGUP", false) // mCanAgeUp
	blaze.WriteTDF(&b, "LDHT", "")     // mLegalDocHost
	blaze.WriteBool(&b, "NTOS", false) // mNeedsLegalDoc
	blaze.WriteTDF(&b, "PRIV", "")     // mPrivacyPolicyUri
	blaze.WriteStruct(&b, "SESS", encodeSession(s))
	blaze.WriteBool(&b, "SPAM", true) // mIsOfLegalContactAge
	blaze.WriteTDF(&b, "THST", "")    // mTosHost
	blaze.WriteTDF(&b, "TSUI", "")    // mTosUri
	blaze.WriteTDF(&b, "TURI", "")    // mTermsOfServiceUri
	return b.Bytes()
}

func buildFullLoginPayload(s *sessionInfo) []byte {
	var b bytes.Buffer
	blaze.WriteBool(&b, "AGUP", false)
	blaze.WriteTDF(&b, "LDHT", "")
	blaze.WriteBool(&b, "NTOS", false)
	blaze.WriteTDF(&b, "PCTK", "") // mPCLoginToken
	blaze.WriteTDF(&b, "PRIV", "")
	blaze.WriteStruct(&b, "SESS", encodeSession(s))
	blaze.WriteBool(&b, "SPAM", true)
	blaze.WriteTDF(&b, "THST", "")
	blaze.WriteTDF(&b, "TSUI", "")
	blaze.WriteTDF(&b, "TURI", "")
	return b.Bytes()
}

func respond(name string, command uint16, messageID uint32, payload []byte) []byte {
	logger.Hex(logger.LevelDebug, "AUTH "+name+" PAYLOAD", payload)
	return blaze.EncodePacket(ComponentAuthentication, command, blaze.PacketTypeResponse, messageID, payload)
}

func logSession(s *sessionInfo) {
	logger.Info("AUTH: BUID = %d", s.BlazeUserID)
	logger.Info("AUTH: UID = %d", s.UserID)
	logger.Info("AUTH: PID = %d", s.Persona.PersonaID)
	logger.Info("AUTH: DisplayName = %s", s.Persona.DisplayName)
	logger.Info("AUTH: ExtId = %d", s.Persona.ExtID)
	logger.Info("AUTH: MAIL = %s", s.Email)
	logger.Info("AUTH: SessionKey = %s", s.SessionKey)
}

func logTicket(t *xi5.Ticket) {
	logger.Info("AUTH: XI5 Ticket:")
	logger.Info("AUTH: Version              = %s", t.TicketVersion)
	logger.Info("AUTH: Serial               = %s", t.Serial)
	logger.Info("AUTH: IssuerId             = 0x%08X", t.IssuerID)
	logger.Info("AUTH: UserId               = %d", t.UserID)
	logger.Info("AUTH: OnlineId             = %s", t.OnlineID)
	logger.Info("AUTH: Region               = %s", t.Region)
	logger.Info("AUTH: Domain               = %s", t.Domain)
	logger.Info("AUTH: ServiceId            = %s", t.ServiceID)
	logger.Info("AUTH: Status               = %d", t.Status)
	logger.Info("AUTH: IssuerName           = %s", t.IssuerName)
	logger.Info("AUTH: Issued               = %s", t.Issued.Format(time.RFC3339))
	logger.Info("AUTH: Expires              = %s", t.Expires.Format(time.RFC3339))
	logger.Info("AUTH: SignedByOfficialRPCN = %t", t.SignedByOfficialRPCN())
}

func HandlePs3Login(p blaze.Packet) []byte {
	logger.Info("===== PS3 LOGIN / XI5 =====")

	req := blaze.ReadTDF(p.Payload)

	ticketBytes := fieldBlob(req, "TCKT")
	if len(ticketBytes) == 0 {
		ticketBytes = GetPsnTicket()
	}
	if len(ticketBytes) == 0 {
		logger.Error("AUTH: PS3LoginRequest.TCKT is empty and no stored ticket is available")
		return nil
	}

	logger.Info("AUTH: XI5 ticket length = %d", len(ticketBytes))

	ticket, err := xi5.NewTicket(ticketBytes)
	if err != nil {
		logger.Error("AUTH: failed to parse XI5 ticket: %v", err)
		return nil
	}

	logTicket(ticket)

	cp := make([]byte, len(ticketBytes))
	copy(cp, ticketBytes)

	mu.Lock()
	psnTicket = cp
	xi5Ticket = ticket
	mu.Unlock()

	mail := fieldString(req, "MAIL")
	if strings.TrimSpace(mail) == "" {
		mail = defaultEmail
		logger.Warn("AUTH: PS3LoginRequest.MAIL is empty. Using fallback email: %s", mail)
	}

	s, err := buildSession(ticket, 0, mail, time.Now().Unix(), nil)
	if err != nil {
		logger.Error("AUTH: failed to build session: %v", err)
		return nil
	}

	mu.Lock()
	session = s
	mu.Unlock()

	logger.Info("===== PS3 LOGIN ACCEPTED =====")
	logSession(s)
	logger.Info("===== PS3 LOGIN COMPLETE =====")

	return respond("Ps3Login", p.Command, p.MessageId, buildConsoleLoginPayload(s))
}

func HandleSilentLogin(p blaze.Packet) []byte {
	logger.Info("===== SILENT LOGIN 0x00C7 =====")

	req := blaze.ReadTDF(p.Payload)

	auth := fieldString(req, "AUTH")
	if strings.TrimSpace(auth) == "" {
		auth = "<empty>"
	}
	requestedPID := fieldInt(req, "PID")

	logger.Info("AUTH: AUTH = %s", auth)
	logger.Info("AUTH: PID = %d", requestedPID)
	logger.Info("AUTH: TYPE = %d", fieldInt(req, "TYPE"))

	ticket := getXi5Ticket()
	if ticket == nil {
		logger.Error("AUTH: no stored XI5 ticket available during SilentLogin")
		return nil
	}

	var existing *sessionInfo
	mu.Lock()
	if session != nil {
		c := *session
		existing = &c
	}
	mu.Unlock()

	logger.Info("AUTH: SilentLogin using stored XI5 identity: UserId=%d OnlineId=%q",
		ticket.UserID, ticket.OnlineID)

	if requestedPID == 0 && existing != nil {
		requestedPID = existing.Persona.PersonaID
	}

	email := defaultEmail
	if existing != nil {
		email = existing.Email
	}

	s, err := buildSession(ticket, requestedPID, email, time.Now().Unix(), existing)
	if err != nil {
		logger.Error("AUTH: failed to build session: %v", err)
		return nil
	}

	mu.Lock()
	session = s
	mu.Unlock()

	logger.Info("===== SILENT LOGIN ACCEPTED =====")
	logSession(s)
	logger.Info("AUTH: SilentLogin response prepared: AGUP=false SPAM=true NTOS=false")
	logger.Info("===== SILENT LOGIN COMPLETE =====")

	return respond("SilentLogin", p.Command, p.MessageId, buildFullLoginPayload(s))
}
