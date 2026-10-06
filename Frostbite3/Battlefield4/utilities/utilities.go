package utilities

import (
	"time"

	"bf4/blaze"
	"bf4/logger"
)

const (
	UtilComponent uint16=9
	cmdFetchClientConfig uint16=1
	cmdPing uint16=2
	PreAuth uint16=7
)

var (
	factory=blaze.NewTdfFactory()
	encoder=factory.CreateEncoder(false)
	decoder=factory.CreateDecoder(false)
)

type PreAuthResponse struct {
	AnonymousChildAccountsEnabled bool `tdf:"ANON"`
	AuthenticationSource string `tdf:"ASRC"`
	ComponentIDs []uint16 `tdf:"CIDS"`
	Config FetchConfigResponse `tdf:"CONF"`
	InstanceName string `tdf:"INST"`
	LegalDocGameIdentifier string `tdf:"LDOC"`
	ParentalConsentGroupName string `tdf:"PCON"`
	ParentalConsentTag string `tdf:"PCTG"`
	PersonaNamespace string `tdf:"PERS"`
	Platform string `tdf:"PLAT"`
	QosSettings QosConfigInfo `tdf:"QOSS"`
	RegistrationSource string `tdf:"REGI"`
	ServerVersion string `tdf:"SVRN"`
	UnderageSupported bool `tdf:"UAGE"`
}

type FetchConfigResponse struct {
	Config map[string]string `tdf:"CONF"`
}

type QosConfigInfo struct {
	BandwidthPingSiteInfo QosPingSiteInfo `tdf:"BWPS"`
	NumLatencyProbes uint32 `tdf:"LNP"`
	PingSiteInfoByAlias map[string]QosPingSiteInfo `tdf:"PSAM"`
	ServiceID uint32 `tdf:"SVID"`
}

type QosPingSiteInfo struct {
	Address string `tdf:"PSA"`
	Port uint16 `tdf:"PSP"`
	SiteName string `tdf:"SNA"`
}

type FetchClientConfigRequest struct {
	ConfigSection string `tdf:"CFID"`
}

type PingResponse struct {
	ServerTime uint32 `tdf:"STIM"`
}

func BuildPreAuthResponse(messageID uint32) []byte {
	logger.Info("UTIL: building PreAuth response (MessageId=%d)",messageID)

	componentIDs := []uint16{
		1,      //Authentication
		5,      //Redirector
		9,      //Util
		4,      //GameManager
		30722,  //UserSessions

	}

	config:=map[string]string{
		"associationListSkipInitialSet":"1",
		"blazeServerClientId":"GOS-BlazeServer-BF4-PS3",
		"bytevaultHostname":"bytevault.gameservices.ea.com",
		"bytevaultPort":"42210",
		"bytevaultSecure":"true",
		"capsStringValidationUri":"client-strings.xboxlive.com",
		"connIdleTimeout":"90s",
		"defaultRequestTimeout":"60s",
		"identityDisplayUri":"console2/welcome",
		"identityRedirectUri":"http://clientconfig.ea.com:80/success",
		"nucleusConnect":"http://clientconfig.ea.com:80/ps3.php?connect=1&z=",
		"nucleusProxy":"http://clientconfig.ea.com:80/ps3.php?proxy=1&z=",
		"pingPeriod":"30s",
		"userManagerMaxCachedUsers":"0",
		"voipHeadsetUpdateRate":"1000",
		"xblTokenUrn":"accounts.ea.com",
		"xlspConnectionIdleTimeout":"300",
	}

	response:=PreAuthResponse{
		AnonymousChildAccountsEnabled:false,
		AuthenticationSource:"300294",
		ComponentIDs:componentIDs,
		Config:FetchConfigResponse{
			Config:config,
		},
		InstanceName:"battlefield-4-ps3",
		LegalDocGameIdentifier:"",
		ParentalConsentGroupName:"",
		ParentalConsentTag:"",
		PersonaNamespace:"cem_ea_id",
		Platform:"ps3",
		QosSettings:QosConfigInfo{
			BandwidthPingSiteInfo:QosPingSiteInfo{
				Address:"0.0.0.0",
				Port:17502,
				SiteName:"rs-prod-ps3",
			},
			NumLatencyProbes:10,
			PingSiteInfoByAlias:map[string]QosPingSiteInfo{},
			ServiceID:1337,
		},
		RegistrationSource:"302123",
		ServerVersion:"Blaze 13.3.1.8.0 (CL# 1148269)",
		UnderageSupported:false,
	}

	logger.Info("UTIL: PreAuth ANON=%v",response.AnonymousChildAccountsEnabled)
	logger.Info("UTIL: PreAuth ASRC=%q",response.AuthenticationSource)
	logger.Info("UTIL: PreAuth CIDS=%d component IDs",len(response.ComponentIDs))
	logger.Info("UTIL: PreAuth INST=%q",response.InstanceName)
	logger.Info("UTIL: PreAuth PERS=%q",response.PersonaNamespace)
	logger.Info("UTIL: PreAuth PLAT=%q",response.Platform)
	logger.Info("UTIL: PreAuth REGI=%q",response.RegistrationSource)
	logger.Info("UTIL: PreAuth SVRN=%q",response.ServerVersion)
	logger.Info("UTIL: PreAuth QOS PSA=%s:%d",response.QosSettings.BandwidthPingSiteInfo.Address,response.QosSettings.BandwidthPingSiteInfo.Port)
	logger.Info("UTIL: PreAuth QOS SNA=%q",response.QosSettings.BandwidthPingSiteInfo.SiteName)
	logger.Info("UTIL: PreAuth QOS LNP=%d",response.QosSettings.NumLatencyProbes)
	logger.Info("UTIL: PreAuth QOS SVID=%d",response.QosSettings.ServiceID)

	for key,value:=range config {
		logger.Info("UTIL: PreAuth CONF %s=%q",key,value)
	}

	payload,err:=encoder.Encode(&response)
	if err!=nil {
		logger.Error("UTIL: failed to encode PreAuth response: %v",err)
		return nil
	}

	packet:=blaze.EncodePacket(UtilComponent, PreAuth, blaze.PacketTypeResponse, messageID, payload,)
	logger.Info("UTIL: PreAuth payload=%d bytes packet=%d bytes",len(payload),len(packet))
	return packet
}

