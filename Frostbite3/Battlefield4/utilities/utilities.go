package utilities

import (
	"bytes"
	"time"

	"bf4/blaze"
	"bf4/logger"
)

const (
	UtilComponent uint16 = 9
	PreAuth       uint16 = 7
)

const (
	cmdFetchClientConfig uint16 = 1
	cmdPing              uint16 = 2
)

func BuildPreAuthResponse(messageID uint32) []byte {
	logger.Info("UTIL: building PreAuth response (MessageId=%d)",messageID)

	payload:=bytes.NewBuffer(nil)

	blaze.WriteBool(payload,"ANON",false)
	blaze.WriteTDF(payload,"ASRC","300294")

	logger.Trace("UTIL: PreAuth writing CIDS")
	componentIDs:=[]int64{
		30728,
		1,
		30729,
		25,
		30730,
		27,
		4,
		28,
		6,
		7,
		9,
		10,
		63490,
		35,
		15,
		30720,
		30722,
		30723,
		30724,
		30726,
		2000,
		30727,
	}

	componentData:=make([][]byte,len(componentIDs))
	for i,id:=range componentIDs {
		var element bytes.Buffer
		blaze.WriteTDFInteger(&element,id)
		componentData[i]=element.Bytes()
	}

	blaze.WriteList(payload,"CIDS",blaze.TDF_INT32,componentData)

	logger.Trace("UTIL: PreAuth writing CONF")
	config:=buildPreAuthConfig()
	logConfig("PreAuth CONF",config)
	blaze.WriteMap(payload,"CONF",blaze.TDF_STRING,blaze.TDF_STRING,config)

	blaze.WriteTDF(payload,"INST","battlefield-4-ps3")
	blaze.WriteBool(payload,"MINR",false)
	blaze.WriteTDF(payload,"NASP","cem_ea_id")
	blaze.WriteTDF(payload,"PLAT","ps3")

	logger.Trace("UTIL: PreAuth writing QOSS/BWPS")

	blaze.WriteTag(payload,"QOSS")
	payload.WriteByte(0x03)

	blaze.WriteTag(payload,"BWPS")
	payload.WriteByte(0x03)

	blaze.WriteTDF(payload,"PSA ","192.168.178.91")

	blaze.WriteTag(payload,"PSP ")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload,17502)

	blaze.WriteTDF(payload,"SNA ","rs-prod-ps3")

	blaze.WriteTag(payload,"LNP ")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload,10)

	blaze.WriteTag(payload,"SVID")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload,1337)

	payload.WriteByte(0x00)
	payload.WriteByte(0x00)

	blaze.WriteTDF(payload,"RSRC","302123")
	blaze.WriteTDF(payload,"SVER","Blaze 13.3.1.8.0 (CL# 1148269)")

	logger.Debug("UTIL: PreAuth payload size: %d bytes",payload.Len())
	blaze.DebugTDF("PreAuth response",payload.Bytes())

	packet:=blaze.EncodePacket(UtilComponent, PreAuth, blaze.PacketTypeResponse, messageID,  payload.Bytes(),)

	logger.Response(packet)
	logger.Info("UTIL: PreAuth response ready (%d bytes)",len(packet))

	return packet
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
		[]byte("http://clientconfig.ea.com:80/success"),
		[]byte("nucleusConnect"),
		[]byte("http://clientconfig.ea.com:80/ps3.php?connect=1&z="),
		[]byte("nucleusProxy"),
		[]byte("http://clientconfig.ea.com:80/ps3.php?proxy=1&z="),
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

func logConfig(name string,config [][]byte) {
	if len(config)%2!=0 {
		logger.Warn("UTIL: %s has an odd number of entries (%d)",name,len(config))
	}

	logger.Debug("UTIL: %s (%d entries)",name,len(config)/2)

	for i:=0;i+1<len(config);i+=2 {
		logger.Trace("UTIL:   %s = %s",config[i],config[i+1])
	}
}

