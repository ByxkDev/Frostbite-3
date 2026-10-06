package xi5

import (
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bf4/logger"
)

const (
	ver20 uint32 = 553648128
	ver21 uint32 = 553713664
	ver30 uint32 = 822083584
	ver40 uint32 = 1090519040

	officialIssuerID uint32 = 0x33333333
)

var (
	ErrInvalidTicket  = errors.New("invalid XI5 ticket")
	ErrNotImplemented = errors.New("XI5 ticket version not yet implemented")
	ErrUnsupported    = errors.New("unsupported XI5 ticket version")
)

type datatype uint16

const (
	dtEmpty  datatype = 0
	dtUInt   datatype = 1
	dtULong  datatype = 2
	dtString datatype = 4
	dtTime   datatype = 7
	dtBinary datatype = 8
	dtBody   datatype = 0x3000
	dtFooter datatype = 0x3002
)

func (d datatype) String() string {
	switch d {
	case dtEmpty:
		return "Empty"
	case dtUInt:
		return "UInt"
	case dtULong:
		return "ULong"
	case dtString:
		return "String"
	case dtTime:
		return "Time"
	case dtBinary:
		return "Binary"
	case dtBody:
		return "Body"
	case dtFooter:
		return "Footer"
	}
	return fmt.Sprintf("0x%04X", uint16(d))
}

type point struct{ x, y *big.Int }

type curve struct {
	p, a, b, n *big.Int
	g          *point
}

var (
	rpcnCurve  *curve
	rpcnPubKey *point
)

const rpcnPublicKeyPEM = "" +
	"-----BEGIN PUBLIC KEY-----\r\n" +
	"ME4wEAYHKoZIzj0CAQYFK4EEACADOgAEsHvA8K3bl2V+nziQOejSucl9wqMdMELn\r\n" +
	"0Eebk9gcQrCr32xCGRox4x+TNC+PAzvVKcLFf9taCn0=\r\n" +
	"-----END PUBLIC KEY-----"

func mustHex(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("xi5: bad hex constant " + s)
	}
	return v
}

func init() {
	logger.Debug("XI5: initializing secp224k1 curve and RPCN public key")

	p := new(big.Int).Lsh(big.NewInt(1), 224)
	p.Sub(p, new(big.Int).Lsh(big.NewInt(1), 32))
	p.Sub(p, big.NewInt(6803))

	rpcnCurve = &curve{
		p: p,
		a: big.NewInt(0),
		b: big.NewInt(5),
		n: mustHex("010000000000000000000000000001DCE8D2EC6184CAF0A971769FB1F7"),
		g: &point{
			x: mustHex("A1455B334DF099DF30FC28A169A467E9E47075A90F7E650EB6B7A45C"),
			y: mustHex("7E089FED7FBA344282CAFBD6F7E319F7C0B0BD59E2CA4BDB556D61A5"),
		},
	}

	var b64 strings.Builder
	for _, line := range strings.Split(rpcnPublicKeyPEM, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		b64.WriteString(line)
	}

	der, err := base64.StdEncoding.DecodeString(b64.String())
	if err != nil {
		logger.Error("XI5: failed to decode RPCN public key: %v", err)
		panic(err)
	}

	const pointLen = 1 + 28 + 28
	if len(der) < pointLen || der[len(der)-pointLen] != 0x04 {
		err := errors.New("xi5: unexpected RPCN public key encoding")
		logger.Error("%v", err)
		panic(err)
	}

	raw := der[len(der)-pointLen:]
	rpcnPubKey = &point{
		x: new(big.Int).SetBytes(raw[1:29]),
		y: new(big.Int).SetBytes(raw[29:57]),
	}

	if !rpcnCurve.onCurve(rpcnPubKey) {
		err := errors.New("xi5: RPCN public key is not on secp224k1")
		logger.Error("%v", err)
		panic(err)
	}

	if !rpcnCurve.onCurve(rpcnCurve.g) {
	err := errors.New("xi5: secp224k1 generator is not on curve (bad parameters)")
	logger.Error("%v", err)
	panic(err)
    }

	logger.Debug("XI5: RPCN public key loaded")
}

