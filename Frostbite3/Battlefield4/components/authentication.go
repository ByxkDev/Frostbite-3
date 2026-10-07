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
	"bf4/logger"
	"bf4/psn"
)

const (
	ComponentAuthentication uint16 = 1

	CommandPs3Login uint16 = 0x0098
	CommandSilentLogin uint16 = 0x00C7

	defaultEmail = "email@gmail.com"

	fixedBlazeUserID int64 = 1000000001

	externalRefTypePS3 uint32 = 2
	personaStatusActive uint32 = 2

)

type consoleLoginProbeRequest struct {
	AUTH string `tdf:"AUTH"`
	EXTB []byte `tdf:"EXTB"`
	EXTI uint64 `tdf:"EXTI"`
}

var (
	authFactory = blaze.NewTdfFactory()
	authEncoder = authFactory.CreateEncoder(false)
	authDecoder = authFactory.CreateDecoder(false)
)

type authPersonaWire struct {
	DSNM string `tdf:"DSNM"`
	LAST uint32 `tdf:"LAST"`
	PID int64 `tdf:"PID"`
	STAS uint32 `tdf:"STAS"`
	XREF uint64 `tdf:"XREF"`
	XTYP uint32 `tdf:"XTYP"`
}

type authSessionWire struct {
	BUID int64 `tdf:"BUID"`
	FRST bool `tdf:"FRST"`
	KEY string `tdf:"KEY"`
	LLOG int64 `tdf:"LLOG"`
	MAIL string `tdf:"MAIL"`
	PDTL authPersonaWire `tdf:"PDTL"`
	UID int64 `tdf:"UID"`
}

type consoleLoginResponse struct {
	AGUP bool `tdf:"AGUP"`
	LDHT string `tdf:"LDHT"`
	NTOS bool `tdf:"NTOS"`
	PCTK string `tdf:"PCTK"`
	PRIV string `tdf:"PRIV"`
	SESS authSessionWire `tdf:"SESS"`
	SPAM bool `tdf:"SPAM"`
	THST string `tdf:"THST"`
	TSUI string `tdf:"TSUI"`
	TURI string `tdf:"TURI"`
}

type userProfileInfoWire struct {
	CITY string `tdf:"CITY"`
	CTRY string `tdf:"CTRY"`
	GNDR int32 `tdf:"GNDR"`
	STAT string `tdf:"STAT"`
	STRT string `tdf:"STRT"`
	ZIP string `tdf:"ZIP"`
}

type createAccountParametersWire struct {
	BDAY int32 `tdf:"BDAY"`
	BMON int32 `tdf:"BMON"`
	BYR int32 `tdf:"BYR"`
	CTRY string `tdf:"CTRY"`
	DVID uint64 `tdf:"DVID"`
	GEST bool `tdf:"GEST"`
	LANG string `tdf:"LANG"`
	MAIL string `tdf:"MAIL"`
	OPT1 uint8 `tdf:"OPT1"`
	OPT3 uint8 `tdf:"OPT3"`
	PASS string `tdf:"PASS"`
	PNAM string `tdf:"PNAM"`
	PRNT string `tdf:"PRNT"`
	PROF userProfileInfoWire `tdf:"PROF"`
	TOSV string `tdf:"TOSV"`
}

type consoleCreateAccountRequest struct {
	CREQ createAccountParametersWire `tdf:"CREQ"`
	PERS string `tdf:"PERS"`
	TICK []byte `tdf:"TICK"`
	UID int64 `tdf:"UID"`
	XREF uint64 `tdf:"XREF"`
}

type fullLoginResponse struct {
	AGUP bool `tdf:"AGUP"`
	LDHT string `tdf:"LDHT"`
	NTOS bool `tdf:"NTOS"`
	PCTK string `tdf:"PCTK"`
	PRIV string `tdf:"PRIV"`
	SESS authSessionWire `tdf:"SESS"`
	SPAM bool `tdf:"SPAM"`
	THST string `tdf:"THST"`
	TSUI string `tdf:"TSUI"`
	TURI string `tdf:"TURI"`
}

type consoleCreateAccountResponse struct {
	RSLT int32 `tdf:"RSLT"`
	SESS authSessionWire `tdf:"SESS"`
}

