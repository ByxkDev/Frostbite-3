package battlelog

import (
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bf4/components"
	"bf4/logger"
	"bf4/server"
)

type Config struct {
	Addr     string 
	Host     string 
	Platform string 
}

func DefaultConfig() Config {
	return Config{Addr: ":80", Platform: "ps3"}
}

type api struct {
	cfg     Config
	started time.Time
}

func Start(cfg Config) (*http.Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = ":80"
	}
	if cfg.Platform == "" {
		cfg.Platform = "ps3"
	}
	a := &api{cfg: cfg, started: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.index)
	mux.HandleFunc("/bf4/servers/show/", a.serverShow)
	mux.HandleFunc("/bf4/warsawdetailedstatspopulate/", a.detailedStats)
	mux.HandleFunc("/bf4/warsawoverviewpopulate/", a.overview)
	mux.HandleFunc("/bf4/battlereport/loadgeneralreport/", a.battleReport)
	mux.HandleFunc("/api/servers", a.apiServers)
	mux.HandleFunc("/api/players", a.apiPlayers)
	mux.HandleFunc("/api/player/", a.apiPlayer)
	mux.HandleFunc("/api/status", a.apiStatus)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("battlelog api: %w", err)
	}
	srv := &http.Server{Handler: logRequests(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("[BATTLELOG] stopped: %v", err)
		}
	}()
	logger.Info("[BATTLELOG] API listening on %s (http://%s%s/)", cfg.Addr, cfg.Host, portSuffix(cfg.Addr))
	return srv, nil
}

func portSuffix(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "80" || port == "" {
		return ""
	}
	return ":" + port
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("[BATTLELOG] %s %s %s", r.RemoteAddr, r.Method, r.URL.RequestURI())
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"type": "error", "message": msg})
}

func pathParts(r *http.Request, prefix string) []string {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, "/")
	for i, p := range parts {
		if u, err := url.PathUnescape(p); err == nil {
			parts[i] = u
		}
	}
	return parts
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func stat(stats map[string]string, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := stats[k]; ok {
			return num(v)
		}
	}
	return 0
}

func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

var modeIDs = map[string]int64{
	"conquestlarge0":       64,
	"conquestsmall0":       1,
	"rushlarge0":           2,
	"squaddeathmatch0":     8,
	"teamdeathmatch0":      32,
	"gunmaster0":           512,
	"domination0":          1024,
	"capturetheflag0":      524288,
	"obliteration":         2097152,
	"airsuperiority0":      8388608,
	"elimination0":         16777216,
	"carrierassaultsmall0": 134217728,
	"carrierassaultlarge0": 67108864,
	"chainlink0":           34359738368,
	"squadobliteration0":   137438953472,
}

func modeID(mode string) int64 {
	if id, ok := modeIDs[strings.ToLower(mode)]; ok {
		return id
	}
	return 0
}

func mapCode(level string) string {
	if i := strings.LastIndex(level, "/"); i >= 0 {
		return level[i+1:]
	}
	return level
}

func serverGUID(g *server.Game) string {
	if g.PGID != "" {
		return g.PGID
	}
	return fmt.Sprintf("bf4emu-%08x", g.ID)
}

func findGame(key string) (*server.Game, bool) {
	for _, g := range server.Games.List() {
		if strings.EqualFold(serverGUID(g), key) || strings.EqualFold(g.PGID, key) || strconv.FormatUint(uint64(g.ID), 10) == key {
			return g, true
		}
	}
	return nil, false
}

func gameStateName(s uint32) string {
	switch s {
	case server.GameStateNew:
		return "NEW"
	case server.GameStateInitializing:
		return "INITIALIZING"
	case server.GameStatePreGame:
		return "PRE_GAME"
	case server.GameStateInGame:
		return "IN_GAME"
	case server.GameStatePostGame:
		return "POST_GAME"
	case server.GameStateDestructing:
		return "DESTRUCTING"
	}
	return "UNKNOWN"
}

func (a *api) serverURL(g *server.Game) string {
	return fmt.Sprintf("/bf4/servers/show/%s/%s/%s/", a.cfg.Platform, serverGUID(g), url.PathEscape(strings.ReplaceAll(g.Name, " ", "-")))
}

