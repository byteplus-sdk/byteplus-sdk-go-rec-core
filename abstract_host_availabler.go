package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/valyala/fasthttp"
)

type HostAvailabler interface {
	GetHost() string
	GetHostByPath(path string) string
	Shutdown()
}

type HostScorer interface {
	ScoreHosts(hosts []string) []*HostAvailabilityScore
}

type HostAvailabilityScore struct {
	Host  string
	Score float64
}

func (h *HostAvailabilityScore) String() string {
	return fmt.Sprintf("%+v", *h)
}

type AbstractHostAvailabler struct {
	projectID            string
	fetchHostsHTTPClient *fasthttp.Client
	hostConfig           map[string][]string
	hostScorer           HostScorer
	stop                 chan bool
}

func NewAbstractHostAvailabler(defaultHosts []string, hostScorer HostScorer) (*AbstractHostAvailabler, error) {
	if len(defaultHosts) == 0 {
		return nil, errors.New("default hosts are empty")
	}
	abstractHostAvailabler := &AbstractHostAvailabler{
		hostScorer: hostScorer,
	}
	abstractHostAvailabler.Init(defaultHosts, true)
	return abstractHostAvailabler, nil
}

func NewAbstractHostAvailablerWithProjectID(
	defaultHosts []string,
	projectID string,
	closeFetchHostsFromServer bool,
	hostScorer HostScorer) (*AbstractHostAvailabler, error) {
	if len(defaultHosts) == 0 {
		return nil, errors.New("default hosts are empty")
	}
	if len(projectID) == 0 {
		return nil, errors.New("projectID is empty")
	}
	abstractHostAvailabler := &AbstractHostAvailabler{
		projectID:  projectID,
		hostScorer: hostScorer,
	}
	abstractHostAvailabler.Init(defaultHosts, closeFetchHostsFromServer)
	return abstractHostAvailabler, nil
}

func (a *AbstractHostAvailabler) Init(defaultHosts []string, closeFetchHostsFromServer bool) {
	a.setHosts(defaultHosts)
	a.stop = make(chan bool)
	if !closeFetchHostsFromServer {
		a.fetchHostsHTTPClient = &fasthttp.Client{}
		a.fetchHostsFromServer()
		a.scheduleFetchHostsFromServer()
	}
	a.scheduleScoreAndUpdateHosts()

}

// setHosts
// clear origin host config, and use hosts as default config
// {
//   "*": {
//     "*": ${hosts}
//   }
// }
func (a *AbstractHostAvailabler) setHosts(hosts []string) {
	a.hostConfig = map[string][]string{
		"*": hosts,
	}
	a.stopFetchHostsFromServer()
	a.doScoreAndUpdateHosts(a.hostConfig)
}

func (a *AbstractHostAvailabler) stopFetchHostsFromServer() {
	if a.stop != nil {
		close(a.stop)
	}
}

func (a *AbstractHostAvailabler) scheduleScoreAndUpdateHosts() {
	AsyncExecute(func() {
		ticker := time.NewTicker(time.Second)
		for true {
			select {
			case <-a.stop:
				ticker.Stop()
				return
			case <-ticker.C:
				a.doScoreAndUpdateHosts(a.hostConfig)
			}
		}
	})
}

// doScoreAndUpdateHosts
// path->host_array
// example:
// {
//     "*": ["bytedance.com", "byteplus.com"],
//     "WriteUsers": ["b-bytedance.com", "b-byteplus.com"],
//     "Predict": ["c-bytedance.com", "c-byteplus.com"]
// }
// default config is required:
// {
//   "*": ["bytedance.com", "byteplus.com"]
// }
func (a *AbstractHostAvailabler) doScoreAndUpdateHosts(hostConfig map[string][]string) {
	hosts := a.distinctHosts(hostConfig)
	newHostScores := a.hostScorer.ScoreHosts(hosts)
	logs.Debug("[ByteplusSDK] score hosts result: %s", newHostScores)
	if len(newHostScores) == 0 {
		logs.Error("[ByteplusSDK] scoring hosts return an empty list")
		return
	}
	newHostConfig := a.copyAndSortHost(hostConfig, newHostScores)
	if a.isHostConfigNotUpdated(hostConfig, newHostConfig) {
		logs.Debug("[ByteplusSDK] host order is not changed, %+v", newHostConfig)
		return
	}
	logs.Debug("[ByteplusSDK] set new host config: %+v, old config: %+v", newHostConfig, a.hostConfig)
	a.hostConfig = newHostConfig
}

func (a *AbstractHostAvailabler) distinctHosts(hostConfig map[string][]string) []string {
	result := make([]string, 0)
	hostMap := make(map[string]bool)
	for _, hosts := range hostConfig {
		for _, host := range hosts {
			if _, exist := hostMap[host]; exist {
				continue
			}
			result = append(result, host)
			hostMap[host] = true
		}
	}
	return result
}

