# BF4 Blaze Emulator

[![Status](https://img.shields.io/badge/status-WIP-orange)](https://github.com/ByxkDev/Frostbite-3)
[![Language](https://img.shields.io/badge/language-Go-blue)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-PS3-lightgrey)](https://www.playstation.com/)

**Syntax BF4 Discord Server:** https://discord.gg/pFaTAHA7dg

---

# ⚠️ Work In Progress

This project is an experimental replacement backend for **Battlefield 4 (PlayStation 3)**.

The goal is to recreate the EA backend infrastructure so that the original Battlefield 4 PS3 client can talk to a custom server **without modifying the game executable**.

## Where we are now

The original PS3 client now gets **all the way into the game**:

* ✅ Boots, connects and logs in to our Blaze backend
* ✅ Reaches the **main menu** and the multiplayer menus
* ✅ Loads soldier, stats, entitlements, inventory, packs and user settings
* ✅ **Test Range is playable**
* ✅ Server browser, matchmaking and join flow are handled by our GameManager
* ✅ A headless **dedicated server** logs in to Blaze, registers a game and accepts joining players
* ⏳ **Real multiplayer matches are not playable yet** the client reaches the dedicated server's game port, but the in-game Frostbite network protocol (UDP) still has to be reverse engineered and written from scratch

The Blaze side (everything up to "join game") is largely done. The current wall is the **Frostbite game protocol** that EA's own dedicated servers spoke see [The Frostbite Game Protocol](#the-frostbite-game-protocol--the-24-byte-connection-request).

---

# Architecture

```text
                         Battlefield 4 (PS3, unmodified)
                                       |
                     DNS: gosredirector.ea.com -> our server
                                       |
        +------------------------------+--------------------------------+
        |                              |                                |
        | TLS (ProtoSSL)               | HTTP                           | UDP game traffic
        v                              v                                v
 GOS Redirector :42127          Nucleus / Battlelog :80          Dedicated server :25210
        |                       (PS3 login, stats API)           (Frostbite game protocol)
        | GetServerInstance                                             ^
        v                                                               |
 Blaze server :33152  <---------- Blaze TCP (as "warsaw server") -------+
   Util / Auth / UserSessions / Stats / Inventory / Packs / GameManager ...
        |
        v
 QoS listener :17502
```

| Service              | Port            | Protocol   | Source                                   |
| -------------------- | --------------- | ---------- | ---------------------------------------- |
| GOS Redirector       | `42127`         | TLS / TCP  | `main.go`, `network/redirector/`         |
| Blaze server         | `33152`         | TCP        | `main.go`, `components/`, `blaze/`       |
| QoS / host listeners | `17502`         | UDP / TCP  | `server/host.go`                         |
| Nucleus (PS3 login)  | `80`            | HTTP       | `network/nucleus/`                       |
| Battlelog API        | `80`            | HTTP       | `network/battlelog/`                     |
| Dedicated server     | `25210`         | UDP + TCP  | `server/dedicated/`, `cmd/bf4host/`      |

---

# Login Flow (working)

```text
DNS redirection
      |
      v
GOS Redirector (TLS)  ->  GetServerInstance
      |
      v
Blaze server :33152
      |
      +-- Util PreAuth / Ping / FetchClientConfig
      +-- ConsoleAuthentication (comp 35, cmd 10)  <- PSN XI5 ticket
      |      -> UserSessions notifications (ExtendedData / UserAdded / UserUpdated)
      +-- Util PostAuth / GetTelemetryServer
      +-- Authentication ListUserEntitlements2 / GetAuthToken / GrantEntitlement2
      +-- UserSessions UpdateNetworkInfo / SetUserInfoAttribute / ResumeSession
      +-- Util UserSettingsLoad / Save / LoadAll / SetUserMode / SetClientMetrics
      +-- Stats GetStatGroup / GetStatsByGroupAsync
      +-- Inventory GetItems, Packs, AssociationLists
      |
      v
Main menu  ->  Test Range  |  Server browser / Matchmaking  ->  JoinGame
                                                                   |
                                                                   v
                                                   Dedicated server game port
                                                   (Frostbite protocol - WIP)
```

# The Frostbite Game Protocol — the 24-byte connection request

This is the **main open problem** of the project.

On EA's original infrastructure, Blaze only handles the "lobby" side: login, server browser, matchmaking and putting a player into a game. The actual match is run by a **Frostbite dedicated server** (the "warsaw server"). The PS3 talks to that server over **UDP using Frostbite's own game protocol**, which is completely separate from Blaze/TDF.

**No public implementation or documentation of BF4's game protocol exists**, so it has to be reverse engineered and most likely written from scratch.

### What we know so far

When a PS3 joins a game, Blaze tells it the dedicated server's address. The first thing the PS3 sends to the game port is a **24-byte connection request**, repeated until it gets an answer:

```text
offset  size  meaning
------  ----  -------------------------------------------------
0       1     counter   - increases by 3 on every retry
1       1     0x80      - constant flag
2..23   22    encrypted payload
```

Example of what the dedicated server logs:

```text
DEDICATED:   1.2.3.4:3659 (Player) connection request #1:  counter=N   flag=0x80, 22 encrypted bytes
DEDICATED:   1.2.3.4:3659 (Player) connection request #5:  counter=N+12 flag=0x80, 22 encrypted bytes
```

Observations:

* Byte 0 is a **counter that rises by 3 per retry** (wrapping as a byte), not a fixed sequence number. This looks like a sequence/handshake counter rather than a packet type.
* Byte 1 is always **`0x80`**, most likely a "connection / handshake" packet type flag.
* The remaining **22 bytes are encrypted**. The key/scheme is not known yet. Likely candidates are a key derived from Blaze data the client already received (game ID, persona ID, session key, the TOKN/UUID from the join notification) or a key exchange that the server is expected to start.
* Without a correct answer from the server, the PS3 keeps retrying and eventually times out back to the menu.

### What the dedicated server does today

`server/dedicated/` is a complete Blaze-side dedicated server:

1. Logs in to our Blaze server as the **"warsaw server"** account
2. Creates the game (`CreateGame` → `FinalizeGameCreation` → `AdvanceGameState` to `IN_GAME`)
3. Opens the game port (UDP + TCP, default `25210`)
4. Receives `NotifyPlayerJoining` / `NotifyPlayerClaimingReservation` and waits for the player
5. Hands **every UDP packet** on the game port to a pluggable `GameEngine`
6. Reports the player back to Blaze with `UpdateMeshConnection` (connected / disconnected)

The engine interface is where the Frostbite protocol will live:

```go
type GameEngine interface {
    Name() string
    HandlePacket(from *net.UDPAddr, player string, data []byte) (replies [][]byte, connected bool)
    Capture(proto, from string, data []byte)
}
```

Right now the only engine is the **`CaptureEngine`**: it answers nothing, decodes the 24-byte request header (counter / flag) and writes all game-port traffic to `data/captures/dedicated_<timestamp>.tsv` (`time  proto  from  player  len  hex`) for analysis.

`-fake-connect` exists for **testing only**: it reports players as connected on their first packet so the Blaze side of the flow can be tested past the join.

### What still needs to be done here

* Collect many captures of the connection request (different players, games, retries)
* Find the encryption used on the 22-byte payload in the PS3 `EBOOT.ELF`
* Work out what the server must reply to complete the handshake
* Reverse engineer the post-handshake packet stream (ghosts / replication, player input, level loading, chat, etc.)
* Write a real `GameEngine` implementation effectively a **Frostbite 3 server from scratch**

Help with this part is **very** welcome see [Contributing](#contributing).

---

# TLS / ProtoSSL

The client successfully:

* Connects to the custom redirector
* Sends a TLS ClientHello
* Negotiates a compatible legacy TLS configuration
* Accepts the generated certificate
* Completes the handshake and continues into Blaze

```text
TLS 1.0 - TLS 1.2

TLS_RSA_WITH_AES_128_CBC_SHA
TLS_RSA_WITH_AES_256_CBC_SHA
```

The configuration is built around the limitations of EA's legacy **ProtoSSL** used by Battlefield 4 PS3.

---

# Certificate Generation

The server loads its certificate from:

```text
network/certificates/gosredirector_mod.pfx   (password set in main.go)
```

**The certificate is not included in this repo. Use the Bug_OldProtoSSL guide from Aim4Kill's GitHub**, together with `network/certificates/generate.bat`.

At runtime the server extracts the certificate and RSA private key from the PFX via PowerShell and converts them for Go's TLS stack.

---

# DNS Redirection

The game connects to:

```text
gosredirector.ea.com
```

Point it to your server with a custom DNS record:

```text
gosredirector.ea.com -> 151.xxx.xxx.xx
```

No EBOOT modification is required.

---

# No Client Binary Modification

The emulator works without:

* Modified EBOOT files
* Patched executables
* Altered game assets
* Custom network libraries

Compatibility comes purely from DNS redirection, TLS compatibility and recreating the server-side protocols.

---

# Authentication & PSN

* `components/authentication.go` — ConsoleAuthentication / login handling
* `psn/xi5ticket.go` — parses the PSN **XI5 ticket** the PS3 sends (online ID, account data, signature)
* `network/nucleus/` — HTTP endpoints for the PS3 Nucleus login (`/ps3.php`, `/success`)
* Persona IDs are derived from the PSN online ID, so every player keeps the same persona between sessions

RPCN is **not supported yet**; the XI5 parser is the groundwork for it.

---

# Battlelog API

`network/battlelog/` serves a small Battlelog-style HTTP API and status page

this is not working yet.

---

# Logging

A full logging system is used throughout the server (`logger/logger.go`) with `DEBUG`, `INFO`, `TRACE`, `WARN` and `ERROR` levels, hex dumps of unknown packets, and TDF field tracing. Unknown Blaze commands are always logged with their raw payload, which is how most new commands are discovered.

---

# Project Structure

```text
Frostbite-3/
├── README.md
├── LICENSE
├── strings.txt                     # strings extracted from the BF4 PS3 EBOOT.ELF
│
└── Frostbite3/
    └── Battlefield4/
        ├── main.go                 # entry point: redirector, Blaze, QoS, Nucleus, Battlelog, dedicated server
        ├── go.mod / go.sum
        │
        ├── blaze/
        │   ├── packet.go           # Blaze packet header parsing
        │   └── tdf.go              # TDF encoder / decoder
        │
        ├── components/             # Blaze component handlers
        │   ├── components.go       # dispatcher, Util, Redirector, ConsoleAuthentication
        │   ├── authentication.go   # login / authentication
        │   ├── session.go          # UserSessions, AssociationLists, PostAuth, telemetry, entitlements
        │   ├── mysoldier.go        # Stats, Inventory, Packs, user settings, entitlements, 0x0801
        │   ├── gamemanager.go      # server browser, matchmaking, join / create / destroy game
        │   ├── host.go             # game host sessions
        │   ├── peer.go             # peer-hosted game settings
        │   ├── clientsession.go    # per-client session state
        │   ├── conn.go             # connection tracking + push notifications
        │   └── battlelog.go        # persona store used by the Battlelog API
        │
        ├── server/
        │   ├── game.go             # game registry / state
        │   ├── types.go            # shared GameManager TDF types
        │   ├── config.go           # game config (data/games.json)
        │   ├── host.go             # QoS / host listeners + traffic capture
        │   └── dedicated/          # headless dedicated server ("warsaw server")
        │       ├── dedicated.go    # Blaze login, game registration, game port, player tracking
        │       ├── blaze.go        # Blaze client
        │       ├── types.go        # dedicated server TDF types
        │       └── engine.go       # GameEngine interface + CaptureEngine (24-byte request decoding)
        │
        ├── cmd/
        │   └── bf4host/
        │       └── main.go         # standalone dedicated server binary
        │
        ├── network/
        │   ├── certificates/
        │   │   └── generate.bat
        │   ├── redirector/
        │   │   └── redirector.go   # GetServerInstance
        │   ├── nucleus/
        │   │   └── nucleus.go      # PS3 Nucleus HTTP login
        │   └── battlelog/
        │       └── battlelog.go    # Battlelog HTTP API
        │
        ├── psn/
        │   └── xi5ticket.go        # PSN XI5 ticket parser
        │
        ├── logger/
        │   └── logger.go
        │
        ├── utilities/
        │   └── utilities.go        # PreAuth / Ping / FetchClientConfig builders
        │
        └── data/
            ├── games.json          # listed games
            ├── items.txt           # inventory / unlocks
            └── stats/              # stat groups (core, weapons, awards, dog tags, singleplayer, ...)
```

---

# Requirements

* Go 1.20+
* Battlefield 4 PS3 v1.20
* Windows PowerShell (for loading the PFX)
* DNS server capable of custom records
* Server / VPS with these ports open: `42127`, `33152`, `17502`, `80`, `25210`

---

# Running

1. Create the certificate (see [Certificate Generation](#certificate-generation)) and place it in `network/certificates/`.
2. Set your public IP / hostnames in `main.go` and `network/redirector/redirector.go`.
3. Start everything from `Frostbite3/Battlefield4`:

```text
go run .
```

This starts the redirector, Blaze server, QoS listener, Nucleus, Battlelog API and (when `DedicatedServer = true`) a built-in dedicated server.

### Standalone dedicated server

The dedicated server can also run as its own process and log in to a remote emulator:

```text
go run ./cmd/bf4host -blaze 151.xxx.xxx.xx:33152 -ip 151.xxx.xxx.xx -port 25210 ^
    -name "[DS] Siege of Shanghai - TDM" ^
    -level Levels/MP/MP_Siege/MP_Siege -mode TeamDeathMatch0 -players 64
```

| Flag             | Default                         | Meaning                                         |
| ---------------- | ------------------------------- | ----------------------------------------------- |
| `-blaze`         | `0.0.0.0:33152`                 | Blaze address of the emulator                   |
| `-ip` / `-port`  | `0.0.0.0` / `25210`             | Address players connect to                      |
| `-name`          | `[DS] Siege of Shanghai - ...`  | Server name in the browser                      |
| `-level`         | `Levels/MP/MP_Siege/MP_Siege`   | Level                                           |
| `-mode`          | `TeamDeathMatch0`               | ConquestLarge0, Domination0, RushLarge0, ...    |
| `-mod`           | `DEFAULT`                       | `DEFAULT`, `XPACK0`..`XPACK7`                   |
| `-players`       | `64`                            | Player slots                                    |
| `-version`       | `5900`                          | Game protocol version (the PS3's `GVER`)        |
| `-join-timeout`  | `20s`                           | Time a joining player has to connect            |
| `-captures`      | `data/captures`                 | Folder for game-port captures                   |
| `-fake-connect`  | `false`                         | **Testing only** — mark players connected early |

---

# Client Analysis

Development relies on information from the original PS3 client:

* `strings.txt` extracted from `EBOOT.ELF` (domains, URLs, component names, TDF tags, config keys)
* Packet captures and game-port captures (`data/captures/`)
* Server logs with raw hex of unknown commands
* PS3 client behaviour (what it retries, where it times out)

For the game protocol, the next step is static analysis of the EBOOT's network/crypto code.

---

# Project Status

| Area                                          | Status             |
| --------------------------------------------- | ------------------ |
| DNS redirection                               | ✅ Working          |
| TLS / ProtoSSL handshake                      | ✅ Working          |
| GOS Redirector / GetServerInstance            | ✅ Working          |
| Blaze packet + TDF encoder/decoder            | ✅ Working          |
| Util (PreAuth, Ping, FetchClientConfig, ...)  | ✅ Working          |
| PS3 login (ConsoleAuthentication + XI5)       | ✅ Working          |
| UserSessions / AssociationLists               | ✅ Working          |
| Entitlements / Auth token                     | ✅ Working          |
| Stats / Inventory / Packs / User settings     | ✅ Working          |
| **Main menu**                                 | ✅ Working          |
| **Test Range**                                | ✅ Playable         |
| Server browser / game list                    | ✅ Working          |
| Matchmaking / JoinGame (Blaze side)           | ✅ Working          |
| Dedicated server (Blaze side)                 | ✅ Working          |
| Logging                                       | ✅ Working          |
| Game-port traffic capture                     | ✅ Working          |
| **Frostbite game protocol (24-byte handshake)** | 🔬 Reverse engineering |
| Multiplayer matches                           | ⏳ Blocked on game protocol |
| Battlelog API                                 | ⏳ Future          |
| Game reporting / end-of-round stats           | ⏳ Future          |
| Leaderboards                                  | ⏳ Future          |
| Friends / Presence / Platoons                 | ⏳ Future          |
| RPCN integration                              | ⏳ Future           |

---

# Roadmap

1. ~~TLS handshake~~
2. ~~GOS Redirector / GetServerInstance~~
3. ~~Blaze TCP server, TDF~~
4. ~~PreAuth, Ping, FetchClientConfig~~
5. ~~PS3 login / XI5 ticket / UserSessions~~
6. ~~Entitlements, stats, inventory, user settings~~
7. ~~Reach the main menu~~
8. ~~Test Range~~
9. ~~Server browser, matchmaking and join flow~~
10. ~~Dedicated server Blaze integration~~
11. **Reverse engineer the 24-byte connection request and its encryption**
12. **Implement the handshake reply so the PS3 connects to the dedicated server**
13. Reverse engineer and write the in-game Frostbite protocol (from scratch)
14. Playable multiplayer match
15. Game reporting, stats updates and leaderboards
16. Friends, presence, platoons
17. RPCN integration

---

# Purpose

This project exists for:

* Game preservation
* Reverse engineering research
* Network protocol research
* Learning about legacy online architectures
* Educational purposes

---

# Disclaimer

This project is an independent implementation created for research and preservation purposes.

It is **not affiliated with, endorsed by, or associated with Electronic Arts or DICE**.

Battlefield and all related trademarks are the property of their respective owners.

---

# For Electronic Arts / DICE

If you are a representative of **Electronic Arts** or **DICE** and have any concerns about this project, please contact me directly before taking any other action. I am happy to talk and will respond quickly.

* **Email:** dqsbro@gmail.com
* **Discord:** @Byxk

This project:

* Contains **no EA or DICE code, assets or server software**
* Does **not** modify or redistribute the game executable
* Does **not** bypass any purchase or ownership check, so players must own an original copy of Battlefield 4
* Is **non-commercial** and will never be monetized
* Exists only to **preserve** the online experience of Battlefield 4 on PlayStation 3 now that the official servers are no longer available on that platform

If you request it, I will cooperate fully, including making changes to the project or taking it down.

---

# Contributing

Contributions are welcome — especially for the **Frostbite game protocol**:

* PS3 EBOOT reverse engineering (network / crypto code)
* Captures of the 24-byte connection request
* Blaze protocol research
* Game reporting / stats
* RPCN research
* Development, documentation and testing

Reach out on Discord: **@Byxk**

**Syntax BF4 Discord Server:** https://discord.gg/pFaTAHA7dg

---

# Credits

* https://www.psdevwiki.com/ps3/X-I-5-Ticket
* https://github.com/RipleyTom/rpcn/blob/master/src/server/client/ticket.rs
* https://github.com/RipleyTom/rpcn
* https://github.com/Aim4kill/BlazeSDK
* https://github.com/Aim4kill/Bug_OldProtoSSL
* https://github.com/buchacho/BF4BlazeEmulator
* https://github.com/PocketRelay/Server
* https://github.com/RPCS3/rpcs3

thank you for your work and help!

---

# License

This project is intended solely for research, education and preservation.

No original EA server software is included. Only original source code written for this emulator is distributed.

---

# Current Development Focus

**The Blaze backend is largely done: the unmodified PS3 client logs in, reaches the menus, loads stats and unlocks, and can play the Test Range. Server browsing, matchmaking and joining work, and a headless dedicated server registers games on Blaze. The current focus is reverse engineering Frostbite's in-game UDP protocol, starting with the encrypted 24-byte connection request (counter +3 per retry, flag `0x80`, 22 encrypted bytes) so that real multiplayer matches can be played on a dedicated server written from scratch.**


https://github.com/user-attachments/assets/02e658f0-022d-40aa-a046-c770474cb216