func ipString(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

func (a *api) serverInfo(g *server.Game) map[string]any {
	players := server.Games.Players(g.ID)
	m := mapCode(g.Level)
	mm := modeID(g.Mode)
	serverType := "OFFICIAL"
	if g.IsPeerHosted() {
		serverType = "PEER"
	}
	return map[string]any{
		"guid":           serverGUID(g),
		"gameId":         g.ID,
		"name":           g.Name,
		"description":    "",
		"platform":       a.cfg.Platform,
		"ip":             ipString(g.HostIP),
		"port":           g.HostPort,
		"map":            m,
		"mapMode":        mm,
		"mapModeName":    g.Mode,
		"mapVariant":     0,
		"level":          g.Level,
		"gameExpansion":  0,
		"gameExpansions": []int{0},
		"preset":         1,
		"region":         16,
		"country":        "NL",
		"ranked":         g.Settings&server.SettingRanked != 0,
		"official":       true,
		"hasPassword":    false,
		"punkbuster":     false,
		"fairfight":      false,
		"tickRate":       30,
		"serverType":     serverType,
		"state":          gameStateName(g.State),
		"version":        g.Version,
		"host":           g.HostDisplayName(),
		"slots": map[string]any{
			"1": map[string]int{"current": 0, "max": 10},                           
			"2": map[string]int{"current": len(players), "max": int(g.MaxPlayers)}, 
			"8": map[string]int{"current": 0, "max": 4},                           
		},
		"maps": map[string]any{
			"maps":         []map[string]any{{"map": m, "mapMode": mm}},
			"nextMapIndex": 0,
		},
		"settings": map[string]string{},
		"created":  g.Created.Unix(),
	}
}

func (a *api) serverPlayers(g *server.Game) []map[string]any {
	out := []map[string]any{}
	for _, p := range server.Games.Players(g.ID) {
		stats := components.PersonaStats(p.Name)
		out = append(out, map[string]any{
			"persona": map[string]any{
				"personaId":   strconv.FormatUint(p.PersonaID, 10),
				"personaName": p.Name,
				"user":        map[string]any{"username": p.Name},
			},
			"rank":   int(stat(stats, "rank")),
			"team":   p.Team,
			"slot":   p.Slot,
			"state":  p.State,
			"joined": p.Joined.Unix(),
		})
	}
	return out
}

func (a *api) serverShow(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r, "/bf4/servers/show/")
	if len(parts) < 2 {
		notFound(w, "usage: /bf4/servers/show/{platform}/{guid}/")
		return
	}
	g, ok := findGame(parts[1])
	if !ok {
		notFound(w, "server not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type": "success",
		"message": map[string]any{
			"SERVER_INFO":    a.serverInfo(g),
			"SERVER_PLAYERS": a.serverPlayers(g),
		},
	})
}

