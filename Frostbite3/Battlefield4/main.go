package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"bf4/components"

)

const (
	RedirectorHostname  = "gosredirector.ea.com"
	RedirectorPort      = 42127
	GameServerHostname  = "151.xxx.xxx.xx"
	GameServerPort      = 33152
	CertificatePath     = "network/certificates/gosredirector.pfx"
	CertificatePassword = "password"
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
		fmt.Printf("[+] Loaded patched PFX certificate\n")
		fmt.Printf("[+] Subject: %s\n", x509Cert.Subject.String())
		fmt.Printf("[+] Issuer : %s\n", x509Cert.Issuer.String())
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script,)

	cmd.Env = append(os.Environ(), "BF4_PFX="+path, "BF4_PFX_PASSWORD="+password,)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)),)
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

func handleBlaze(conn net.Conn, serverName string) {
	defer conn.Close()

	fmt.Printf("[%s] Client connected: %s\n", serverName, conn.RemoteAddr())

	buf := make([]byte, 65535)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("[%s] Client disconnected: %v\n", serverName, err)
			return
		}

		if n == 0 {
			continue
		}

		data := append([]byte(nil), buf[:n]...)

		fmt.Printf("[%s] Received %d bytes\n", serverName, len(data))

		reply := components.HandlePacket(data)
		if reply == nil {
			continue
		}

		fmt.Printf("[%s] Sending %d bytes\n", serverName, len(reply))

		if _, err := conn.Write(reply); err != nil {
			fmt.Printf("[%s] Send error: %v\n", serverName, err)
			return
		}
	}
}

func startRedirector() {
	fmt.Printf("Starting Redirector %s:%d\n", RedirectorHostname, RedirectorPort,)

	cert, err := loadPFX(CertificatePath,CertificatePassword,)
	if err != nil {
		panic(fmt.Errorf("failed to load redirector certificate: %w", err,))
	}

	tlsConfig := redirectorTLSConfig(cert)
	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", RedirectorPort), tlsConfig,)
	if err != nil {
		panic(fmt.Errorf("failed to start redirector: %w", err,))
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[REDIRECTOR] Accept error: %v\n", err,)
			continue
		}

		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[REDIRECTOR] Panic: %v\n", r,)
				}
			}()

			handleBlaze(conn, "REDIRECTOR")

		} (conn)
	}
}

func startGameServer() {
	fmt.Printf("Starting Game Server %s:%d\n", GameServerHostname, GameServerPort,)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", GameServerPort),)
	if err != nil {
		panic(fmt.Errorf("failed to start game server: %w", err,))
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[GAME] Accept error: %v\n", err,)
			continue
		}

		go func(conn net.Conn) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[GAME] Panic: %v\n", r,)
				}
			}()

			handleBlaze(conn, "GAME")

		} (conn)
	}
}

func main() {
	fmt.Printf("Redirector : %s:%d\n", RedirectorHostname, RedirectorPort,)
	fmt.Printf("Game: %s:%d\n", GameServerHostname, GameServerPort,)
	fmt.Printf("Certificate: %s\n", CertificatePath,)

	go startRedirector()
	go startGameServer()

	fmt.Println("Waiting for PS3 connections...")

	for {
		time.Sleep(time.Hour)
	}
}
