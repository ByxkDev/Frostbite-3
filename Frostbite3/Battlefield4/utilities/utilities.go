package utilities

import (
	"bytes"
	"fmt"
	"time"

	"bf4/blaze"
)

const (
	UtilComponent uint16 = 9
	PreAuth       uint16 = 7
)

func BuildPreAuthResponse(messageID uint32) []byte {
	payload := bytes.NewBuffer(nil)

	blaze.WriteBool(payload, "ANON", false)
	blaze.WriteTDF(payload, "ASRC", "300294")

	payload.Write([]byte{
		0x8E, 0x99, 0x33, 0x04, 0x00, 0x16, 0x88, 0xE0, 0x03, 0x01, 0x89, 0xE0, 0x03, 0x19, 0x8A, 0xE0, 0x03, 0x1B, 0x04, 0x1C, 0x06, 0x07, 0x09, 0x0A, 0x82, 0xE0, 0x07, 0x23, 0x0F, 0x80, 0xE0, 0x03, 0x82, 0xE0, 0x03, 0x83, 0xE0, 0x03, 0x84, 0xE0, 0x03, 0x86, 0xE0, 0x03, 0x39, 0x01, 0xF8, 0x7E, 0x00,
	})

	blaze.WriteTag(payload, "CONF")
	payload.WriteByte(blaze.TDF_STRUCT)

	blaze.WriteMap(payload, "CONF", blaze.TDF_STRING, blaze.TDF_STRING, buildPreAuthConfig())
	payload.WriteByte(0x00)
	payload.WriteByte(0x00)

	blaze.WriteTDF(payload, "INST", "battlefield-4-pc")
	blaze.WriteBool(payload, "MINR", false)
	blaze.WriteTDF(payload, "NASP", "cem_ea_id")
	blaze.WriteTDF(payload, "PLAT", "ps3")

	blaze.WriteTag(payload, "QOSS")
	payload.WriteByte(blaze.TDF_STRUCT)

	blaze.WriteTag(payload, "BWPS")
	payload.WriteByte(blaze.TDF_STRUCT)

	blaze.WriteTDF(payload, "PSA ", "151.244.72.66")

	blaze.WriteTag(payload, "PSP ")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload, 17502)

	blaze.WriteTDF(payload, "SNA ", "rs-prod-ps3")

	payload.WriteByte(0x00)

	blaze.WriteTag(payload, "LNP ")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload, 10)

	blaze.WriteTag(payload, "SVID")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload, 1337)

	payload.WriteByte(0x00)

	blaze.WriteTDF(payload, "RSRC", "302123")
	blaze.WriteTDF(payload, "SVER", "Blaze 13.3.1.8.0 (CL# 1148269)")

	fmt.Printf("[UTIL] PreAuth payload size: %d bytes\n", payload.Len())
	fmt.Printf("[UTIL] PreAuth payload: % X\n", payload.Bytes())

	return blaze.EncodePacket(UtilComponent, PreAuth, blaze.PacketTypeResponse, messageID, payload.Bytes(),)
}

func buildPreAuthConfig() [][]byte {
	return [][]byte{
		[]byte("associationListSkipInitialSet"),
		[]byte("1"),
		[]byte("blazeServerClientId"),
		[]byte("GOS-BlazeServer-BF4-PS3"),
		[]byte("bytevaultHostname"),
		[]byte("bytevault.gameservices.ea.com"),
		[]byte("bytevaultPort"),
		[]byte("42210"),
		[]byte("bytevaultSecure"),
		[]byte("true"),
		[]byte("capsStringValidationUri"),
		[]byte("client-strings.xboxlive.com"),
		[]byte("connIdleTimeout"),
		[]byte("90s"),
		[]byte("defaultRequestTimeout"),
		[]byte("60s"),
		[]byte("identityDisplayUri"),
		[]byte("console2/welcome"),
		[]byte("identityRedirectUri"),
		[]byte("http://127.0.0.1/success"),
		[]byte("nucleusConnect"),
		[]byte("https://accounts.ea.com"),
		[]byte("nucleusProxy"),
		[]byte("https://gateway.ea.com"),
		[]byte("pingPeriod"),
		[]byte("30s"),
		[]byte("userManagerMaxCachedUsers"),
		[]byte("0"),
		[]byte("voipHeadsetUpdateRate"),
		[]byte("1000"),
		[]byte("xblTokenUrn"),
		[]byte("accounts.ea.com"),
		[]byte("xlspConnectionIdleTimeout"),
		[]byte("300"),
	}
}

func writeRawString(buf *bytes.Buffer, value string) {
	n := uint32(len(value) + 1)
	buf.WriteByte(byte(n >> 24))
	buf.WriteByte(byte(n >> 16))
	buf.WriteByte(byte(n >> 8))
	buf.WriteByte(byte(n))
	buf.WriteString(value)
	buf.WriteByte(0)
}

