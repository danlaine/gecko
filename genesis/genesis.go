// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

// TODO: Move this to a separate repo and leave only a byte array

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ava-labs/gecko/ids"
	"github.com/ava-labs/gecko/vms/avm"
	"github.com/ava-labs/gecko/vms/evm"
	"github.com/ava-labs/gecko/vms/platformvm"
	"github.com/ava-labs/gecko/vms/spchainvm"
	"github.com/ava-labs/gecko/vms/spdagvm"
	"github.com/ava-labs/gecko/vms/timestampvm"
)

// Note that since an AVA network has exactly one Platform Chain,
// and the Platform Chain defines the genesis state of the network
// (who is staking, which chains exist, etc.), defining the genesis
// state of the Platform Chain is the same as defining the genesis
// state of the network.

// Hardcoded network IDs
const (
	MainnetID  uint32 = 1
	TestnetID  uint32 = 2
	BorealisID uint32 = 2
	LocalID    uint32 = 12345

	MainnetName  = "mainnet"
	TestnetName  = "testnet"
	BorealisName = "borealis"
	LocalName    = "local"
)

var (
	validNetworkName = regexp.MustCompile(`network-[0-9]+`)
)

// Hard coded genesis constants
var (
	// Give special names to the mainnet and testnet
	NetworkIDToNetworkName = map[uint32]string{
		MainnetID: MainnetName,
		TestnetID: BorealisName,
		LocalID:   LocalName,
	}
	NetworkNameToNetworkID = map[string]uint32{
		MainnetName:  MainnetID,
		TestnetName:  TestnetID,
		BorealisName: BorealisID,
		LocalName:    LocalID,
	}
	Keys = []string{
		"ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN",
	}
	Addresses = []string{
		"6Y3kysjF9jnHnYkdS9yGAuoHyae2eNmeV",
	}
	ParsedAddresses = []ids.ShortID{}
	StakerIDs       = []string{
		"7Xhw2mDxuDS44j42TCB6U5579esbSt3Lg",
		"MFrZFVCXPv5iCn6M9K6XduxGTYp891xXZ",
		"NFBbbJ4qCmNaCzeW7sxErhvWqvEQMnYcN",
		"GWPcbFJZFfZreETSoWjPimr846mXEKCtu",
		"P7oB2McjBGgW2NXXWVYjV8JEDFoW9xDE5",
	}
	ParsedStakerIDs = []ids.ShortID{}
)

func init() {
	for _, addrStr := range Addresses {
		addr, err := ids.ShortFromString(addrStr)
		if err != nil {
			panic(err)
		}
		ParsedAddresses = append(ParsedAddresses, addr)
	}
	for _, stakerIDStr := range StakerIDs {
		stakerID, err := ids.ShortFromString(stakerIDStr)
		if err != nil {
			panic(err)
		}
		ParsedStakerIDs = append(ParsedStakerIDs, stakerID)
	}
}

// NetworkName returns a human readable name for the network with
// ID [networkID]
func NetworkName(networkID uint32) string {
	if name, exists := NetworkIDToNetworkName[networkID]; exists {
		return name
	}
	return fmt.Sprintf("network-%d", networkID)
}

// NetworkID returns the ID of the network with name [networkName]
func NetworkID(networkName string) (uint32, error) {
	networkName = strings.ToLower(networkName)
	if id, exists := NetworkNameToNetworkID[networkName]; exists {
		return id, nil
	}

	if id, err := strconv.ParseUint(networkName, 10, 0); err == nil {
		if id > math.MaxUint32 {
			return 0, fmt.Errorf("NetworkID %s not in [0, 2^32)", networkName)
		}
		return uint32(id), nil
	}
	if validNetworkName.MatchString(networkName) {
		if id, err := strconv.Atoi(networkName[8:]); err == nil {
			if id > math.MaxUint32 {
				return 0, fmt.Errorf("NetworkID %s not in [0, 2^32)", networkName)
			}
			return uint32(id), nil
		}
	}

	return 0, fmt.Errorf("Failed to parse %s as a network name", networkName)
}

