package core

import (
	"fmt"
)

type Region string

const (
	regionUnknown Region = ""
)

// RegionConfig some attributes bound to the region
type RegionConfig struct {
	Hosts                []string
	VolcCredentialRegion string
}

// regionHostsMap record region->regionInfo mapping for sdk which build with sdk-core-go-copy
// sdk can register its own region and regionInfo
var regionHostsMap = make(map[Region]*RegionConfig)

func RegisterRegion(region Region, regionInfo *RegionConfig) {
	if _, exist := regionHostsMap[region]; exist {
		panic(fmt.Sprintf("region has already exist: %s", region))
	}
	regionHostsMap[region] = regionInfo
}

func getRegionConfig(region Region) *RegionConfig {
	return regionHostsMap[region]
}

func getRegionHosts(region Region) []string {
	return regionHostsMap[region].Hosts
}

func getVolcCredentialRegion(region Region) string {
	return regionHostsMap[region].VolcCredentialRegion
}
