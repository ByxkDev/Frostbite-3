# BF4 Blaze Emulator

[![Status](https://img.shields.io/badge/status-in%20game-brightgreen)](https://github.com/ByxkDev/Frostbite-3)
[![Language](https://img.shields.io/badge/language-Go-blue)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-PS3-lightgrey)](https://www.playstation.com/)

**Syntax BF4 Discord Server:** https://discord.gg/pFaTAHA7dg

---

# Overview

This project is a replacement backend for **Battlefield 4 (PlayStation 3)**.

It recreates EA's online infrastructure (Blaze) **and** a Frostbite dedicated game server, so the original, unmodified Battlefield 4 PS3 client can log in, find a server, join it and spawn into a multiplayer map, **without modifying the game executable**.

## Where we are now

Most of the game is working. The original PS3 client goes from boot all the way into a running multiplayer map:

* ✅ Boots, connects and logs in to our Blaze backend
* ✅ Reaches the **main menu** and the multiplayer menus
* ✅ Loads soldier, stats, entitlements, inventory, packs and user settings
* ✅ **Test Range is playable**
* ✅ Server browser (server info screen, Ranked, settings) and the join flow
* ✅ **Frostbite game connection works**: the encrypted connection request is solved (ProtoTunnel + CommUDP), the PS3 connects to our game server
* ✅ Time sync, level load and bundle streaming: **Siege of Shanghai (TDM) loads**, including buildings, the skyscraper and ~965 map objects
* ✅ Deploy screen, team, squad, spawn points and **spawning into the map** with the HUD and minimap
* ✅ Soldier entity is created on the PS3, controlled by the player and placed at the spawn point; the PS3 streams its movement input to the server
* ✅ Kit weapons are sent and their weapon bundles load (the D-pad shows the kit)
* 🔧 **Still in progress:** first-person arms / weapon in hand and ammo, HUD health, sounds, redeploy, other maps and game modes

Everything up to "spawn in the map" works. The remaining work is the last parts of the in-game Frostbite protocol: the soldier's visuals, weapons and gameplay state.

---

# ⚠️ Not included in this repository

Some parts of the project are **not published** in this repo:

| Not included | What it is |
| --- | --- |
| `Battlefield4/server/dedicated/` | The Frostbite **dedicated game server** (the in-game UDP protocol: connection, level loading, ghosts, spawning, soldier, weapons) |
| `Battlefield4/data/` | Runtime data: game list, stats, items, packs, CDN files, ProtoTunnel settings and captures |
| `Battlefield4/data/ebx/` (EBX data) | Game data extracted from the user's own copy of the game (network registry, spawn points, kits, weapon unlock tables) |

Because of this, the repository on its own **does not build a full working server**. It shows the Blaze backend, the networking and the overall structure. No game files, EA code or EA assets are distributed.

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
 GOS Redirector :42127          Nucleus / QoS (HTTP)              Dedicated server :25210
        |                       (PS3 login, ping sites)          (Frostbite game protocol)
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
| QoS / ping sites     | `17502`         | UDP / HTTP | `server/host.go`                         |
| Nucleus (PS3 login)  | `80`            | HTTP       | `network/nucleus/`                       |
| Dedicated server     | `25210`         | UDP        | `server/dedicated/` (not included)       |

---

# Login and Join Flow (working)

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
      +-- ConsoleAuthentication (PSN XI5 ticket)
      +-- UserSessions, PostAuth, telemetry, entitlements
      +-- User settings, Stats, Inventory, Packs, AssociationLists
      |
      v
Main menu  ->  Test Range  |  Server browser  ->  JoinGame
                                                    |
                                                    v
                                     Dedicated server (Frostbite, UDP)
                                                    |
      ProtoTunnel / CommUDP handshake  ->  time sync  ->  LoadLevel
      ->  bundle / sub-level streaming  ->  level loaded  ->  ghosts
      ->  deploy screen  ->  spawn  ->  soldier, HUD, movement input
```

---

# The Frostbite Game Protocol

On EA's infrastructure, Blaze only handles the lobby side (login, server browser, putting a player into a game). The match itself runs on a **Frostbite dedicated server** that talks to the PS3 over UDP with Frostbite's own protocol. No public documentation of it exists, so it was reverse engineered from the PS3 client and written from scratch.

What is implemented (in the dedicated server, not included in this repo):

* **Connection:** the "24-byte connection request" is a ProtoTunnel packet (RC4 keyed from the game's join data) carrying a CommUDP handshake. The server answers and the connection opens.
* **Game frames:** the channel layout of a game frame (messages, ghosts, state sync, movement).
* **Messages:** time sync, LoadLevel, bundle baselines, level-loaded ack, tiny events, server info, squads, difficulty, deploy / player events, OnPlayerSpawned.
* **Sub-levels and bundles:** the map's sub-levels and the kit's weapon bundles are streamed in the order the client expects.
* **Ghosts (replicated objects):** players, player manager, game-mode objects, sub-levels, spawn points, ~965 map objects and the soldier entity with its creation data and components (health, customization, weapons).
* **Movement:** the move channel is answered, so the PS3 sends its player input.

Still open: first-person arms and weapon visuals, ammo, HUD health, sound, redeploy, and gameplay (shooting, damage, scoring) between players.

---

# TLS / ProtoSSL

The client:

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

* `components/authentication.go`: ConsoleAuthentication / login handling
* `psn/xi5ticket.go`: parses the PSN **XI5 ticket** the PS3 sends (online ID, account data, signature)
* `network/nucleus/`: HTTP endpoints for the PS3 Nucleus login (`/ps3.php`, `/success`)
* Persona IDs are derived from the PSN online ID, so every player keeps the same persona between sessions

RPCN/RPCS3 is **Supported**

---

# Logging

A full logging system is used throughout the server (`logger/logger.go`) with `DEBUG`, `INFO`, `TRACE`, `WARN` and `ERROR` levels, hex dumps of unknown packets and TDF field tracing. Unknown Blaze commands are always logged with their raw payload, which is how most new commands are discovered.

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
        ├── main.go                 # entry point: redirector, Blaze, QoS, Nucleus, DNS, dedicated server
        │
        ├── blaze/                  # Blaze packet header + TDF encoder / decoder
        ├── components/             # Blaze components: Util, Auth, UserSessions, Stats,
        │                           # Inventory, Packs, GameManager, host sessions, ...
        ├── server/
        │   ├── game.go             # game registry / state
        │   ├── types.go            # shared GameManager TDF types
        │   ├── config.go           # game config
        │   ├── host.go             # QoS / ping site listeners
        │   └── dedicated/          # Frostbite dedicated server  (NOT INCLUDED)
        │
        ├── handshake/              # ProtoTunnel / CommUDP handshake helpers
        ├── ebx/                    # EBX parsing (the EBX data itself is NOT INCLUDED)
        ├── network/
        │   ├── certificates/       # generate.bat (certificate not included)
        │   ├── redirector/         # GetServerInstance
        │   ├── nucleus/            # PS3 Nucleus HTTP login
        │   └── dns/                # optional built-in DNS
        ├── psn/                    # PSN XI5 ticket parser
        ├── logger/
        ├── utilities/              # PreAuth / Ping / FetchClientConfig builders
        ├── cmd/                    # side tools (bf4host, fetchcdn, ps3read, ...)
        ├── tools/re/               # reverse-engineering helper scripts
        ├── docs/                   # protocol notes
        │
        └── data/                   # runtime data + EBX data  (NOT INCLUDED)
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

This starts the redirector, Blaze server, QoS listener and Nucleus. Remember that the dedicated game server and the `data/` folder (including the EBX data) are **not included** in this repo, so joining a multiplayer game will not work from this repository alone.

---

# Client Analysis

Development relies on information from the original PS3 client:

* `strings.txt` extracted from `EBOOT.ELF` (domains, URLs, component names, TDF tags, config keys)
* Static analysis of the game's own client code (network, ghost and soldier readers)
* RAM snapshots of the running PS3 (webMAN) while connected to our server
* Server logs with raw hex of unknown commands and packets
* Public EBX dumps of the map data

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
| Server browser / server info                  | ✅ Working          |
| Matchmaking / JoinGame (Blaze side)           | ✅ Working          |
| Dedicated server (Blaze side)                 | ✅ Working          |
| **Frostbite connection (ProtoTunnel / CommUDP)** | ✅ Working       |
| Level loading (Siege of Shanghai, TDM)        | ✅ Working          |
| Buildings and map objects                     | ✅ Working          |
| Deploy screen and spawning                    | ✅ Working          |
| Soldier entity and movement input             | ✅ Working          |
| Weapon bundles / kit on the D-pad             | ✅ Working          |
| First-person arms, weapon in hand, ammo       | 🔧 In progress      |
| HUD health                                    | 🔧 In progress      |
| Sound, redeploy                               | 🔧 In progress      |
| Gameplay between players (shooting, damage, score) | ⏳ Future      |
| Other maps and game modes                     | ⏳ Future           |
| Game reporting / end-of-round stats           | ⏳ Future           |
| Leaderboards                                  | ⏳ Future           |
| Friends / Presence / Platoons                 | ⏳ Future           |
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
11. ~~Reverse engineer the connection request and its encryption~~
12. ~~Handshake reply: the PS3 connects to the dedicated server~~
13. ~~Level loading, sub-level / bundle streaming, map objects~~
14. ~~Deploy screen and spawning~~
15. **First-person arms, weapons, ammo and HUD health**
16. Gameplay between players: playable multiplayer match
17. Game reporting, stats updates and leaderboards
18. Friends, presence, platoons
19. RPCN integration

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
* Does **not** include any game data (the EBX data and the `data/` folder are not in this repo)
* Does **not** modify or redistribute the game executable
* Does **not** bypass any purchase or ownership check, so players must own an original copy of Battlefield 4
* Is **non-commercial** and will never be monetized
* Exists only to **preserve** the online experience of Battlefield 4 on PlayStation 3 now that the official servers are no longer available on that platform

If you request it, I will cooperate fully, including making changes to the project or taking it down.

---

# Contributing

Contributions are welcome, especially for:

* PS3 EBOOT reverse engineering (soldier, weapons, HUD)
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

**The unmodified PS3 client logs in, reaches the menus, plays the Test Range, joins our Frostbite dedicated server, loads Siege of Shanghai with all buildings and spawns into the map with the HUD. The current focus is the soldier in first person: arms, weapon in hand, ammo and HUD health, followed by gameplay between players.**


https://github.com/user-attachments/assets/02e658f0-022d-40aa-a046-c770474cb216