// Aliases returns the default aliases based on the network ID
func Aliases(networkID uint32) (generalAliases map[string][]string, chainAliases map[[32]byte][]string, vmAliases map[[32]byte][]string) {
	generalAliases = map[string][]string{
		"vm/" + platformvm.ID.String():  []string{"vm/platform"},
		"vm/" + avm.ID.String():         []string{"vm/avm"},
		"vm/" + evm.ID.String():         []string{"vm/evm"},
		"vm/" + spdagvm.ID.String():     []string{"vm/spdag"},
		"vm/" + spchainvm.ID.String():   []string{"vm/spchain"},
		"vm/" + timestampvm.ID.String(): []string{"vm/timestamp"},
		"bc/" + ids.Empty.String():      []string{"P", "platform", "bc/P", "bc/platform"},
	}
	chainAliases = map[[32]byte][]string{
		ids.Empty.Key(): []string{"P", "platform"},
	}
	vmAliases = map[[32]byte][]string{
		platformvm.ID.Key():  []string{"platform"},
		avm.ID.Key():         []string{"avm"},
		evm.ID.Key():         []string{"evm"},
		spdagvm.ID.Key():     []string{"spdag"},
		spchainvm.ID.Key():   []string{"spchain"},
		timestampvm.ID.Key(): []string{"timestamp"},
	}

	genesisBytes := Genesis(networkID)
	genesis := &platformvm.Genesis{}                  // TODO let's not re-create genesis to do aliasing
	platformvm.Codec.Unmarshal(genesisBytes, genesis) // TODO check for error
	genesis.Initialize()

	for _, chain := range genesis.Chains {
		switch {
		case avm.ID.Equals(chain.VMID):
			generalAliases["bc/"+chain.ID().String()] = []string{"X", "avm", "bc/X", "bc/avm"}
			chainAliases[chain.ID().Key()] = []string{"X", "avm"}
		case evm.ID.Equals(chain.VMID):
			generalAliases["bc/"+chain.ID().String()] = []string{"C", "evm", "bc/C", "bc/evm"}
			chainAliases[chain.ID().Key()] = []string{"C", "evm"}
		case spdagvm.ID.Equals(chain.VMID):
			generalAliases["bc/"+chain.ID().String()] = []string{"bc/spdag"}
			chainAliases[chain.ID().Key()] = []string{"spdag"}
		case spchainvm.ID.Equals(chain.VMID):
			generalAliases["bc/"+chain.ID().String()] = []string{"bc/spchain"}
			chainAliases[chain.ID().Key()] = []string{"spchain"}
		case timestampvm.ID.Equals(chain.VMID):
			generalAliases["bc/"+chain.ID().String()] = []string{"bc/timestamp"}
			chainAliases[chain.ID().Key()] = []string{"timestamp"}
		}
	}
	return
}