func (c *curve) onCurve(pt *point) bool {
	if pt == nil {
		return false
	}

	lhs := new(big.Int).Mul(pt.y, pt.y)
	lhs.Mod(lhs, c.p)

	rhs := new(big.Int).Mul(pt.x, pt.x)
	rhs.Mul(rhs, pt.x)
	rhs.Add(rhs, new(big.Int).Mul(c.a, pt.x))
	rhs.Add(rhs, c.b)
	rhs.Mod(rhs, c.p)

	return lhs.Cmp(rhs) == 0
}

func (c *curve) add(p1, p2 *point) *point {
	if p1 == nil {
		return p2
	}
	if p2 == nil {
		return p1
	}

	var lambda *big.Int

	if p1.x.Cmp(p2.x) == 0 {
		sum := new(big.Int).Add(p1.y, p2.y)
		sum.Mod(sum, c.p)
		if sum.Sign() == 0 {
			return nil
		}

		num := new(big.Int).Mul(p1.x, p1.x)
		num.Mul(num, big.NewInt(3))
		num.Add(num, c.a)
		den := new(big.Int).Lsh(p1.y, 1)
		den.ModInverse(den, c.p)
		lambda = num.Mul(num, den)
	} else {
		num := new(big.Int).Sub(p2.y, p1.y)
		den := new(big.Int).Sub(p2.x, p1.x)
		den.Mod(den, c.p)
		den.ModInverse(den, c.p)
		lambda = num.Mul(num, den)
	}
	lambda.Mod(lambda, c.p)

	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, p1.x)
	x3.Sub(x3, p2.x)
	x3.Mod(x3, c.p)

	y3 := new(big.Int).Sub(p1.x, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, p1.y)
	y3.Mod(y3, c.p)

	return &point{x: x3, y: y3}
}

func (c *curve) mul(pt *point, k *big.Int) *point {
	var result *point
	addend := pt

	for i := 0; i < k.BitLen(); i++ {
		if k.Bit(i) == 1 {
			result = c.add(result, addend)
		}
		addend = c.add(addend, addend)
	}

	return result
}

func (c *curve) verify(pub *point, hash []byte, r, s *big.Int) bool {
	if r.Sign() <= 0 || s.Sign() <= 0 || r.Cmp(c.n) >= 0 || s.Cmp(c.n) >= 0 {
		logger.Debug("XI5: signature r/s out of range")
		return false
	}

	e := new(big.Int).SetBytes(hash)
	if excess := len(hash)*8 - c.n.BitLen(); excess > 0 {
		e.Rsh(e, uint(excess))
	}

	w := new(big.Int).ModInverse(s, c.n)
	if w == nil {
		return false
	}

	u1 := new(big.Int).Mul(e, w)
	u1.Mod(u1, c.n)
	u2 := new(big.Int).Mul(r, w)
	u2.Mod(u2, c.n)

	pt := c.add(c.mul(c.g, u1), c.mul(pub, u2))
	if pt == nil {
		return false
	}

	v := new(big.Int).Mod(pt.x, c.n)
	return v.Cmp(r) == 0
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) length() int    { return len(r.data) }
func (r *reader) remaining() int { return len(r.data) - r.pos }

func (r *reader) readN(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, fmt.Errorf("failed to read %d bytes from stream (remaining %d): %w",
			n, r.remaining(), errEOF)
	}
	out := make([]byte, n)
	copy(out, r.data[r.pos:r.pos+n])
	r.pos += n
	return out, nil
}

var errEOF = errors.New("unexpected end of stream")

