package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"bf4/blaze"
	"bf4/components"
	"bf4/logger"
	"bf4/network/battlelog"
	"bf4/network/nucleus"
	"bf4/server"
	"bf4/server/dedicated"
)

const (
	RedirectorHostname  = "gosredirector.ea.com"
	RedirectorPort      = 42127
	GameServerHostname  = "0.0.0.0"
	GameServerPort      = 33152
	CertificatePath     = "network/certificates/gosredirector_mod.pfx"
	CertificatePassword = "123456"

	QoSPort = 17502

	DedicatedServer     = true
	DedicatedServerPort = 25210

	BattlelogAPI  = true
	BattlelogPort = 80
)

func loadPFX(path, password string) (tls.Certificate, error) {
	if _, err := os.Stat(path); err != nil {
		return tls.Certificate{}, fmt.Errorf("certificate not found: %w", err)
	}

	certPEM, err := extractCertificatePowerShell(path, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to extract certificate: %w", err)
	}

	keyPEM, err := extractPrivateKeyPowerShell(path, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to extract private key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return tls.Certificate{}, fmt.Errorf("invalid certificate PEM")
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
		return tls.Certificate{}, fmt.Errorf("invalid PKCS#8 private key PEM")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certBlock.Bytes},
		PrivateKey:  privateKey,
	}

	x509Cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err == nil {
		logger.Info("[TLS] Loaded patched PFX certificate")
		logger.Info("[TLS] Subject: %s", x509Cert.Subject.String())
		logger.Info("[TLS] Issuer: %s", x509Cert.Issuer.String())
		logger.Info("[TLS] Serial: %s", x509Cert.SerialNumber.String())
		logger.Info("[TLS] Not Before: %s", x509Cert.NotBefore.Format(time.RFC3339))
		logger.Info("[TLS] Not After: %s", x509Cert.NotAfter.Format(time.RFC3339))
	}

	return cert, nil
}

func extractCertificatePowerShell(path, password string) ([]byte, error) {
	script :=
		`
	$ErrorActionPreference = "Stop"

	$pfx = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2

	$pfx.Import(
		[IO.Path]::GetFullPath($env:BF4_PFX),
		$env:BF4_PFX_PASSWORD,
		[System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::Exportable
	)

	$bytes = $pfx.Export(
		[System.Security.Cryptography.X509Certificates.X509ContentType]::Cert
	)

	[Convert]::ToBase64String($bytes)
	`

	return runPowerShell(script, path, password, false)
}

func extractPrivateKeyPowerShell(path, password string) ([]byte, error) {
	script :=
		`
	$ErrorActionPreference = "Stop"

	$pfx = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2

	$pfx.Import(
		[IO.Path]::GetFullPath($env:BF4_PFX),
		$env:BF4_PFX_PASSWORD,
		[System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]::Exportable
	)

	if (-not $pfx.HasPrivateKey) {
		throw "PFX does not contain a private key"
	}

	$rsa = [System.Security.Cryptography.X509Certificates.RSACertificateExtensions]::GetRSAPrivateKey($pfx)

	if ($null -eq $rsa) {
		throw "Private key is not RSA"
	}

	if ($rsa -is [System.Security.Cryptography.RSACng]) {
		$bytes = $rsa.Key.Export(
			[System.Security.Cryptography.CngKeyBlobFormat]::Pkcs8PrivateBlob
		)
	}
	else {
		throw "Expected RSACng but received: " + $rsa.GetType().FullName
	}

	[Convert]::ToBase64String($bytes)
    `

	return runPowerShell(script, path, password, true)
}

func runPowerShell(script, path, password string, privateKey bool) ([]byte, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)

	cmd.Env = append(os.Environ(), "BF4_PFX="+path, "BF4_PFX_PASSWORD="+password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	encoded := strings.TrimSpace(string(output))
	encoded = strings.ReplaceAll(encoded, "\r", "")
	encoded = strings.ReplaceAll(encoded, "\n", "")

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid PowerShell base64 output: %w", err)
	}

	if privateKey {
		return pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: data,
		}), nil
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	}), nil
}

func redirectorTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS10,
		MaxVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{
			cert,
		},
		CipherSuites: []uint16{
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		NextProtos: []string{"http/1.1"},
	}
}

func dumpPacket(direction string, data []byte) {
	if len(data) < 12 {
		logger.Warn("[%s] Packet too small for Blaze header: %d bytes", direction, len(data))
		logger.Hex(logger.LevelTrace, direction+" RAW", data)
		return
	}

	packet := blaze.Parse(data)

	logger.Trace("%s HEADER: Size=%d Component=%d Command=%d Type=0x%04X MessageId=%d", direction, packet.Size, packet.Component, packet.Command, packet.Type, packet.MessageId)
}

func readBlazePacket(conn net.Conn) ([]byte, error) {
	const headerSize = 12

	header := make([]byte, headerSize, headerSize+2)

	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	payloadSize := int(binary.BigEndian.Uint16(header[0:2]))

	if binary.BigEndian.Uint16(header[8:10])&0x0010 != 0 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(conn, ext); err != nil {
			return nil, err
		}
		payloadSize |= int(binary.BigEndian.Uint16(ext)) << 16
		header = append(header, ext...)
	}

	packet := make([]byte, len(header)+payloadSize)
	copy(packet, header)

	if payloadSize > 0 {
		if _, err := io.ReadFull(conn, packet[len(header):]); err != nil {
			return nil, err
		}
	}

	return packet, nil
}

func writeAll(conn net.Conn, mu *sync.Mutex, serverName string, data []byte) error {
	mu.Lock()
	defer mu.Unlock()

	written := 0
	for written < len(data) {
		count, err := conn.Write(data[written:])
		if err != nil {
			return fmt.Errorf("send error after %d/%d bytes: %w", written, len(data), err)
		}
		if count <= 0 {
			return fmt.Errorf("write returned %d bytes", count)
		}
		written += count
		logger.Debug("[%s] Write progress: %d/%d bytes", serverName, written, len(data))
	}
	return nil
}

func handleBlaze(conn net.Conn, serverName string) {
	defer conn.Close()

	remote := conn.RemoteAddr().String()
	local := conn.LocalAddr().String()

	logger.Info("[%s] CONNECTION", serverName)
	logger.Info("[%s] Remote: %s", serverName, remote)
	logger.Info("[%s] Local: %s", serverName, local)
	logger.Info("[%s] Type: %T", serverName, conn)

	if tlsConn, ok := conn.(*tls.Conn); ok {
		logger.Debug("[%s] TLS connection detected", serverName)

		if err := tlsConn.Handshake(); err != nil {
			logger.Error("[%s] TLS handshake failed: %v", serverName, err)
			return
		}

		state := tlsConn.ConnectionState()

		logger.Info("[%s] TLS version: 0x%04X", serverName, state.Version)
		logger.Info("[%s] TLS cipher: 0x%04X", serverName, state.CipherSuite)
		logger.Info("[%s] TLS negotiated protocol: %s", serverName, state.NegotiatedProtocol)
		logger.Info("[%s] TLS server name: %s", serverName, state.ServerName)
	}

	var writeMu sync.Mutex

	var bc *components.Conn
	if serverName == "GAME" {
		bc = components.NewConn(remote, func(b []byte) error {
			dumpPacket("PUSH", b)
			return writeAll(conn, &writeMu, serverName, b)
		})
		defer components.ConnClosed(bc)
	}

	for {
		data, err := readBlazePacket(conn)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logger.Info("[%s] Connection closed", serverName)
			} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				logger.Info("[%s] Client disconnected: EOF", serverName)
			} else {
				logger.Error("[%s] Read error: %v", serverName, err)
			}

			return
		}

		logger.Info("[%s] RECEIVED %d BYTES", serverName, len(data))
		logger.Info("[%s] Remote: %s", serverName, remote)

		dumpPacket("IN", data)

		var reply []byte
		if bc != nil {
			reply = components.HandleConnPacket(bc, data)
		} else {
			reply = components.HandlePacket(data)
		}

		if reply == nil {
			logger.Debug("[%s] Handler returned no response", serverName)
			continue
		}

		logger.Info("[%s] RESPONSE GENERATED: %d bytes", serverName, len(reply))
		dumpPacket("OUT", reply)

		logger.Info("[%s] Sending response...", serverName)

		if err := writeAll(conn, &writeMu, serverName, reply); err != nil {
			logger.Error("[%s] %v", serverName, err)
			return
		}

		logger.Info("[%s] Successfully sent %d bytes", serverName, len(reply))
	}
}