func (a *AbstractHostAvailabler) copyAndSortHost(hostConfig map[string][]string,
	newHostScores []*HostAvailabilityScore) map[string][]string {
	hostScoreIndex := make(map[string]float64, len(newHostScores))
	for _, hostScore := range newHostScores {
		hostScoreIndex[hostScore.Host] = hostScore.Score
	}
	newHostConfig := make(map[string][]string, len(hostConfig))

	for path, hosts := range hostConfig {
		newHosts := make([]string, len(hosts))
		copy(newHosts, hosts)
		// from big to small
		sort.Slice(newHosts, func(i, j int) bool {
			return hostScoreIndex[newHosts[i]] > hostScoreIndex[newHosts[j]]
		})
		newHostConfig[path] = newHosts
	}
	return newHostConfig
}

func (a *AbstractHostAvailabler) isHostConfigNotUpdated(oldHostConfig, newHostConfig map[string][]string) bool {
	if oldHostConfig == nil {
		return false
	}
	if newHostConfig == nil {
		return true
	}
	for path, oldHosts := range a.hostConfig {
		newHosts := newHostConfig[path]
		if !a.isEqualHosts(oldHosts, newHosts) {
			return false
		}
	}
	return true
}

func (a *AbstractHostAvailabler) isEqualHosts(hostsA, hostsB []string) bool {
	if len(hostsA) != len(hostsB) {
		return false
	}
	for i, _ := range hostsA {
		if hostsA[i] != hostsB[i] {
			return false
		}
	}
	return true
}

func (a *AbstractHostAvailabler) scheduleFetchHostsFromServer() {
	AsyncExecute(func() {
		ticker := time.NewTicker(time.Second * 10)
		for true {
			select {
			case <-a.stop:
				ticker.Stop()
				return
			case <-ticker.C:
				a.fetchHostsFromServer()
			}
		}
	})
}

func (a *AbstractHostAvailabler) fetchHostsFromServer() {
	url := fmt.Sprintf("http://%s/data/api/sdk/host?project_id=%s", a.GetHost(), a.projectID)
	for i := 0; i < 3; i++ {
		rspHostConfig := a.doFetchHostsFromServer(url)
		if len(rspHostConfig) == 0 {
			continue
		}
		if a.isServerHostsNotUpdated(rspHostConfig) {
			logs.Debug("[ByteplusSDK] hosts from server are not changed, config: %+v", rspHostConfig)
			return
		}
		if hosts, exist := rspHostConfig["*"]; exist || len(hosts) == 0 {
			logs.Warn("[ByteplusSDK] hosts from server is empty, url:'%s' config: %+v", url, rspHostConfig)
			return
		}
		a.doScoreAndUpdateHosts(rspHostConfig)
		return
	}
	logs.Warn("[ByteplusSDK] fetch host from server fail although retried, url: {}", url)
}

func (a *AbstractHostAvailabler) doFetchHostsFromServer(url string) map[string][]string {
	rspHostConfig := make(map[string][]string)
	request := fasthttp.AcquireRequest()
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	request.SetRequestURI(url)
	request.Header.SetMethod(fasthttp.MethodGet)
	start := time.Now()
	err := a.fetchHostsHTTPClient.DoTimeout(request, response, time.Second*5)
	cost := time.Now().Sub(start)
	if err != nil {
		logs.Warn("[ByteplusSDK] fetch host from server fail, url:%s cost:%s err:%s", url, cost, err.Error())
		return nil
	}
	if response.StatusCode() == fasthttp.StatusNotFound {
		logs.Warn("[ByteplusSDK] fetch host from server return not found status, cost:%s", cost)
		return nil
	}
	if response.StatusCode() != fasthttp.StatusOK {
		logs.Warn("[ByteplusSDK] fetch host from server return not ok status:%d cost:%s", response.StatusCode(), cost)
		return nil
	}
	rspBytes := response.Body()
	logs.Debug("[ByteplusSDK] fetch host from server, cost:%s rsp:%s", cost, rspBytes)
	if len(rspBytes) > 0 {
		json.Unmarshal(rspBytes, &rspHostConfig)
		return rspHostConfig
	}
	return nil
}

func (a *AbstractHostAvailabler) isServerHostsNotUpdated(newHostConfig map[string][]string) bool {
	if len(newHostConfig) != len(a.hostConfig) {
		return false
	}
	for path, newHosts := range newHostConfig {
		oldHosts, exist := a.hostConfig[path]
		if !exist {
			return false
		}
		if len(oldHosts) != len(newHosts) {
			return false
		}
		if !a.containsAll(newHosts, oldHosts) {
			return false
		}
	}
	return true
}

func (a *AbstractHostAvailabler) containsAll(hosts []string, hosts2 []string) bool {
	hostIndexMap := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		hostIndexMap[host] = true
	}
	for _, host := range hosts2 {
		if !hostIndexMap[host] {
			return false
		}
	}
	return true
}

func (a *AbstractHostAvailabler) GetHost() string {
	return a.hostConfig["*"][0]
}

func (a *AbstractHostAvailabler) GetHostByPath(path string) string {
	pathHosts, exist := a.hostConfig[path]
	if exist && len(pathHosts) > 0 {
		return pathHosts[0]
	}
	return a.hostConfig["*"][0]
}

func (a *AbstractHostAvailabler) Shutdown() {
	if a.stop != nil {
		close(a.stop)
	}
}
