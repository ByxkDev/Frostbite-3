package components

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/logger"
)

const (
	Stats         uint16 = 0x0007
	Component0801 uint16 = 0x0801
	Inventory     uint16 = 0x0803
)

var UsersDir = "users"
var ItemsFile = "data/items.txt"

func HandleComponent0801(p blaze.Packet) []byte {
	switch p.Command {
	case 0x0032:
		return handleRspGetConfig(p)
	case 0x0014:
		return handleComponent0801Cmd20(p)
	default:
		logger.Warn("0801: unknown command %d", p.Command)
		logger.Hex(logger.LevelWarn, "0801 RAW", p.Payload)
		return nil
	}
}

type grantEntitlement2Request struct {
	GNAM string `tdf:"GNAM"`
	PJID string `tdf:"PJID"`
	PRID string `tdf:"PRID"`
	TAG  string `tdf:"TAG"`
	TYPE uint64 `tdf:"TYPE"`
}

type grantEntitlement2Response struct {
	ENTI entitlement `tdf:"ENTI"`
	ISGR bool        `tdf:"ISGR"`
}

var (
	grantedMu sync.Mutex
	granted   []entitlement
)

func handleAuthGrantEntitlement2(p blaze.Packet) []byte {
	var req grantEntitlement2Request
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("AUTH: GrantEntitlement2 decode failed: %v", err)
	}

	grantedMu.Lock()
	ent := entitlement{
		GNAM: req.GNAM,
		ID:   uint64(len(ps3Entitlements) + len(granted) + 1),
		PID:  currentPersonaID(),
		PJID: req.PJID,
		PRCA: 2,
		PRID: req.PRID,
		STAT: 1,
		TAG:  req.TAG,
		TYPE: req.TYPE,
	}
	exists := false
	for _, g := range granted {
		if g.TAG == ent.TAG && g.GNAM == ent.GNAM {
			ent, exists = g, true
			break
		}
	}
	if !exists {
		granted = append(granted, ent)
	}
	grantedMu.Unlock()

	logger.Info("AUTH: GrantEntitlement2 GNAM=%q TAG=%q (new=%t)", ent.GNAM, ent.TAG, !exists)
	return encodeReply(p, &grantEntitlement2Response{ENTI: ent, ISGR: true}, "Auth GrantEntitlement2")
}

func grantedEntitlements() []entitlement {
	grantedMu.Lock()
	defer grantedMu.Unlock()
	return append([]entitlement(nil), granted...)
}

type userSettingsLoadRequest struct {
	KEY string `tdf:"KEY"`
	UID uint64 `tdf:"UID"`
}

type userSettingsLoadResponse struct {
	DATA string `tdf:"DATA"`
	KEY  string `tdf:"KEY"`
}

type userSettingsSaveRequest struct {
	DATA string `tdf:"DATA"`
	KEY  string `tdf:"KEY"`
	UID  uint64 `tdf:"UID"`
}

type userSettingsLoadAllResponse struct {
	SMAP map[string]string `tdf:"SMAP"`
}

var settingsMu sync.Mutex

func settingsPath() string {
	name := GetConsoleOnlineID()
	if name == "" {
		name = "unknown"
	}
	name = strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, name)
	return filepath.Join(UsersDir, name, "settings.json")
}

func loadSettings() map[string]string {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	m := map[string]string{}
	b, err := os.ReadFile(settingsPath())
	if err == nil {
		if err := json.Unmarshal(b, &m); err != nil {
			logger.Warn("UTIL: settings file unreadable: %v", err)
		}
	}
	return m
}

func saveSetting(key, data string) {
	m := loadSettings()
	m[key] = data

	settingsMu.Lock()
	defer settingsMu.Unlock()

	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Error("UTIL: cannot create %s: %v", filepath.Dir(path), err)
		return
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		logger.Error("UTIL: cannot write %s: %v", path, err)
	}
}

func handleUtilUserSettingsLoad(p blaze.Packet) []byte {
	var req userSettingsLoadRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("UTIL: UserSettingsLoad decode failed: %v", err)
	}
	data := loadSettings()[req.KEY]
	logger.Info("UTIL: UserSettingsLoad KEY=%q (%d bytes)", req.KEY, len(data))
	return encodeReply(p, &userSettingsLoadResponse{DATA: data, KEY: req.KEY}, "Util UserSettingsLoad")
}

func handleUtilUserSettingsSave(p blaze.Packet) []byte {
	var req userSettingsSaveRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("UTIL: UserSettingsSave decode failed: %v", err)
	}
	logger.Info("UTIL: UserSettingsSave KEY=%q (%d bytes)", req.KEY, len(req.DATA))
	saveSetting(req.KEY, req.DATA)
	return encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
}

