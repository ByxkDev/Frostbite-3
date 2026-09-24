package blaze

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	UtilComponent uint16 = 4
	PreAuth       uint16 = 1
)

func BuildPreAuthResponse(messageID uint32) []byte {
	payload := bytes.NewBuffer(nil)
	// ANON - mAnonymousChildAccountsEnabled
	WriteBool(payload, "ANON", false)
	// AUTH - mAuthenticationSource
	WriteTDF(payload, "AUTH", "")
	// COMP - mComponentIds
	WriteList(payload, "COMP", TDF_UINT16, nil)
	// CONF - mConfig
	config := buildPreAuthConfig()
	WriteMap(payload, "CONF", TDF_STRING, TDF_STRING, config)
	// INST - mInstanceName
	WriteTDF(payload, "INST", "battlefield-4-pc")
	// LDOC - mLegalDocGameIdentifier
	WriteTDF(payload, "LDOC", "")
	// PCON - mParentalConsentEntitlementGroupName
	WriteTDF(payload, "PCON", "")
	// PCTG - mParentalConsentEntitlementTag
	WriteTDF(payload, "PCTG", "")
	// PERS - mPersonaNamespace
	WriteTDF(payload, "PERS", "cem_ea_id")
	// PLAT - mPlatform
	WriteTDF(payload, "PLAT", "ps3")
	// QOSS - mQosSettings
	WriteStruct(payload, "QOSS", nil)
	// REGI - mRegistrationSource
	WriteTDF(payload, "REGI", "302123")
	// SVRN - mServerVersion
	WriteTDF(payload, "SVRN", "Blaze 13.3.1.8.0 (CL# 1148269)")
	// UAGE - mUnderageSupported
	WriteBool(payload, "UAGE", false)

	fmt.Printf("PreAuth response built: %d bytes\n", payload.Len())
	return EncodePacket(UtilComponent, PreAuth, 0, messageID, payload.Bytes(),)
}

func buildPreAuthConfig() [][]byte {
	config := []struct {
		Key   string
		Value string
	}{
		{"associationListSkipInitialSet", "1"},
		{"blazeServerClientId", "GOS-BlazeServer-BF4-PS3"},
		{"bytevaultHostname", "bytevault.gameservices.ea.com"},
		{"bytevaultPort", "42210"},
		{"bytevaultSecure", "true"},
		{"capsStringValidationUri", "client-strings.xboxlive.com"},
		{"connIdleTimeout", "90s"},
		{"defaultRequestTimeout", "60s"},
		{"identityDisplayUri", "console2/welcome"},
		{"identityRedirectUri", "http://127.0.0.1/success"},
		{"nucleusConnect", "https://accounts.ea.com"},
		{"nucleusProxy", "https://gateway.ea.com"},
		{"pingPeriod", "30s"},
		{"userManagerMaxCachedUsers", "0"},
		{"voipHeadsetUpdateRate", "1000"},
		{"xblTokenUrn", "accounts.ea.com"},
		{"xlspConnectionIdleTimeout", "300"},
	}

	entries := make([][]byte, 0, len(config))

	for _, item := range config {
		entry := bytes.NewBuffer(nil)
		writeRawString(entry, item.Key)
		writeRawString(entry, item.Value)
		entries = append(entries, entry.Bytes())
	}

	return entries
}

func writeRawString(buf *bytes.Buffer, value string) {
	binary.Write(buf, binary.BigEndian, uint32(len(value)+1))
	buf.WriteString(value)
	buf.WriteByte(0)
}