// Genesis returns the genesis data of the Platform Chain.
// Since the Platform Chain causes the creation of all other
// chains, this function returns the genesis data of the entire network.
// The ID of the new network is [networkID].
func Genesis(networkID uint32) []byte {
	if networkID != LocalID {
		panic("unknown network ID provided")
	}

	return []byte{
		0x00, 0x00, 0x00, 0x01, 0x3c, 0xb7, 0xd3, 0x84,
		0x2e, 0x8c, 0xee, 0x6a, 0x0e, 0xbd, 0x09, 0xf1,
		0xfe, 0x88, 0x4f, 0x68, 0x61, 0xe1, 0xb2, 0x9c,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00,
		0x05, 0xde, 0x31, 0xb4, 0xd8, 0xb2, 0x29, 0x91,
		0xd5, 0x1a, 0xa6, 0xaa, 0x1f, 0xc7, 0x33, 0xf2,
		0x3a, 0x85, 0x1a, 0x8c, 0x94, 0x00, 0x00, 0x12,
		0x30, 0x9c, 0xe5, 0x40, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x5d, 0xbb, 0x75, 0x80, 0x00, 0x00, 0x00,
		0x00, 0x5f, 0x9c, 0xa9, 0x00, 0x00, 0x00, 0x30,
		0x39, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x3c, 0xb7, 0xd3, 0x84, 0x2e, 0x8c, 0xee,
		0x6a, 0x0e, 0xbd, 0x09, 0xf1, 0xfe, 0x88, 0x4f,
		0x68, 0x61, 0xe1, 0xb2, 0x9c, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x05, 0xaa, 0x18,
		0xd3, 0x99, 0x1c, 0xf6, 0x37, 0xaa, 0x6c, 0x16,
		0x2f, 0x5e, 0x95, 0xcf, 0x16, 0x3f, 0x69, 0xcd,
		0x82, 0x91, 0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5,
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x5d, 0xbb,
		0x75, 0x80, 0x00, 0x00, 0x00, 0x00, 0x5f, 0x9c,
		0xa9, 0x00, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3c, 0xb7,
		0xd3, 0x84, 0x2e, 0x8c, 0xee, 0x6a, 0x0e, 0xbd,
		0x09, 0xf1, 0xfe, 0x88, 0x4f, 0x68, 0x61, 0xe1,
		0xb2, 0x9c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x05, 0xe9, 0x09, 0x4f, 0x73, 0x69,
		0x80, 0x02, 0xfd, 0x52, 0xc9, 0x08, 0x19, 0xb4,
		0x57, 0xb9, 0xfb, 0xc8, 0x66, 0xab, 0x80, 0x00,
		0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x5d, 0xbb, 0x75, 0x80, 0x00,
		0x00, 0x00, 0x00, 0x5f, 0x9c, 0xa9, 0x00, 0x00,
		0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x3c, 0xb7, 0xd3, 0x84, 0x2e,
		0x8c, 0xee, 0x6a, 0x0e, 0xbd, 0x09, 0xf1, 0xfe,
		0x88, 0x4f, 0x68, 0x61, 0xe1, 0xb2, 0x9c, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
		0x47, 0x9f, 0x66, 0xc8, 0xbe, 0x89, 0x58, 0x30,
		0x54, 0x7e, 0x70, 0xb4, 0xb2, 0x98, 0xca, 0xfd,
		0x43, 0x3d, 0xba, 0x6e, 0x00, 0x00, 0x12, 0x30,
		0x9c, 0xe5, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x5d, 0xbb, 0x75, 0x80, 0x00, 0x00, 0x00, 0x00,
		0x5f, 0x9c, 0xa9, 0x00, 0x00, 0x00, 0x30, 0x39,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x3c, 0xb7, 0xd3, 0x84, 0x2e, 0x8c, 0xee, 0x6a,
		0x0e, 0xbd, 0x09, 0xf1, 0xfe, 0x88, 0x4f, 0x68,
		0x61, 0xe1, 0xb2, 0x9c, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x05, 0xf2, 0x9b, 0xce,
		0x5f, 0x34, 0xa7, 0x43, 0x01, 0xeb, 0x0d, 0xe7,
		0x16, 0xd5, 0x19, 0x4e, 0x4a, 0x4a, 0xea, 0x5d,
		0x7a, 0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x5d, 0xbb, 0x75,
		0x80, 0x00, 0x00, 0x00, 0x00, 0x5f, 0x9c, 0xa9,
		0x00, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x3c, 0xb7, 0xd3,
		0x84, 0x2e, 0x8c, 0xee, 0x6a, 0x0e, 0xbd, 0x09,
		0xf1, 0xfe, 0x88, 0x4f, 0x68, 0x61, 0xe1, 0xb2,
		0x9c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x05, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
		0x41, 0x56, 0x4d, 0x61, 0x76, 0x6d, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x73,
		0x65, 0x63, 0x70, 0x32, 0x35, 0x36, 0x6b, 0x31,
		0x66, 0x78, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x7c, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x03, 0x41, 0x56, 0x41, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x03, 0x41, 0x56, 0x41, 0x00, 0x03, 0x41,
		0x56, 0x41, 0x09, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x04, 0x00, 0x9f, 0xdf, 0x42, 0xf6,
		0xe4, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x01, 0x3c, 0xb7, 0xd3, 0x84, 0x2e,
		0x8c, 0xee, 0x6a, 0x0e, 0xbd, 0x09, 0xf1, 0xfe,
		0x88, 0x4f, 0x68, 0x61, 0xe1, 0xb2, 0x9c, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x41, 0x74,
		0x68, 0x65, 0x72, 0x65, 0x75, 0x6d, 0x65, 0x76,
		0x6d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x02, 0xc9, 0x7b, 0x22,
		0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x3a,
		0x7b, 0x22, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x49,
		0x64, 0x22, 0x3a, 0x34, 0x33, 0x31, 0x31, 0x30,
		0x2c, 0x22, 0x68, 0x6f, 0x6d, 0x65, 0x73, 0x74,
		0x65, 0x61, 0x64, 0x42, 0x6c, 0x6f, 0x63, 0x6b,
		0x22, 0x3a, 0x30, 0x2c, 0x22, 0x64, 0x61, 0x6f,
		0x46, 0x6f, 0x72, 0x6b, 0x42, 0x6c, 0x6f, 0x63,
		0x6b, 0x22, 0x3a, 0x30, 0x2c, 0x22, 0x64, 0x61,
		0x6f, 0x46, 0x6f, 0x72, 0x6b, 0x53, 0x75, 0x70,
		0x70, 0x6f, 0x72, 0x74, 0x22, 0x3a, 0x74, 0x72,
		0x75, 0x65, 0x2c, 0x22, 0x65, 0x69, 0x70, 0x31,
		0x35, 0x30, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x22,
		0x3a, 0x30, 0x2c, 0x22, 0x65, 0x69, 0x70, 0x31,
		0x35, 0x30, 0x48, 0x61, 0x73, 0x68, 0x22, 0x3a,
		0x22, 0x30, 0x78, 0x32, 0x30, 0x38, 0x36, 0x37,
		0x39, 0x39, 0x61, 0x65, 0x65, 0x62, 0x65, 0x61,
		0x65, 0x31, 0x33, 0x35, 0x63, 0x32, 0x34, 0x36,
		0x63, 0x36, 0x35, 0x30, 0x32, 0x31, 0x63, 0x38,
		0x32, 0x62, 0x34, 0x65, 0x31, 0x35, 0x61, 0x32,
		0x63, 0x34, 0x35, 0x31, 0x33, 0x34, 0x30, 0x39,
		0x39, 0x33, 0x61, 0x61, 0x63, 0x66, 0x64, 0x32,
		0x37, 0x35, 0x31, 0x38, 0x38, 0x36, 0x35, 0x31,
		0x34, 0x66, 0x30, 0x22, 0x2c, 0x22, 0x65, 0x69,
		0x70, 0x31, 0x35, 0x35, 0x42, 0x6c, 0x6f, 0x63,
		0x6b, 0x22, 0x3a, 0x30, 0x2c, 0x22, 0x65, 0x69,
		0x70, 0x31, 0x35, 0x38, 0x42, 0x6c, 0x6f, 0x63,
		0x6b, 0x22, 0x3a, 0x30, 0x2c, 0x22, 0x62, 0x79,
		0x7a, 0x61, 0x6e, 0x74, 0x69, 0x75, 0x6d, 0x42,
		0x6c, 0x6f, 0x63, 0x6b, 0x22, 0x3a, 0x30, 0x2c,
		0x22, 0x63, 0x6f, 0x6e, 0x73, 0x74, 0x61, 0x6e,
		0x74, 0x69, 0x6e, 0x6f, 0x70, 0x6c, 0x65, 0x42,
		0x6c, 0x6f, 0x63, 0x6b, 0x22, 0x3a, 0x30, 0x2c,
		0x22, 0x70, 0x65, 0x74, 0x65, 0x72, 0x73, 0x62,
		0x75, 0x72, 0x67, 0x42, 0x6c, 0x6f, 0x63, 0x6b,
		0x22, 0x3a, 0x30, 0x7d, 0x2c, 0x22, 0x6e, 0x6f,
		0x6e, 0x63, 0x65, 0x22, 0x3a, 0x22, 0x30, 0x78,
		0x30, 0x22, 0x2c, 0x22, 0x74, 0x69, 0x6d, 0x65,
		0x73, 0x74, 0x61, 0x6d, 0x70, 0x22, 0x3a, 0x22,
		0x30, 0x78, 0x30, 0x22, 0x2c, 0x22, 0x65, 0x78,
		0x74, 0x72, 0x61, 0x44, 0x61, 0x74, 0x61, 0x22,
		0x3a, 0x22, 0x30, 0x78, 0x30, 0x30, 0x22, 0x2c,
		0x22, 0x67, 0x61, 0x73, 0x4c, 0x69, 0x6d, 0x69,
		0x74, 0x22, 0x3a, 0x22, 0x30, 0x78, 0x35, 0x66,
		0x35, 0x65, 0x31, 0x30, 0x30, 0x22, 0x2c, 0x22,
		0x64, 0x69, 0x66, 0x66, 0x69, 0x63, 0x75, 0x6c,
		0x74, 0x79, 0x22, 0x3a, 0x22, 0x30, 0x78, 0x30,
		0x22, 0x2c, 0x22, 0x6d, 0x69, 0x78, 0x48, 0x61,
		0x73, 0x68, 0x22, 0x3a, 0x22, 0x30, 0x78, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x22,
		0x2c, 0x22, 0x63, 0x6f, 0x69, 0x6e, 0x62, 0x61,
		0x73, 0x65, 0x22, 0x3a, 0x22, 0x30, 0x78, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x22,
		0x2c, 0x22, 0x61, 0x6c, 0x6c, 0x6f, 0x63, 0x22,
		0x3a, 0x7b, 0x22, 0x37, 0x35, 0x31, 0x61, 0x30,
		0x62, 0x39, 0x36, 0x65, 0x31, 0x30, 0x34, 0x32,
		0x62, 0x65, 0x65, 0x37, 0x38, 0x39, 0x34, 0x35,
		0x32, 0x65, 0x63, 0x62, 0x32, 0x30, 0x32, 0x35,
		0x33, 0x66, 0x62, 0x61, 0x34, 0x30, 0x64, 0x62,
		0x65, 0x38, 0x35, 0x22, 0x3a, 0x7b, 0x22, 0x62,
		0x61, 0x6c, 0x61, 0x6e, 0x63, 0x65, 0x22, 0x3a,
		0x22, 0x30, 0x78, 0x33, 0x33, 0x62, 0x32, 0x65,
		0x33, 0x63, 0x39, 0x66, 0x64, 0x30, 0x38, 0x30,
		0x34, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x22, 0x7d, 0x7d, 0x2c, 0x22, 0x6e,
		0x75, 0x6d, 0x62, 0x65, 0x72, 0x22, 0x3a, 0x22,
		0x30, 0x78, 0x30, 0x22, 0x2c, 0x22, 0x67, 0x61,
		0x73, 0x55, 0x73, 0x65, 0x64, 0x22, 0x3a, 0x22,
		0x30, 0x78, 0x30, 0x22, 0x2c, 0x22, 0x70, 0x61,
		0x72, 0x65, 0x6e, 0x74, 0x48, 0x61, 0x73, 0x68,
		0x22, 0x3a, 0x22, 0x30, 0x78, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
		0x30, 0x30, 0x30, 0x30, 0x30, 0x22, 0x7d, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x30, 0x39, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x13, 0x53, 0x69,
		0x6d, 0x70, 0x6c, 0x65, 0x20, 0x44, 0x41, 0x47,
		0x20, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74,
		0x73, 0x73, 0x70, 0x64, 0x61, 0x67, 0x76, 0x6d,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x60, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x12, 0x30, 0x9c, 0xe5, 0x40,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x3c, 0xb7, 0xd3, 0x84, 0x2e, 0x8c, 0xee,
		0x6a, 0x0e, 0xbd, 0x09, 0xf1, 0xfe, 0x88, 0x4f,
		0x68, 0x61, 0xe1, 0xb2, 0x9c, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x30, 0x39, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x15,
		0x53, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x20, 0x43,
		0x68, 0x61, 0x69, 0x6e, 0x20, 0x50, 0x61, 0x79,
		0x6d, 0x65, 0x6e, 0x74, 0x73, 0x73, 0x70, 0x63,
		0x68, 0x61, 0x69, 0x6e, 0x76, 0x6d, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00,
		0x01, 0x3c, 0xb7, 0xd3, 0x84, 0x2e, 0x8c, 0xee,
		0x6a, 0x0e, 0xbd, 0x09, 0xf1, 0xfe, 0x88, 0x4f,
		0x68, 0x61, 0xe1, 0xb2, 0x9c, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12,
		0x30, 0x9c, 0xe5, 0x40, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x30, 0x39, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x17, 0x53, 0x69, 0x6d, 0x70,
		0x6c, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, 0x73,
		0x74, 0x61, 0x6d, 0x70, 0x20, 0x53, 0x65, 0x72,
		0x76, 0x65, 0x72, 0x74, 0x69, 0x6d, 0x65, 0x73,
		0x74, 0x61, 0x6d, 0x70, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x5d, 0xbb, 0x75, 0x80,
	}
}

// VMGenesis ...
func VMGenesis(networkID uint32, vmID ids.ID) *platformvm.CreateChainTx {
	genesisBytes := Genesis(networkID)
	genesis := platformvm.Genesis{}
	platformvm.Codec.Unmarshal(genesisBytes, &genesis)
	for _, chain := range genesis.Chains {
		if chain.VMID.Equals(vmID) {
			return chain
		}
	}
	return nil
}