func handleUtilUserSettingsLoadAll(p blaze.Packet) []byte {
	m := loadSettings()
	logger.Info("UTIL: UserSettingsLoadAll %d keys from %s", len(m), settingsPath())
	return encodeReply(p, &userSettingsLoadAllResponse{SMAP: m}, "Util UserSettingsLoadAll")
}

type setUserModeRequest struct {
	MODE uint64 `tdf:"MODE"`
	STAT uint64 `tdf:"STAT"`
}

func handleUtilSetUserMode(p blaze.Packet) []byte {
	var req setUserModeRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("UTIL: SetUserMode decode failed: %v", err)
	}
	logger.Info("UTIL: SetUserMode MODE=%d STAT=%d -> empty reply", req.MODE, req.STAT)
	return encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
}

type statGroupRequest struct {
	NAME string `tdf:"NAME"`
}

type statDesc struct {
	CATG string `tdf:"CATG"`
	DFLT string `tdf:"DFLT"`
	FRMT string `tdf:"FRMT"`
	NAME string `tdf:"NAME"`
	TYPE uint64 `tdf:"TYPE"`
}

type statGroupResponse struct {
	CNAM string     `tdf:"CNAM"`
	DESC string     `tdf:"DESC"`
	NAME string     `tdf:"NAME"`
	STAT []statDesc `tdf:"STAT"`
}

var StatsDir = "data/stats"

var statCategories = []string{
	"player_awards",
	"player_awardsDogTags",
	"player_awardsXP",
	"player_core",
	"player_statcategory",
	"player_unknown",
	"player_weapons1",
	"player_weaponsxp",
	"spplayer_singleplayer",
}

const statDefault = "0.00"

var (
	statDescsOnce sync.Once
	statDescs     []statDesc
)

func loadStatDescs() []statDesc {
	statDescsOnce.Do(func() {
		for _, cat := range statCategories {
			path := filepath.Join(StatsDir, cat+".txt")
			f, err := os.Open(path)
			if err != nil {
				logger.Warn("STATS: missing %s: %v", path, err)
				continue
			}
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				name := strings.TrimSpace(sc.Text())
				if name == "" {
					continue
				}
				statDescs = append(statDescs, statDesc{CATG: cat, DFLT: statDefault, FRMT: "%.2f", NAME: name, TYPE: 1})
			}
			f.Close()
		}
		logger.Info("STATS: loaded %d stat names from %s", len(statDescs), StatsDir)
	})
	return statDescs
}

func userStatValues() []string {
	descs := loadStatDescs()

	saved := map[string]string{}
	path := filepath.Join(filepath.Dir(settingsPath()), "stats.txt")
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
				saved[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		f.Close()
	}

	values := make([]string, len(descs))
	for i, d := range descs {
		if v, ok := saved[d.NAME]; ok {
			values[i] = v
		} else {
			values[i] = statDefault
		}
	}
	return values
}

func handleStatsGetStatGroup(p blaze.Packet) []byte {
	var req statGroupRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("STATS: getStatGroup decode failed: %v", err)
	}

	resp := statGroupResponse{CNAM: statCategories[0], DESC: req.NAME, NAME: req.NAME, STAT: loadStatDescs()}
	logger.Info("STATS: getStatGroup %q -> %d stats", req.NAME, len(resp.STAT))
	return encodeReply(p, &resp, "Stats getStatGroup")
}

type statsByGroupRequest struct {
	EID  []uint64 `tdf:"EID"`
	NAME string   `tdf:"NAME"`
	VID  uint64   `tdf:"VID"`
}

type entityStats struct {
	EID  uint64   `tdf:"EID"`
	STAT []string `tdf:"STAT"`
}

type statValues struct {
	STAT []entityStats `tdf:"STAT"`
}

type statsByGroupNotify struct {
	GRNM string     `tdf:"GRNM"`
	KEY  string     `tdf:"KEY"`
	LAST bool       `tdf:"LAST"`
	STS  statValues `tdf:"STS"`
	VID  uint64     `tdf:"VID"`
}