func (a *api) apiServers(w http.ResponseWriter, r *http.Request) {
	list := []map[string]any{}
	for _, g := range server.Games.List() {
		info := a.serverInfo(g)
		info["url"] = a.serverURL(g)
		list = append(list, info)
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": "success", "message": map[string]any{"servers": list}})
}

func kitScores(s map[string]string) map[string]float64 {
	return map[string]float64{
		"1":    stat(s, "sc_assault"),
		"2":    stat(s, "sc_engineer"),
		"8":    stat(s, "sc_recon"),
		"32":   stat(s, "sc_support"),
		"2048": stat(s, "sc_commander"),
	}
}

func generalStats(p components.Persona) map[string]any {
	s := components.PersonaStats(p.Name)
	kills := stat(s, "kills", "c___k_g")
	deaths := stat(s, "deaths", "c___d_g")
	kdr := kills
	if deaths > 0 {
		kdr = kills / deaths
	}
	fired := stat(s, "shotsFired", "c___sfw_g")
	hit := stat(s, "shotsHit", "c___shw_g")
	acc := 0.0
	if fired > 0 {
		acc = hit / fired * 100
	}
	wins := stat(s, "wins", "c_mwin__roo_g")
	losses := stat(s, "losses", "c_mlos__roo_g")
	wlr := wins
	if losses > 0 {
		wlr = wins / losses
	}
	timePlayed := stat(s, "timePlayed")
	score := stat(s, "score")
	spm := 0.0
	kpm := 0.0
	if timePlayed > 0 {
		spm = score / (timePlayed / 60)
		kpm = kills / (timePlayed / 60)
	}
	kits := kitScores(s)
	zeroKits := map[string]float64{"1": 0, "2": 0, "8": 0, "32": 0, "2048": 0}

	return map[string]any{
		"kills":                kills,
		"deaths":               deaths,
		"kdRatio":              round2(kdr),
		"killAssists":          stat(s, "killAssists", "c___ka_g"),
		"headshots":            stat(s, "headshots", "c___hsh_g"),
		"longestHeadshot":      stat(s, "longestHeadshot", "c___hsd_ghva"),
		"shotsFired":           fired,
		"shotsHit":             hit,
		"accuracy":             round2(acc),
		"wins":                 wins,
		"losses":               losses,
		"numRounds":            stat(s, "numRounds", "c___ro_g"),
		"wlRatio":              round2(wlr),
		"dogtagsTaken":         stat(s, "dogtagsTaken", "c___dt_g"),
		"revives":              stat(s, "revives", "c___r_g"),
		"heals":                stat(s, "heals", "c___h_g"),
		"repairs":              stat(s, "repairs", "c___re_g"),
		"resupplies":           stat(s, "resupplies", "c___rs_g"),
		"skill":                stat(s, "skill"),
		"elo":                  stat(s, "elo"),
		"score":                score,
		"rank":                 int(stat(s, "rank")),
		"timePlayed":           timePlayed,
		"scorePerMinute":       round2(spm),
		"killsPerMinute":       round2(kpm),
		"kitScores":            kits,
		"kitTimes":             zeroKits,
		"kitTimesInPercentage": zeroKits,
		"serviceStars":         map[string]float64{"1": 0, "2": 0, "8": 0, "32": 0, "2048": 0},
		"serviceStarsProgress": zeroKits,
		"gameModesScore": map[string]float64{
			"64":      stat(s, "sc_conquest"),
			"2":       stat(s, "sc_rush"),
			"32":      stat(s, "sc_deathmatch"),
			"1024":    stat(s, "sc_domination"),
			"2097152": stat(s, "sc_obliteration"),
		},
		"sc_general": stat(s, "sc_general"),
		"sc_team":    stat(s, "sc_team"),
		"sc_bonus":   stat(s, "sc_bonus"),
		"sc_squad":   stat(s, "sc_squad"),
		"sc_award":   stat(s, "sc_award"),
		"sc_unlock":  stat(s, "sc_unlock"),
		"sc_vehicle": stat(s, "sc_vehicleall"),
	}
}

func personaFromPath(w http.ResponseWriter, r *http.Request, prefix string) (components.Persona, bool) {
	parts := pathParts(r, prefix)
	if len(parts) < 1 {
		notFound(w, "usage: "+prefix+"{personaId}/{platform}/")
		return components.Persona{}, false
	}
	if id, err := strconv.ParseUint(parts[0], 10, 64); err == nil {
		if p, ok := components.FindPersona(id); ok {
			return p, true
		}
	}
	if p, ok := components.FindPersonaByName(parts[0]); ok {
		return p, true
	}
	notFound(w, "player not found")
	return components.Persona{}, false
}

func (a *api) detailedStats(w http.ResponseWriter, r *http.Request) {
	p, ok := personaFromPath(w, r, "/bf4/warsawdetailedstatspopulate/")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":    "success",
		"message": "OK",
		"data": map[string]any{
			"personaId":    strconv.FormatUint(p.ID, 10),
			"personaName":  p.Name,
			"generalStats": generalStats(p),
		},
	})
}

func (a *api) overview(w http.ResponseWriter, r *http.Request) {
	p, ok := personaFromPath(w, r, "/bf4/warsawoverviewpopulate/")
	if !ok {
		return
	}
	gs := generalStats(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"type":    "success",
		"message": "OK",
		"data": map[string]any{
			"overviewStats": gs,
			"viewedPersonaInfo": map[string]any{
				"personaId":   strconv.FormatUint(p.ID, 10),
				"personaName": p.Name,
				"namespace":   "ps3",
				"online":      p.Online,
			},
			"currentRankNeeded": map[string]any{"level": gs["rank"]},
		},
	})
}

func (a *api) battleReport(w http.ResponseWriter, r *http.Request) {
	notFound(w, "battle reports are not recorded on this server yet")
}

