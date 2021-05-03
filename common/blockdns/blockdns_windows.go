// +build windows

package blockdns

import (
	"math"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"

	win "github.com/xtls/xray-core/common/blockdns/winsys"
)

func FixDnsLeakage(tunName string) error {
	// Open the engine with a session.
	var engine uintptr
	session := &win.FWPM_SESSION0{Flags: win.FWPM_SESSION_FLAG_DYNAMIC}
	err := win.FwpmEngineOpen0(nil, win.RPC_C_AUTHN_DEFAULT, nil, session, unsafe.Pointer(&engine))
	if err != nil {
		return newError("failed to open engine").Base(err)
	}

	// Add a sublayer.
	key, err := windows.GenerateGUID()
	if err != nil {
		return newError("failed to generate GUID").Base(err)
	}
	displayData, err := win.CreateDisplayData("Xray", "Sublayer")
	if err != nil {
		return newError("failed to create display data").Base(err)
	}
	sublayer := win.FWPM_SUBLAYER0{}
	sublayer.SubLayerKey = key
	sublayer.DisplayData = *displayData
	sublayer.Weight = math.MaxUint16
	err = win.FwpmSubLayerAdd0(engine, &sublayer, 0)
	if err != nil {
		return newError("failed to add sublayer").Base(err)
	}

	var filterId uint64

	// Block all IPv6 traffic.
	blockV6FilterDisplayData, err := win.CreateDisplayData("Xray", "Block all IPv6 traffic")
	if err != nil {
		return newError("failed to create block v6 filter filter display data").Base(err)
	}
	blockV6Filter := win.FWPM_FILTER0{}
	blockV6Filter.DisplayData = *blockV6FilterDisplayData
	blockV6Filter.SubLayerKey = key
	blockV6Filter.LayerKey = win.FWPM_LAYER_ALE_AUTH_CONNECT_V6
	blockV6Filter.Action.Type = win.FWP_ACTION_BLOCK
	blockV6Filter.Weight.Type = win.FWP_UINT8
	blockV6Filter.Weight.Value = uintptr(13)
	err = win.FwpmFilterAdd0(engine, &blockV6Filter, 0, &filterId)
	if err != nil {
		return newError("failed to add block v6 filter").Base(err)
	}
	newError("Added filter to block all IPv6 traffic").AtDebug().WriteToLog()

	// Allow all IPv4 traffic from the current process i.e. Mellow.
	appID, err := win.GetCurrentProcessAppID()
	if err != nil {
		return err
	}
	defer win.FwpmFreeMemory0(unsafe.Pointer(&appID))
	permitMellowCondition := make([]win.FWPM_FILTER_CONDITION0, 1)
	permitMellowCondition[0].FieldKey = win.FWPM_CONDITION_ALE_APP_ID
	permitMellowCondition[0].MatchType = win.FWP_MATCH_EQUAL
	permitMellowCondition[0].ConditionValue.Type = win.FWP_BYTE_BLOB_TYPE
	permitMellowCondition[0].ConditionValue.Value = uintptr(unsafe.Pointer(appID))
	permitMellowFilterDisplayData, err := win.CreateDisplayData("Xray", "Permit all Mellow traffic")
	if err != nil {
		return newError("failed to create permit Mellow filter display data").Base(err)
	}
	permitMellowFilter := win.FWPM_FILTER0{}
	permitMellowFilter.FilterCondition = (*win.FWPM_FILTER_CONDITION0)(unsafe.Pointer(&permitMellowCondition[0]))
	permitMellowFilter.NumFilterConditions = 1
	permitMellowFilter.DisplayData = *permitMellowFilterDisplayData
	permitMellowFilter.SubLayerKey = key
	permitMellowFilter.LayerKey = win.FWPM_LAYER_ALE_AUTH_CONNECT_V4
	permitMellowFilter.Action.Type = win.FWP_ACTION_PERMIT
	permitMellowFilter.Weight.Type = win.FWP_UINT8
	permitMellowFilter.Weight.Value = uintptr(12)
	permitMellowFilter.Flags = win.FWPM_FILTER_FLAG_CLEAR_ACTION_RIGHT
	err = win.FwpmFilterAdd0(engine, &permitMellowFilter, 0, &filterId)
	if err != nil {
		return newError("failed to add permit Mellow filter").Base(err)
	}
	newError("Added filter to allow all traffic from Mellow").AtDebug().WriteToLog()

	// Allow all IPv4 traffic to the TAP adapter.
	iface, err := net.InterfaceByName(tunName)
	if err != nil {
		return newError("fialed to get interface by name " + tunName).Base(err)
	}
	tapWhitelistCondition := make([]win.FWPM_FILTER_CONDITION0, 1)
	tapWhitelistCondition[0].FieldKey = win.FWPM_CONDITION_LOCAL_INTERFACE_INDEX
	tapWhitelistCondition[0].MatchType = win.FWP_MATCH_EQUAL
	tapWhitelistCondition[0].ConditionValue.Type = win.FWP_UINT32
	tapWhitelistCondition[0].ConditionValue.Value = uintptr(uint32(iface.Index))
	tapWhitelistFilterDisplayData, err := win.CreateDisplayData("Xray", "Allow all traffic to the TAP device")
	if err != nil {
		return newError("failed to create tap device whitelist filter display data").Base(err)
	}
	tapWhitelistFilter := win.FWPM_FILTER0{}
	tapWhitelistFilter.FilterCondition = (*win.FWPM_FILTER_CONDITION0)(unsafe.Pointer(&tapWhitelistCondition[0]))
	tapWhitelistFilter.NumFilterConditions = 1
	tapWhitelistFilter.DisplayData = *tapWhitelistFilterDisplayData
	tapWhitelistFilter.SubLayerKey = key
	tapWhitelistFilter.LayerKey = win.FWPM_LAYER_ALE_AUTH_CONNECT_V4
	tapWhitelistFilter.Action.Type = win.FWP_ACTION_PERMIT
	tapWhitelistFilter.Weight.Type = win.FWP_UINT8
	tapWhitelistFilter.Weight.Value = uintptr(11)
	err = win.FwpmFilterAdd0(engine, &tapWhitelistFilter, 0, &filterId)
	if err != nil {
		return newError("failed to add tap device whitelist filter").Base(err)
	}
	newError("Added filter to allow all traffic to " + tunName).AtDebug().WriteToLog()

	// Block all UDP traffic targeting port 53.
	blockAllUDP53Condition := make([]win.FWPM_FILTER_CONDITION0, 2)
	blockAllUDP53Condition[0].FieldKey = win.FWPM_CONDITION_IP_PROTOCOL
	blockAllUDP53Condition[0].MatchType = win.FWP_MATCH_EQUAL
	blockAllUDP53Condition[0].ConditionValue.Type = win.FWP_UINT8
	blockAllUDP53Condition[0].ConditionValue.Value = uintptr(uint8(win.IPPROTO_UDP))
	blockAllUDP53Condition[1].FieldKey = win.FWPM_CONDITION_IP_REMOTE_PORT
	blockAllUDP53Condition[1].MatchType = win.FWP_MATCH_EQUAL
	blockAllUDP53Condition[1].ConditionValue.Type = win.FWP_UINT16
	blockAllUDP53Condition[1].ConditionValue.Value = uintptr(uint16(53))
	blockAllUDP53FilterDisplayData, err := win.CreateDisplayData("Xray", "Block all UDP traffic targeting port 53")
	if err != nil {
		return newError("failed to create filter display data").Base(err)
	}
	blockAllUDP53Filter := win.FWPM_FILTER0{}
	blockAllUDP53Filter.FilterCondition = (*win.FWPM_FILTER_CONDITION0)(unsafe.Pointer(&blockAllUDP53Condition[0]))
	blockAllUDP53Filter.NumFilterConditions = 2
	blockAllUDP53Filter.DisplayData = *blockAllUDP53FilterDisplayData
	blockAllUDP53Filter.SubLayerKey = key
	blockAllUDP53Filter.LayerKey = win.FWPM_LAYER_ALE_AUTH_CONNECT_V4
	blockAllUDP53Filter.Action.Type = win.FWP_ACTION_BLOCK
	blockAllUDP53Filter.Weight.Type = win.FWP_UINT8
	blockAllUDP53Filter.Weight.Value = uintptr(10)
	err = win.FwpmFilterAdd0(engine, &blockAllUDP53Filter, 0, &filterId)
	if err != nil {
		return newError("failed to add filter").Base(err)
	}
	newError("Added filter to block all udp traffic targeting 53 remote port").WriteToLog()

	return nil
}