func handleStatsGetStatsByGroupAsync(p blaze.Packet) []byte {
	var req statsByGroupRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("STATS: getStatsByGroupAsync decode failed: %v", err)
	}

	eids := req.EID
	if len(eids) == 0 {
		eids = []uint64{currentPersonaID()}
	}

	values := userStatValues()
	var entities []entityStats
	for _, eid := range eids {
		entities = append(entities, entityStats{EID: eid, STAT: values})
	}

	notify := statsByGroupNotify{
		GRNM: req.NAME,
		KEY:  "No_Scope_Defined",
		LAST: true,
		STS:  statValues{STAT: entities},
		VID:  req.VID,
	}
	payload, err := component35TdfEncoder.Encode(&notify)
	if err != nil {
		logger.Error("STATS: encode 0x0032 failed: %v", err)
		return nil
	}

	logger.Info("STATS: getStatsByGroupAsync %q EIDs=%v VID=%d -> %d values", req.NAME, eids, req.VID, len(values))
	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
	return append(reply, encodeBlazePacket(Stats, 0x0032, 0, blazeTypeNotification, 0, payload)...)
}

func HandleStats(p blaze.Packet) []byte {
	switch p.Command {
	case 0x0004:
		return handleStatsGetStatGroup(p)
	case 0x0010:
		return handleStatsGetStatsByGroupAsync(p)
	default:
		logger.Warn("STATS: unknown command %d (0x%04X)", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "STATS RAW", p.Payload)
		return nil
	}
}

type getItemsRequest struct {
	CFLG uint64 `tdf:"CFLG"`
	HIST uint64 `tdf:"HIST"`
	RTYP uint64 `tdf:"RTYP"`
	UID  uint64 `tdf:"UID"`
}

type consumable struct {
	ACTT uint64 `tdf:"ACTT"`
	CKEY string `tdf:"CKEY"`
	DURA uint64 `tdf:"DURA"`
	QANT uint64 `tdf:"QANT"`
}

type inventoryItems struct {
	CLST []consumable `tdf:"CLST"`
	ULST []string     `tdf:"ULST"`
}

type getItemsResponse struct {
	INVT inventoryItems `tdf:"INVT"`
}

var (
	itemsOnce sync.Once
	itemKeys  []string
)

func loadItems() []string {
	itemsOnce.Do(func() {
		f, err := os.Open(ItemsFile)
		if err != nil {
			logger.Info("INVENTORY: no %s, inventory will be empty", ItemsFile)
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if s := strings.TrimSpace(sc.Text()); s != "" {
				itemKeys = append(itemKeys, s)
			}
		}
		logger.Info("INVENTORY: loaded %d items from %s", len(itemKeys), ItemsFile)
	})
	return itemKeys
}

func handleInventoryGetItems(p blaze.Packet) []byte {
	var req getItemsRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("INVENTORY: getItems decode failed: %v", err)
	}

	var resp getItemsResponse
	if req.CFLG == 2 {
		resp.INVT.CLST = userConsumables()
	} else {
		resp.INVT.ULST = append([]string{}, loadItems()...)
	}

	logger.Info("INVENTORY: getItems CFLG=%d RTYP=%d -> %d items, %d consumables", req.CFLG, req.RTYP, len(resp.INVT.ULST), len(resp.INVT.CLST))
	return encodeReply(p, &resp, "Inventory getItems")
}

func HandleInventory(p blaze.Packet) []byte {
	switch p.Command {
	case 0x0001:
		return handleInventoryGetItems(p)
	case 0x0003:
		return handleInventoryUseConsumable(p)
	case 0x0006:
		return handleInventoryGetTemplate(p)
	default:
		logger.Warn("INVENTORY: unknown command %d (0x%04X)", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "INVENTORY RAW", p.Payload)
		return nil
	}
}

var (
	ConsumablesFile     = "data/consumables.txt"
	DefaultConsumables  = map[string]uint64{"ulte_boost_25": 99, "ulte_boost_50": 99, "ulte_boost_100": 99, "ulte_boost_200": 99}
	ConsumableDuration  = uint64(3600)
	consumablesMu       sync.Mutex
	consumableOrderOnce sync.Once
	consumableOrder     []string
	consumableGrants    map[string]uint64
)

type ownedConsumable struct {
	Key       string `json:"key"`
	Qty       uint64 `json:"qty"`
	Activated int64  `json:"activated,omitempty"`
}

func consumableDefaults() ([]string, map[string]uint64) {
	consumableOrderOnce.Do(func() {
		consumableGrants = map[string]uint64{}
		if f, err := os.Open(ConsumablesFile); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				k, v, _ := strings.Cut(strings.TrimSpace(sc.Text()), "=")
				k = strings.TrimSpace(k)
				if k == "" || strings.HasPrefix(k, "#") {
					continue
				}
				var n uint64 = 1
				fmt.Sscan(strings.TrimSpace(v), &n)
				if _, seen := consumableGrants[k]; !seen {
					consumableOrder = append(consumableOrder, k)
				}
				consumableGrants[k] = n
			}
			f.Close()
		}
		if len(consumableOrder) == 0 {
			for _, k := range []string{"ulte_boost_25", "ulte_boost_50", "ulte_boost_100", "ulte_boost_200"} {
				consumableOrder = append(consumableOrder, k)
				consumableGrants[k] = DefaultConsumables[k]
			}
		}
	})
	return consumableOrder, consumableGrants
}