func (a *api) apiPlayers(w http.ResponseWriter, r *http.Request) {
	list := []map[string]any{}
	for _, p := range components.KnownPersonas() {
		list = append(list, a.playerEntry(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": "success", "message": map[string]any{"players": list}})
}

func (a *api) playerEntry(p components.Persona) map[string]any {
	id := strconv.FormatUint(p.ID, 10)
	return map[string]any{
		"personaId":   id,
		"personaName": p.Name,
		"online":      p.Online,
		"statsUrl":    "/bf4/warsawdetailedstatspopulate/" + id + "/1/",
		"overviewUrl": "/bf4/warsawoverviewpopulate/" + id + "/1/",
	}
}

func (a *api) apiPlayer(w http.ResponseWriter, r *http.Request) {
	p, ok := personaFromPath(w, r, "/api/player/")
	if !ok {
		return
	}
	e := a.playerEntry(p)
	e["stats"] = generalStats(p)
	writeJSON(w, http.StatusOK, map[string]any{"type": "success", "message": e})
}

func (a *api) apiStatus(w http.ResponseWriter, r *http.Request) {
	online := 0
	players := components.KnownPersonas()
	for _, p := range players {
		if p.Online {
			online++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type": "success",
		"message": map[string]any{
			"uptimeSeconds": int(time.Since(a.started).Seconds()),
			"servers":       len(server.Games.List()),
			"players":       len(players),
			"online":        online,
		},
	})
}

func (a *api) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/bf4/" && r.URL.Path != "/bf4" {
		notFound(w, "unknown endpoint")
		return
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
	<title>BF4 Battlelog</title><style>
	body{font-family:system-ui,sans-serif;background:#111;color:#ddd;margin:0;padding:16px;max-width:960px}
	h1{font-size:22px}h2{font-size:17px;margin-top:28px;border-bottom:1px solid #333;padding-bottom:6px}
	table{width:100%;border-collapse:collapse;font-size:14px}td,th{text-align:left;padding:6px 8px;border-bottom:1px solid #222}
	th{color:#888;font-weight:500}a{color:#6cf}.on{color:#5d5}.off{color:#777}code{color:#fc6}
	</style></head><body><h1>BF4 Battlelog (local)</h1>`)

	b.WriteString(`<h2>Servers</h2><table><tr><th>Name</th><th>Map</th><th>Mode</th><th>Players</th><th>State</th></tr>`)
	games := server.Games.List()
	if len(games) == 0 {
		b.WriteString(`<tr><td colspan="5" class="off">No servers</td></tr>`)
	}
	for _, g := range games {
		fmt.Fprintf(&b, `<tr><td><a href="%s?json=1">%s</a></td><td>%s</td><td>%s</td><td>%d/%d</td><td>%s</td></tr>`,
			html.EscapeString(a.serverURL(g)), html.EscapeString(g.Name), html.EscapeString(mapCode(g.Level)),
			html.EscapeString(g.Mode), server.Games.PlayerCount(g), g.MaxPlayers, gameStateName(g.State))
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h2>Players</h2><table><tr><th>Name</th><th>Persona ID</th><th>Status</th><th>Rank</th><th>K/D</th></tr>`)
	players := components.KnownPersonas()
	sort.SliceStable(players, func(i, j int) bool { return players[i].Online && !players[j].Online })
	if len(players) == 0 {
		b.WriteString(`<tr><td colspan="5" class="off">No players yet</td></tr>`)
	}
	for _, p := range players {
		gs := generalStats(p)
		status, cls := "offline", "off"
		if p.Online {
			status, cls = "online", "on"
		}
		fmt.Fprintf(&b, `<tr><td><a href="/bf4/warsawdetailedstatspopulate/%d/1/">%s</a></td><td>%d</td><td class="%s">%s</td><td>%v</td><td>%v</td></tr>`,
			p.ID, html.EscapeString(p.Name), p.ID, cls, status, gs["rank"], gs["kdRatio"])
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h2>Endpoints</h2><table>
	<tr><td><code>/bf4/servers/show/{platform}/{guid}/?json=1</code></td><td>server info + players</td></tr>
	<tr><td><code>/bf4/warsawdetailedstatspopulate/{personaId}/1/</code></td><td>player stats</td></tr>
	<tr><td><code>/bf4/warsawoverviewpopulate/{personaId}/1/</code></td><td>player overview</td></tr>
	<tr><td><code>/api/servers</code></td><td>all servers</td></tr>
	<tr><td><code>/api/players</code></td><td>all players</td></tr>
	<tr><td><code>/api/player/{name}</code></td><td>one player by name or id</td></tr>
	<tr><td><code>/api/status</code></td><td>server status</td></tr>
	</table></body></html>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}