func BuildPingResponse(messageID uint32) []byte {
	stim := time.Now().Unix()

	payload := bytes.NewBuffer(nil)
	blaze.WriteTag(payload, "STIM")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload, stim)

	response := blaze.EncodePacket(UtilComponent, 2, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

	fmt.Printf("[UTIL] Ping response: MessageId=%d STIM=%d Size=%d bytes\n", messageID, stim, len(response),)

	return response
}

func BuildFetchClientConfigResponse(messageID uint32, requestPayload []byte) []byte {
	var payload bytes.Buffer

	fields := blaze.ReadTDF(requestPayload)

	for _, field := range fields {
		if field.Tag != "CFID" {
			continue
		}

		cfid, ok := field.Value.(string)
		if !ok {
			break
		}

		fmt.Printf("[UTIL] FetchClientConfig CFID=%q\n", cfid)

		switch cfid {
		case "IdentityParams":
			config := [][]byte{
				[]byte("identityDisplayUri"),
				[]byte("console2/welcome"),
				[]byte("identityRedirectUri"),
				[]byte("http://127.0.0.1/success"),
				[]byte("nucleusConnect"),
				[]byte("https://accounts.ea.com"),
				[]byte("nucleusProxy"),
				[]byte("https://gateway.ea.com"),
				[]byte("xblTokenUrn"),
				[]byte("accounts.ea.com"),
				[]byte("capsStringValidationUri"),
				[]byte("client-strings.xboxlive.com"),
			}

			for i := 0; i+1 < len(config); i += 2 {
				fmt.Printf("[UTIL] IdentityParams %s = %s\n", config[i], config[i+1])
			}

			blaze.WriteMap(&payload, "CONF", blaze.TDF_STRING, blaze.TDF_STRING, config,)

		case "GOSAchievements":
			config := [][]byte{
				[]byte("Achievements"),
				[]byte("ACH32_00,ACH33_00,ACH34_00,ACH35_00,ACH36_00,ACH37_00,ACH38_00,ACH39_00,ACH40_00,XPACH01_00,XPACH02_00,XPACH03_00,XPACH04_00,XPACH05_00,XP2ACH01_00,XP2ACH04_00,XP2ACH03_00,XP2ACH05_00,XP3ACH01_00,XP3ACH05_00,XP3ACH03_00,XP3ACH04_00,XP3ACH02_00,XP4ACH01_00,XP4ACH02_00,XP4ACH03_00,XP4ACH04_00,XP4ACH05_00,XP5ACH01_00,XP5ACH02_00,XP5ACH03_00,XP5ACH04_00,XP5ACH05_00"),
				[]byte("WinCodes"),
				[]byte("r01_00,r05_00,r04_00,r03_00,r02_00,r10_00,r08_00,r07_00,r06_00,r09_00,r11_00,r12_00,r13_00,r14_00,r15_00,r16_00,r17_00,r18_00,r19_00,r20_00,r21_00,r22_00,r23_00,r24_00,r25_00,r26_00,r27_00,r28_00,r29_00,r30_00,r31_00,r32_00,r33_00,r35_00,r36_00,r34_00,r38_00,r39_00,r40_00,r41_00,r42_00,r43_00,r44_00,r45_00,xp2rgm_00,xp2rntdmcq_00,xp2rtdmc_00,xp3rts_00,xp3rdom_00,xp3rnts_00,xp3rngm_00,xp4rndom_00,xp4rscav_00,xp4rnscv_00,xp4ramb1_00,xp4ramb2_00,xp5r502_00,xp5r501_00,xp5ras_00,xp5asw_00"),
			}

			blaze.WriteMap(&payload, "CONF", blaze.TDF_STRING, blaze.TDF_STRING, config,)
        
        //default thanks to heavywguy
		default:
			config := [][]byte{
				[]byte("client_id"),
				[]byte("GOS-BlazeServer-BF4-PS3"),
				[]byte("display"),
				[]byte("console2/welcome"),
				[]byte("redirect_uri"),
				[]byte("http://127.0.0.1/success"),
			}

			fmt.Printf("[UTIL] FetchClientConfig CONF:\n")
			for i := 0; i+1 < len(config); i += 2 {
				fmt.Printf("[UTIL]   %s = %s\n", config[i], config[i+1])
			}

			blaze.WriteMap(&payload, "CONF", blaze.TDF_STRING, blaze.TDF_STRING, config,)
		}

		break
	}

	response := blaze.EncodePacket(UtilComponent, 1, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

	fmt.Printf("[UTIL] FetchClientConfig response: MessageId=%d Size=%d bytes\n", messageID, len(response),)
	fmt.Printf("[UTIL] FetchClientConfig payload: % X\n", payload.Bytes(),)

	return response
}