func (r *reader) readUShort() (uint16, error) {
	b, err := r.readN(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *reader) readUInt32() (uint32, error) {
	b, err := r.readN(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func (r *reader) skip(count int) error {
	if count < 0 {
		return fmt.Errorf("invalid skip count %d", count)
	}
	if count == 0 {
		return nil
	}
	if r.remaining() < count {
		return fmt.Errorf("failed to skip %d bytes from stream: %w", count, errEOF)
	}
	r.pos += count
	return nil
}

func (r *reader) seek(pos int) error {
	if pos < 0 || pos > len(r.data) {
		return fmt.Errorf("seek out of range: %d", pos)
	}
	r.pos = pos
	return nil
}

func (r *reader) readFullField(expected datatype) ([]byte, error) {
	dt, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	if datatype(dt) != expected {
		return nil, fmt.Errorf("expected datatype: %s | actual datatype: %s",
			expected, datatype(dt))
	}

	size, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	if err := r.seek(r.pos - 4); err != nil {
		return nil, err
	}

	data, err := r.readN(int(size) + 4)
	if err != nil {
		return nil, fmt.Errorf("failed to read %d bytes from stream: %w", size, err)
	}

	logger.Trace("XI5: read full field %s (%d bytes)", expected, len(data))
	return data, nil
}

func (r *reader) readField(expected datatype) ([]byte, error) {
	dt, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	if datatype(dt) != expected {
		return nil, fmt.Errorf("expected datatype: %s | actual datatype: %s",
			expected, datatype(dt))
	}

	size, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	data, err := r.readN(int(size))
	if err != nil {
		return nil, fmt.Errorf("failed to read %d bytes from stream: %w", size, err)
	}

	logger.Trace("XI5: read field %s (%d bytes)", expected, size)
	return data, nil
}

func (r *reader) readSection(expectedType uint16) ([]byte, error) {
	t, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	if t != expectedType {
		return nil, fmt.Errorf("expected XI5 section 0x%04X | actual section 0x%04X",
			expectedType, t)
	}

	size, err := r.readUShort()
	if err != nil {
		return nil, err
	}

	data, err := r.readN(int(size))
	if err != nil {
		return nil, fmt.Errorf("failed to read XI5 section 0x%04X with size %d: %w",
			t, size, err)
	}

	logger.Trace("XI5: read section 0x%04X (%d bytes)", t, size)
	return data, nil
}

func (r *reader) readBody() ([]byte, error)       { return r.readField(dtBody) }
func (r *reader) readFullBody() ([]byte, error)   { return r.readFullField(dtBody) }
func (r *reader) readFooter() ([]byte, error)     { return r.readField(dtFooter) }
func (r *reader) readFullFooter() ([]byte, error) { return r.readFullField(dtFooter) }
func (r *reader) readBinary() ([]byte, error)     { return r.readField(dtBinary) }
func (r *reader) readFullBinary() ([]byte, error) { return r.readFullField(dtBinary) }

func cString(data []byte) string {
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}
	return string(data)
}

func (r *reader) readBinaryAsString() (string, error) {
	data, err := r.readBinary()
	if err != nil {
		return "", err
	}
	return cString(data), nil
}

func (r *reader) readString() (string, error) {
	data, err := r.readField(dtString)
	if err != nil {
		return "", err
	}
	return cString(data), nil
}

func (r *reader) readUInt() (uint32, error) {
	data, err := r.readField(dtUInt)
	if err != nil {
		return 0, err
	}
	if len(data) < 4 {
		return 0, fmt.Errorf("UInt field too short: %d bytes", len(data))
	}
	return binary.BigEndian.Uint32(data), nil
}

func (r *reader) readULong() (uint64, error) {
	data, err := r.readField(dtULong)
	if err != nil {
		return 0, err
	}
	if len(data) < 8 {
		return 0, fmt.Errorf("ULong field too short: %d bytes", len(data))
	}
	return binary.BigEndian.Uint64(data), nil
}

func (r *reader) readTime() (time.Time, error) {
	data, err := r.readField(dtTime)
	if err != nil {
		return time.Time{}, err
	}
	if len(data) < 8 {
		return time.Time{}, fmt.Errorf("Time field too short: %d bytes", len(data))
	}
	ms := binary.BigEndian.Uint64(data)
	return time.UnixMilli(int64(ms)).UTC(), nil
}

type Ticket struct {
	TicketVersion string
	Serial        string
	IssuerID      uint32
	Issued        time.Time
	Expires       time.Time
	UserID        uint64
	OnlineID      string
	Region        string
	Domain        string
	ServiceID     string
	Status        uint32
	IssuerName    string

	fullBodyData []byte
	signature    []byte
}

func versionName(v uint32) string {
	switch v {
	case ver20:
		return "XI5_VER_2_0"
	case ver21:
		return "XI5_VER_2_1"
	case ver30:
		return "XI5_VER_3_0"
	case ver40:
		return "XI5_VER_4_0"
	}
	return "UNKNOWN"
}

func NewTicket(data []byte) (*Ticket, error) {
	logger.Debug("XI5: parsing ticket (%d bytes)", len(data))
	logger.Hex(logger.LevelTrace, "XI5 TICKET", data)

	if len(data) < 8 {
		logger.Error("XI5: ticket too short (%d bytes)", len(data))
		return nil, fmt.Errorf("%w: too short", ErrInvalidTicket)
	}

	ms := &reader{data: data}

	version, err := ms.readUInt32()
	if err != nil {
		logger.Error("XI5: failed to read version: %v", err)
		return nil, err
	}

	if version != ver20 && version != ver21 && version != ver30 && version != ver40 {
		logger.Error("XI5: invalid ticket version: %d", version)
		return nil, fmt.Errorf("%w: invalid ticket version: %d", ErrUnsupported, version)
	}

	t := &Ticket{TicketVersion: versionName(version)}
	logger.Debug("XI5: ticket version %s", t.TicketVersion)

	size, err := ms.readUInt32()
	if err != nil {
		logger.Error("XI5: failed to read ticket size: %v", err)
		return nil, err
	}

	if int64(size) != int64(ms.length())-8 {
		logger.Error("XI5: size mismatch, specified=%d actual=%d", size, ms.length()-8)
		return nil, fmt.Errorf("%w: specified ticket size: %d | actual ticket size: %d",
			ErrInvalidTicket, size, ms.length()-8)
	}

	switch version {
	case ver21:
		err = t.parseV21(ms)

	case ver30:
		err = t.parseV30(ms)

	case ver20, ver40:
		logger.Warn("XI5: %s tickets are not implemented, dumping to bad_xi5", t.TicketVersion)

		if mkErr := os.MkdirAll("bad_xi5", 0o755); mkErr != nil {
			logger.Error("XI5: failed to create bad_xi5 directory: %v", mkErr)
		} else {
			name := filepath.Join("bad_xi5", fmt.Sprintf("%d.bin", time.Now().UnixNano()))
			if wrErr := os.WriteFile(name, data, 0o644); wrErr != nil {
				logger.Error("XI5: failed to write %s: %v", name, wrErr)
			} else {
				logger.Info("XI5: saved unsupported ticket to %s", name)
			}
		}

		return nil, fmt.Errorf("%w: reading %s ticket", ErrNotImplemented, t.TicketVersion)

	default:
		logger.Error("XI5: unsupported ticket version: %s", t.TicketVersion)
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, t.TicketVersion)
	}

	if err != nil {
		logger.Error("XI5: failed to parse %s ticket: %v", t.TicketVersion, err)
		return nil, err
	}

	logger.Info("XI5: parsed ticket OnlineId=%s UserId=%d Version=%s",
		t.OnlineID, t.UserID, t.TicketVersion)
	logger.Debug("XI5: ticket details: %s", t.String())

	return t, nil
}

func (t *Ticket) parseCommon(body *reader) error {
	var err error

	if err = body.seek(4); err != nil {
		return err
	}

	if t.Serial, err = body.readBinaryAsString(); err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	logger.Trace("XI5: Serial=%s", t.Serial)

	if t.IssuerID, err = body.readUInt(); err != nil {
		return fmt.Errorf("issuer id: %w", err)
	}
	logger.Trace("XI5: IssuerId=0x%08X", t.IssuerID)

	if t.Issued, err = body.readTime(); err != nil {
		return fmt.Errorf("issued: %w", err)
	}
	logger.Trace("XI5: Issued=%s", t.Issued)

	if t.Expires, err = body.readTime(); err != nil {
		return fmt.Errorf("expires: %w", err)
	}
	logger.Trace("XI5: Expires=%s", t.Expires)

	if t.UserID, err = body.readULong(); err != nil {
		return fmt.Errorf("user id: %w", err)
	}
	logger.Trace("XI5: UserId=%d", t.UserID)

	if t.OnlineID, err = body.readString(); err != nil {
		return fmt.Errorf("online id: %w", err)
	}
	logger.Trace("XI5: OnlineId=%s", t.OnlineID)

	if t.Region, err = body.readBinaryAsString(); err != nil {
		return fmt.Errorf("region: %w", err)
	}
	logger.Trace("XI5: Region=%s", t.Region)

	if t.Domain, err = body.readString(); err != nil {
		return fmt.Errorf("domain: %w", err)
	}
	logger.Trace("XI5: Domain=%s", t.Domain)

	if t.ServiceID, err = body.readBinaryAsString(); err != nil {
		return fmt.Errorf("service id: %w", err)
	}
	logger.Trace("XI5: ServiceId=%s", t.ServiceID)

	return nil
}

func (t *Ticket) parseFooter(footerData []byte) error {
	footer := &reader{data: footerData}

	var err error

	if t.IssuerName, err = footer.readBinaryAsString(); err != nil {
		return fmt.Errorf("issuer name: %w", err)
	}
	logger.Trace("XI5: IssuerName=%s", t.IssuerName)

	if t.signature, err = footer.readBinary(); err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	logger.Hex(logger.LevelTrace, "XI5 SIGNATURE", t.signature)

	return nil
}

func (t *Ticket) parseV21(ms *reader) error {
	logger.Debug("XI5: parsing v2.1 ticket")

	var err error

	if t.fullBodyData, err = ms.readFullBody(); err != nil {
		return fmt.Errorf("body: %w", err)
	}

	footerData, err := ms.readFooter()
	if err != nil {
		return fmt.Errorf("footer: %w", err)
	}

	body := &reader{data: t.fullBodyData}

	if err = t.parseCommon(body); err != nil {
		return err
	}

	if t.Status, err = body.readUInt(); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	logger.Trace("XI5: Status=%d", t.Status)

	return t.parseFooter(footerData)
}

func (t *Ticket) parseV30(ms *reader) error {
	logger.Debug("XI5: parsing v3.0 ticket")

	var err error

	if t.fullBodyData, err = ms.readFullBody(); err != nil {
		return fmt.Errorf("body: %w", err)
	}

	footerData, err := ms.readFooter()
	if err != nil {
		return fmt.Errorf("footer: %w", err)
	}

	body := &reader{data: t.fullBodyData}

	if err = t.parseCommon(body); err != nil {
		return err
	}

	if _, err = body.readSection(0x3011); err != nil {
		return err
	}

	if t.Status, err = body.readUInt(); err != nil {
		return fmt.Errorf("status: %w", err)
	}
	logger.Trace("XI5: Status=%d", t.Status)

	if _, err = body.readSection(0x3010); err != nil {
		return err
	}

	if body.remaining() == 4 {
		logger.Trace("XI5: skipping 4 trailing bytes in v3.0 body")
		if err = body.skip(4); err != nil {
			return err
		}
	}

	if body.remaining() != 0 {
		return fmt.Errorf("unexpected data remaining in XI5 v3.0 body: %d bytes",
			body.remaining())
	}

	return t.parseFooter(footerData)
}

func (t *Ticket) SignedByOfficialRPCN() bool {
	if t.IssuerID != officialIssuerID {
		logger.Debug("XI5: issuer id 0x%08X is not official RPCN", t.IssuerID)
		return false
	}

	var sig struct{ R, S *big.Int }

	if _, err := asn1.Unmarshal(t.signature, &sig); err != nil {
		logger.Warn("XI5: failed to decode signature: %v", err)
		return false
	}

	hash := sha256.Sum224(t.fullBodyData)
	logger.Hex(logger.LevelTrace, "XI5 SHA224", hash[:])

	ok := rpcnCurve.verify(rpcnPubKey, hash[:], sig.R, sig.S)

	if ok {
		logger.Debug("XI5: signature verified for OnlineId=%s", t.OnlineID)
	} else {
		logger.Warn("XI5: signature verification FAILED for OnlineId=%s", t.OnlineID)
	}

	return ok
}

func (t *Ticket) String() string {
	var b strings.Builder

	b.WriteString("{\n")
	fmt.Fprintf(&b, "    TicketVersion = %s\n", t.TicketVersion)
	fmt.Fprintf(&b, "    Serial = %s\n", t.Serial)
	fmt.Fprintf(&b, "    IssuerId = %d\n", t.IssuerID)
	fmt.Fprintf(&b, "    Issued = %s\n", t.Issued)
	fmt.Fprintf(&b, "    Expires = %s\n", t.Expires)
	fmt.Fprintf(&b, "    UserId = %d\n", t.UserID)
	fmt.Fprintf(&b, "    OnlineId = %s\n", t.OnlineID)
	fmt.Fprintf(&b, "    Region = %s\n", t.Region)
	fmt.Fprintf(&b, "    Domain = %s\n", t.Domain)
	fmt.Fprintf(&b, "    ServiceId = %s\n", t.ServiceID)
	fmt.Fprintf(&b, "    Status = %d\n", t.Status)
	fmt.Fprintf(&b, "    IssuerName = %s\n", t.IssuerName)
	b.WriteString("}\n")

	return b.String()
}