func startRedirector() {
	logger.Info("Starting Redirector %s:%d", RedirectorHostname, RedirectorPort)

	cert, err := loadPFX(CertificatePath, CertificatePassword)
	if err != nil {
		logger.Error("Failed to load redirector certificate: %v", err)
		panic(err)
	}

	tlsConfig := redirectorTLSConfig(cert)

	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", RedirectorPort), tlsConfig)
	if err != nil {
		logger.Error("Failed to start redirector: %v", err)
		panic(err)
	}

	defer listener.Close()

	logger.Info("[REDIRECTOR] TLS listener active on %s:%d", RedirectorHostname, RedirectorPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("[REDIRECTOR] Accept error: %v", err)
			continue
		}

		logger.Info("[REDIRECTOR] TCP connection accepted: %s -> %s", conn.RemoteAddr(), conn.LocalAddr())

		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("[REDIRECTOR] Panic: %v", r)
				}
			}()

			handleBlaze(conn, "REDIRECTOR")
		}(conn)
	}
}

func startGameServer() {
	logger.Info("Starting Game Server %s:%d", GameServerHostname, GameServerPort)

	listener, err := net.Listen(
		"tcp",
		fmt.Sprintf(":%d", GameServerPort),
	)
	if err != nil {
		logger.Error("Failed to start game server: %v", err)
		panic(err)
	}

	defer listener.Close()

	logger.Info("[GAME] TCP listener active on %s:%d", GameServerHostname, GameServerPort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("[GAME] Accept error: %v", err)
			continue
		}

		logger.Info("[GAME] TCP connection accepted: %s -> %s", conn.RemoteAddr(), conn.LocalAddr())

		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("[GAME] Panic: %v", r)
				}
			}()

			handleBlaze(conn, "GAME")
		}(conn)
	}
}

func main() {
	logger.Init(true, true)
	defer logger.Close()

	logger.Info("Redirector : %s:%d", RedirectorHostname, RedirectorPort)
	logger.Info("Game       : %s:%d", GameServerHostname, GameServerPort)
	logger.Info("Certificate: %s", CertificatePath)

	components.ServerHost = GameServerHostname

	components.ListedGames = !DedicatedServer
	components.PeerHostedMatchmaking = false
	components.InitGames()

	server.StartHostListeners(server.Games, QoSPort)

	go startRedirector()
	go startGameServer()

	go func() {
		nucleusServer := nucleus.New()

		if err := nucleusServer.Start(); err != nil {
			logger.Error("[NUCLEUS] Server stopped: %v", err)
		}
	}()

	var ds *dedicated.Server
	if DedicatedServer {
		cfg := dedicated.DefaultConfig()
		cfg.Blaze = fmt.Sprintf("%s:%d", GameServerHostname, GameServerPort)
		cfg.IP = GameServerHostname
		cfg.Port = DedicatedServerPort

		time.Sleep(500 * time.Millisecond) 
		var err error
		if ds, err = dedicated.Start(cfg); err != nil {
			logger.Error("Dedicated server not started: %v", err)
		}
	}

	if BattlelogAPI {
		cfg := battlelog.DefaultConfig()
		cfg.Addr = fmt.Sprintf(":%d", BattlelogPort)
		cfg.Host = GameServerHostname
		if _, err := battlelog.Start(cfg); err != nil {
			logger.Error("Battlelog API not started: %v", err)
		}
	}

	logger.Info("Waiting for PS3 connections...")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	logger.Info("Shutting down...")
	if ds != nil {
		ds.Stop()
	}
}