type ps3LoginRequest struct {
	MAIL string `tdf:"MAIL"`
	TCKT []byte `tdf:"TCKT"`
}

type silentLoginRequest struct {
	AUTH string `tdf:"AUTH"`
	PID int64 `tdf:"PID"`
	TYPE int64 `tdf:"TYPE"`
}

type PersonaDetails struct {
	DisplayName string
	ExtId uint64
	ExtType int32
	LastAuthenticated uint32
	PersonaId int64
	Status int32
}

type SessionInfo struct {
	BlazeUserId int64
	Email string
	IsFirstLogin bool
	LastLoginDateTime int64
	PersonaDetails PersonaDetails
	SessionKey string
	UserId int64
}

type ConsoleCreateAccountResponse struct {
	CreateResult int32
	SessionInfo SessionInfo
}

type sessionInfo struct {
	BlazeUserId int64
	Email string
	IsFirstLogin bool
	LastLoginDateTime int64
	PersonaDetails PersonaDetails
	SessionKey string
	UserId int64
}

var (
	mu sync.Mutex
	session *sessionInfo
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

func buildSession(t *xi5.Ticket,requestedPersonaID int64,email string,now int64,existing *sessionInfo)(*sessionInfo,error) {
	if t == nil {
		return nil,errors.New("XI5 ticket is required to build the session")
	}

	if t.UserID == 0 {
		return nil,errors.New("XI5 ticket contains an invalid UserId")
	}

	if strings.TrimSpace(t.OnlineID) == "" {
		return nil,errors.New("XI5 ticket contains an empty OnlineId")
	}

	if strings.TrimSpace(email) == "" && existing != nil {
		email = existing.Email
	}

	if strings.TrimSpace(email) == "" {
		email = defaultEmail
	}

	maxInt64 := uint64(^uint64(0) >> 1)

	if t.UserID > maxInt64 {
		return nil,errors.New("XI5 UserId does not fit in int64")
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
		return nil,errors.New("timestamp cannot be negative")
	}

	if now > int64(^uint32(0)) {
		return nil,errors.New("timestamp does not fit in uint32")
	}

	key := newSessionKey()

	if existing != nil && strings.TrimSpace(existing.SessionKey) != "" {
		key = existing.SessionKey
	}

	persona := PersonaDetails{
		DisplayName: t.OnlineID,
		ExtId: uint64(t.UserID),
		ExtType: int32(externalRefTypePS3),
		LastAuthenticated: uint32(now),
		PersonaId: personaID,
		Status: int32(personaStatusActive),
	}

	return &sessionInfo{
		BlazeUserId: fixedBlazeUserID,
		Email: email,
		IsFirstLogin: false,
		LastLoginDateTime: now,
		PersonaDetails: persona,
		SessionKey: key,
		UserId: fixedBlazeUserID,
	},nil
}

func sessionToWire(s *sessionInfo) authSessionWire {
	return authSessionWire{
		BUID: s.BlazeUserId,
		FRST: s.IsFirstLogin,
		KEY: s.SessionKey,
		LLOG: s.LastLoginDateTime,
		MAIL: s.Email,
		PDTL: authPersonaWire{
			DSNM: s.PersonaDetails.DisplayName,
			LAST: s.PersonaDetails.LastAuthenticated,
			PID: s.PersonaDetails.PersonaId,
			STAS: uint32(s.PersonaDetails.Status),
			XREF: s.PersonaDetails.ExtId,
			XTYP: uint32(s.PersonaDetails.ExtType),
		},
		UID: s.UserId,
	}
}

func buildFullLoginPayload(s *sessionInfo)([]byte,error) {
	return authEncoder.Encode(&fullLoginResponse{
		AGUP: false,
		NTOS: false,
		SESS: sessionToWire(s),
		SPAM: true,
	})
}

func respond(name string,command uint16,messageID uint32,payload []byte)[]byte {
	logger.Hex(logger.LevelDebug,"AUTH "+name+" PAYLOAD",payload)
	return blaze.EncodePacket(ComponentAuthentication,command,blaze.PacketTypeResponse,messageID,payload)
}

func logSession(s *sessionInfo) {
	logger.Info("AUTH: BUID = %d",s.BlazeUserId)
	logger.Info("AUTH: UID = %d",s.UserId)
	logger.Info("AUTH: PID = %d",s.PersonaDetails.PersonaId)
	logger.Info("AUTH: DisplayName = %s",s.PersonaDetails.DisplayName)
	logger.Info("AUTH: ExtId = %d",s.PersonaDetails.ExtId)
	logger.Info("AUTH: ExtType = %d",s.PersonaDetails.ExtType)
	logger.Info("AUTH: Status = %d",s.PersonaDetails.Status)
	logger.Info("AUTH: MAIL = %s",s.Email)
	logger.Info("AUTH: SessionKey = %s",s.SessionKey)
}

func logTicket(t *xi5.Ticket) {
	logger.Info("AUTH: XI5 Ticket:")
	logger.Info("AUTH: Version = %s",t.TicketVersion)
	logger.Info("AUTH: Serial = %s",t.Serial)
	logger.Info("AUTH: IssuerId = 0x%08X",t.IssuerID)
	logger.Info("AUTH: UserId = %d",t.UserID)
	logger.Info("AUTH: OnlineId = %s",t.OnlineID)
	logger.Info("AUTH: Region = %s",t.Region)
	logger.Info("AUTH: Domain = %s",t.Domain)
	logger.Info("AUTH: ServiceId = %s",t.ServiceID)
	logger.Info("AUTH: Status = %d",t.Status)
	logger.Info("AUTH: IssuerName = %s",t.IssuerName)
	logger.Info("AUTH: Issued = %s",t.Issued.Format(time.RFC3339))
	logger.Info("AUTH: Expires = %s",t.Expires.Format(time.RFC3339))
	logger.Info("AUTH: SignedByOfficialRPCN = %t",t.SignedByOfficialRPCN())
}

func HandleSilentLogin(p blaze.Packet)[]byte {
	var req silentLoginRequest

	if err := authDecoder.Decode(p.Payload,&req); err != nil {
		logger.Warn("AUTH: SilentLogin request decode problem: %v",err)
	}

	auth := req.AUTH

	if strings.TrimSpace(auth) == "" {
		auth = "<empty>"
	}

	requestedPID := req.PID

	logger.Info("AUTH: AUTH = %s",auth)
	logger.Info("AUTH: PID = %d",requestedPID)
	logger.Info("AUTH: TYPE = %d",req.TYPE)

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

	logger.Info("AUTH: SilentLogin using stored XI5 identity: UserId=%d OnlineId=%q", ticket.UserID, ticket.OnlineID)

	if requestedPID == 0 && existing != nil {
		requestedPID = existing.PersonaDetails.PersonaId
	}

	email := defaultEmail

	if existing != nil {
		email = existing.Email
	}

	s,err := buildSession(ticket, requestedPID, email, time.Now().Unix(), existing,)

	if err != nil {
		logger.Error("AUTH: failed to build session: %v",err)
		return nil
	}

	payload,err := buildFullLoginPayload(s)
	if err != nil {
		logger.Error("AUTH: failed to encode SilentLogin response: %v",err)
		return nil
	}

	mu.Lock()
	session = s
	mu.Unlock()

	logSession(s)
	logger.Info("AUTH: SilentLogin response prepared: AGUP=false SPAM=true NTOS=false")

	return respond("SilentLogin",p.Command,p.MessageId,payload)
}

type ticketLoginRequest struct {
	TCKT []byte `tdf:"TCKT"`
}

func HandleTicketLogin(p blaze.Packet) []byte {
	var req ticketLoginRequest

	if err := authDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("AUTH: 0xC7 request decode problem: %v", err)
	}

	logger.Info("AUTH: 0xC7 ticket login, TCKT length=%d", len(req.TCKT))

	if len(req.TCKT) > 0 {
		SetPsnTicket(req.TCKT)
	} else if getXi5Ticket() == nil {
		logger.Error("AUTH: 0xC7 has no TCKT and no stored XI5 ticket")
		return nil
	}

	if t := getXi5Ticket(); t != nil {
		logTicket(t)
	}

	return HandleSilentLogin(p)
}