func BuildPingResponse(messageID uint32) []byte {
	stim:=time.Now().Unix()

	logger.Debug("UTIL: building Ping response (MessageId=%d STIM=%d)",messageID,stim)

	payload:=bytes.NewBuffer(nil)
	blaze.WriteTag(payload,"STIM")
	payload.WriteByte(0x00)
	blaze.WriteTDFInteger(payload,stim)

	response:=blaze.EncodePacket(UtilComponent, cmdPing, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

	logger.Response(response)
	logger.Debug("UTIL: Ping response: MessageId=%d STIM=%d Size=%d bytes",messageID,stim,len(response))

	return response
}

func BuildFetchClientConfigResponse(messageID uint32,requestPayload []byte) []byte {
	logger.Info("UTIL: building FetchClientConfig response (MessageId=%d)",messageID)
	logger.Hex(logger.LevelTrace,"UTIL FetchClientConfig request payload",requestPayload)

	var payload bytes.Buffer

	fields:=blaze.ReadTDF(requestPayload)
	logger.Debug("UTIL: FetchClientConfig request contains %d TDF fields",len(fields))

	foundCFID:=false

	for _,field:=range fields {
		if field.Tag!="CFID" {
			continue
		}

		foundCFID=true

		cfid,ok:=field.Value.(string)
		if !ok {
			logger.Warn("UTIL: FetchClientConfig CFID has unexpected type %T",field.Value)
			break
		}

		logger.Debug("UTIL: FetchClientConfig CFID=%q",cfid)

		switch cfid {
		case "IdentityParams":
			config:=[][]byte{
				[]byte("identityDisplayUri"),
				[]byte("console2/welcome"),
				[]byte("identityRedirectUri"),
				[]byte("http://clientconfig.ea.com:80/success"),
				[]byte("nucleusConnect"),
				[]byte("http://clientconfig.ea.com:80/ps3.php?connect=1&z="),
				[]byte("nucleusProxy"),
				[]byte("http://clientconfig.ea.com:80/ps3.php?proxy=1&z="),
				[]byte("xblTokenUrn"),
				[]byte("accounts.ea.com"),
				[]byte("capsStringValidationUri"),
				[]byte("client-strings.xboxlive.com"),
			}

			logConfig("IdentityParams",config)
			blaze.WriteMap(&payload,"CONF",blaze.TDF_STRING,blaze.TDF_STRING,config)

		case "GOSAchievements":
			config:=[][]byte{
				[]byte("Achievements"),
				[]byte("ACH32_00,ACH33_00,ACH34_00,ACH35_00,ACH36_00,ACH37_00,ACH38_00,ACH39_00,ACH40_00,XPACH01_00,XPACH02_00,XPACH03_00,XPACH04_00,XPACH05_00,XP2ACH01_00,XP2ACH04_00,XP2ACH03_00,XP2ACH05_00,XP3ACH01_00,XP3ACH05_00,XP3ACH03_00,XP3ACH04_00,XP3ACH02_00,XP4ACH01_00,XP4ACH02_00,XP4ACH03_00,XP4ACH04_00,XP4ACH05_00,XP5ACH01_00,XP5ACH02_00,XP5ACH03_00,XP5ACH04_00,XP5ACH05_00"),
				[]byte("WinCodes"),
				[]byte("r01_00,r05_00,r04_00,r03_00,r02_00,r10_00,r08_00,r07_00,r06_00,r09_00,r11_00,r12_00,r13_00,r14_00,r15_00,r16_00,r17_00,r18_00,r19_00,r20_00,r21_00,r22_00,r23_00,r24_00,r25_00,r26_00,r27_00,r28_00,r29_00,r30_00,r31_00,r32_00,r33_00,r35_00,r36_00,r34_00,r38_00,r39_00,r40_00,r41_00,r42_00,r43_00,r44_00,r45_00,xp2rgm_00,xp2rntdmcq_00,xp2rtdmc_00,xp3rts_00,xp3rdom_00,xp3rnts_00,xp3rngm_00,xp4rndom_00,xp4rscav_00,xp4rnscv_00,xp4ramb1_00,xp4ramb2_00,xp5r502_00,xp5r501_00,xp5ras_00,xp5asw_00"),
			}

			logConfig("GOSAchievements",config)
			blaze.WriteMap(&payload,"CONF",blaze.TDF_STRING,blaze.TDF_STRING,config)

		default:
			logger.Debug("UTIL: FetchClientConfig unknown CFID %q, sending default config",cfid)

			config:=[][]byte{
				[]byte("client_id"),
				[]byte("GOS-BlazeServer-BF4-PS3"),
				[]byte("display"),
				[]byte("console2/welcome"),
				[]byte("redirect_uri"),
				[]byte("http://clientconfig.ea.com:80/success"),
			}

			logConfig("FetchClientConfig default CONF",config)
			blaze.WriteMap(&payload,"CONF",blaze.TDF_STRING,blaze.TDF_STRING,config)
		}

		break
	}

	if !foundCFID {
		logger.Warn("UTIL: FetchClientConfig request had no CFID field, sending empty payload")
	}

	response:=blaze.EncodePacket(UtilComponent, cmdFetchClientConfig, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

	logger.Response(response)
	logger.Debug("UTIL: FetchClientConfig response: MessageId=%d Size=%d bytes",messageID,len(response))
	logger.Hex(logger.LevelDebug,"UTIL FetchClientConfig payload",payload.Bytes())

	return response
}