func BuildPingResponse(messageID uint32) []byte {
	stim:=uint32(time.Now().Unix())

	response:=PingResponse{
		ServerTime:stim,
	}

	payload,err:=encoder.Encode(&response)
	if err!=nil {
		logger.Error("UTIL: failed to encode Ping response: %v",err)
		return nil
	}

	packet:=blaze.EncodePacket(UtilComponent, cmdPing, blaze.PacketTypeResponse, messageID, payload,)
	logger.Debug("UTIL: Ping STIM=%d payload=%d packet=%d",stim,len(payload),len(packet))
	return packet
}

func BuildFetchClientConfigResponse(messageID uint32,configSection string) []byte {
	logger.Info("UTIL: building FetchClientConfig response CFID=%q MessageId=%d",configSection,messageID)

	config:=map[string]string{}

	switch configSection {
	case "IdentityParams":
		config["identityDisplayUri"]="console2/welcome"
		config["identityRedirectUri"]="http://clientconfig.ea.com:80/success"
		config["nucleusConnect"]="http://clientconfig.ea.com:80/ps3.php?connect=1&z="
		config["nucleusProxy"]="http://clientconfig.ea.com:80/ps3.php?proxy=1&z="
		config["xblTokenUrn"]="accounts.ea.com"
		config["capsStringValidationUri"]="client-strings.xboxlive.com"

	default:
		config["client_id"]="GOS-BlazeServer-BF4-PS3"
		config["display"]="console2/welcome"
		config["redirect_uri"]="http://clientconfig.ea.com:80/success"
	}

	for key,value:=range config {
		logger.Info("UTIL: CONF %s=%q",key,value)
	}

	response:=FetchConfigResponse{
		Config:config,
	}

	payload,err:=encoder.Encode(&response)
	if err!=nil {
		logger.Error("UTIL: failed to encode FetchClientConfig response: %v",err)
		return nil
	}

	packet:=blaze.EncodePacket(UtilComponent, cmdFetchClientConfig, blaze.PacketTypeResponse, messageID, payload,)
	logger.Info("UTIL: FetchClientConfig payload=%d bytes packet=%d",len(payload),len(packet))
	return packet
}

func HandlePreAuth(messageID uint32) []byte {
	return BuildPreAuthResponse(messageID)
}

func HandlePing(messageID uint32) []byte {
	return BuildPingResponse(messageID)
}

func HandleFetchClientConfig(messageID uint32,configSection string) []byte {
	return BuildFetchClientConfigResponse(messageID,configSection)
}