func consumablesPath() string {
	return filepath.Join(filepath.Dir(settingsPath()), "consumables.json")
}

func loadConsumables() []ownedConsumable {
	var owned []ownedConsumable
	if b, err := os.ReadFile(consumablesPath()); err == nil {
		if err := json.Unmarshal(b, &owned); err != nil {
			logger.Warn("INVENTORY: consumables file unreadable: %v", err)
		}
	}
	order, grants := consumableDefaults()
	have := map[string]bool{}
	for _, c := range owned {
		have[c.Key] = true
	}
	added := false
	for _, k := range order {
		if !have[k] {
			owned = append(owned, ownedConsumable{Key: k, Qty: grants[k]})
			added = true
		}
	}
	if added {
		saveConsumables(owned)
	}
	return owned
}

func saveConsumables(owned []ownedConsumable) {
	path := consumablesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Error("INVENTORY: cannot create %s: %v", filepath.Dir(path), err)
		return
	}
	b, _ := json.MarshalIndent(owned, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		logger.Error("INVENTORY: cannot write %s: %v", path, err)
	}
}

func (c ownedConsumable) toTDF(uid uint64) consumable {
	var left uint64
	if c.Activated > 0 {
		if el := uint64(time.Now().Unix() - c.Activated); el < ConsumableDuration {
			left = ConsumableDuration - el
		}
	}
	return consumable{ACTT: uid, CKEY: c.Key, DURA: left, QANT: c.Qty}
}

func userConsumables() []consumable {
	consumablesMu.Lock()
	owned := loadConsumables()
	consumablesMu.Unlock()

	uid := currentPersonaID()
	out := make([]consumable, 0, len(owned))
	for _, c := range owned {
		out = append(out, c.toTDF(uid))
	}
	return out
}

type useConsumableResponse struct {
	CNSU consumable `tdf:"CNSU"`
	UID  uint64     `tdf:"UID"`
}

func handleInventoryUseConsumable(p blaze.Packet) []byte {
	key := tdfFindString(p.Payload, "IKEY")

	consumablesMu.Lock()
	owned := loadConsumables()
	idx := -1
	for i := range owned {
		if owned[i].Key == key {
			idx = i
		}
	}
	if idx < 0 || owned[idx].Qty == 0 {
		consumablesMu.Unlock()
		logger.Warn("INVENTORY: useConsumable %q: not owned or none left", key)
		return encodeBlazePacket(p.Component, p.Command, 0x0001, 0x3000, uint32(p.MessageId), nil)
	}
	for i := range owned {
		owned[i].Activated = 0
	}
	owned[idx].Qty--
	owned[idx].Activated = time.Now().Unix()
	saveConsumables(owned)
	used := owned[idx]
	consumablesMu.Unlock()

	uid := currentPersonaID()
	body := &useConsumableResponse{CNSU: used.toTDF(uid), UID: uid}
	reply := encodeReply(p, body, "Inventory useConsumable")
	payload, err := component35TdfEncoder.Encode(body)
	if err != nil || reply == nil {
		return reply
	}
	return append(reply, encodeBlazePacket(Inventory, 0x000A, 0, blazeTypeNotification, 0, payload)...)
}

type inventoryTemplateResponse struct {
	ILST []string `tdf:"ILST"`
}

func handleInventoryGetTemplate(p blaze.Packet) []byte {
	order, _ := consumableDefaults()
	list := append([]string{}, loadItems()...)
	list = append(list, grantedPackKeys()...)
	list = append(list, order...)
	return encodeReply(p, &inventoryTemplateResponse{ILST: list}, "Inventory getTemplate")
}

func tdfFindString(payload []byte, tag string) string {
	m := blaze.MustTdfMember(tag)
	i := bytes.Index(payload, append(m.Bytes[:], 0x01))
	if i < 0 {
		return ""
	}
	n, k := tdfReadVarint(payload[i+4:])
	start := i + 4 + k
	if k == 0 || n == 0 || start+int(n) > len(payload) {
		return ""
	}
	return string(payload[start : start+int(n)-1])
}
