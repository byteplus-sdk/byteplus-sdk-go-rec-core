package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/metrics"
	"github.com/valyala/fasthttp"
)

type HostAvailabler interface {
	GetHosts() []string

	GetHost(path string) string

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

type HostAvailablerBase struct {
	projectID            string
	fetchHostsHTTPClient *fasthttp.Client
	defaultHosts         []string
	hostConfig           map[string][]string
	hostScorer           HostScorer
	stop                 chan bool
}

func (a *HostAvailablerBase) Init(defaultHosts []string, fetchHostInterval, scoreHostInterval time.Duration) error {
	if len(defaultHosts) == 0 {
		return errors.New("default hosts are empty")
	}
	a.setHosts(defaultHosts)
	a.stop = make(chan bool)
	if len(a.projectID) > 0 {
		a.fetchHostsHTTPClient = &fasthttp.Client{}
		a.fetchHostsFromServer()
		a.scheduleFetchHostsFromServer(fetchHostInterval)
	}
	a.scheduleScoreAndUpdateHosts(scoreHostInterval)
	return nil
}

// setHosts
// clear origin host config, and use hosts as default config
// {
//   "*": {
//     "*": ${hosts}
//   }
// }
func (a *HostAvailablerBase) setHosts(hosts []string) {
	a.defaultHosts = hosts
	a.hostConfig = map[string][]string{
		"*": hosts,
	}
	a.stopFetchHostsFromServer()
	a.doScoreAndUpdateHosts(a.hostConfig)
}

func (a *HostAvailablerBase) stopFetchHostsFromServer() {
	if a.stop != nil {
		close(a.stop)
	}
}

func (a *HostAvailablerBase) scheduleScoreAndUpdateHosts(scoreHostInterval time.Duration) {
	AsyncExecute(func() {
		ticker := time.NewTicker(scoreHostInterval)
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
func (a *HostAvailablerBase) doScoreAndUpdateHosts(hostConfig map[string][]string) {
	logID := "score_" + uuid.NewString()
	hosts := a.distinctHosts(hostConfig)
	newHostScores := a.hostScorer.ScoreHosts(hosts)
	metrics.Info(logID, "[ByteplusSDK][Score]score hosts, project_id:%s, result:%s", a.projectID, newHostScores)
	logs.Debug("score hosts result: %s", newHostScores)
	if len(newHostScores) == 0 {
		metricsTags := []string{
			"type:scoring_hosts_return_empty_list",
			"project_id:" + a.projectID,
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(logID, "[ByteplusSDK][Score] scoring hosts return an empty list, project_id:%s", a.projectID)
		logs.Error("scoring hosts return an empty list")
		return
	}
	newHostConfig := a.copyAndSortHost(hostConfig, newHostScores)
	if a.isHostConfigNotUpdated(a.hostConfig, newHostConfig) {
		metrics.Info(logID, "[ByteplusSDK][Score] host order is not changed, project_id:%s, config:%+v",
			a.projectID, newHostConfig)
		logs.Debug("host order is not changed, %+v", newHostConfig)
		return
	}
	metricsTags := []string{
		"type:set_new_host_config",
		"project_id:" + a.projectID,
	}
	metrics.Counter(metricsKeyCommonInfo, 1, metricsTags...)
	metrics.Info(logID, "[ByteplusSDK][Score] set new host config: %+v, old config: %+v, project_id:%s",
		newHostConfig, a.hostConfig, a.projectID)
	logs.Debug("set new host config: %+v, old config: %+v", newHostConfig, a.hostConfig)
	a.hostConfig = newHostConfig
}

func (a *HostAvailablerBase) distinctHosts(hostConfig map[string][]string) []string {
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

func (a *HostAvailablerBase) copyAndSortHost(hostConfig map[string][]string,
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

func (a *HostAvailablerBase) isHostConfigNotUpdated(oldHostConfig, newHostConfig map[string][]string) bool {
	if oldHostConfig == nil {
		return false
	}
	if newHostConfig == nil {
		return true
	}
	if len(oldHostConfig) != len(newHostConfig) {
		return false
	}
	for path, oldHosts := range a.hostConfig {
		newHosts := newHostConfig[path]
		if !a.isEqualHosts(oldHosts, newHosts) {
			return false
		}
	}
	return true
}

func (a *HostAvailablerBase) isEqualHosts(hostsA, hostsB []string) bool {
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

func (a *HostAvailablerBase) scheduleFetchHostsFromServer(fetchHostInterval time.Duration) {
	AsyncExecute(func() {
		ticker := time.NewTicker(fetchHostInterval)
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

func (a *HostAvailablerBase) fetchHostsFromServer() {
	url := fmt.Sprintf("http://%s/data/api/sdk/host?project_id=%s", a.defaultHosts[0], a.projectID)
	reqID := "fetch_" + uuid.NewString()
	for i := 0; i < 3; i++ {
		rspHostConfig := a.doFetchHostsFromServer(reqID, url)
		if rspHostConfig == nil {
			continue
		}
		if a.isServerHostsNotUpdated(rspHostConfig) {
			logFormat := "[ByteplusSDK][Fetch] hosts from server are not changed, project_id:%s, url: %s config: %+v"
			metrics.Info(reqID, logFormat, a.projectID, url, rspHostConfig)
			logs.Debug("hosts from server are not changed, url: %s config: %+v", url, rspHostConfig)
			return
		}
		if hosts, exist := rspHostConfig["*"]; !exist || len(hosts) == 0 {
			metricsTags := []string{
				"type:no_default_hosts",
				"project_id:" + a.projectID,
				"url:" + escapeMetricsTagValue(url),
			}
			metrics.Counter(metricsKeyCommonWarn, 1, metricsTags...)
			logFormat := "[ByteplusSDK][Fetch] no default value in hosts from server, project_id:%s, url: %s, config: %+v"
			metrics.Warn(reqID, logFormat, a.projectID, url, rspHostConfig)
			logs.Warn("no default value in hosts from server, url: %s, config: %+v", url, rspHostConfig)
			return
		}
		a.doScoreAndUpdateHosts(rspHostConfig)
		return
	}
	metricsTags := []string{
		"type:fetch_host_fail_although_retried",
		"project_id:" + a.projectID,
		"url:" + escapeMetricsTagValue(url),
	}
	metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
	logFormat := "[ByteplusSDK][Fetch] fetch host from server fail although retried, project_id:%s, url: %s"
	metrics.Warn(reqID, logFormat, a.projectID, url)
	logs.Warn("fetch host from server fail although retried, url: %s", url)
}

func (a *HostAvailablerBase) doFetchHostsFromServer(reqID, url string) map[string][]string {
	rspHostConfig := make(map[string][]string)
	request := fasthttp.AcquireRequest()
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	request.SetRequestURI(url)
	request.Header.SetMethod(fasthttp.MethodGet)
	request.Header.Set("Request-Id", reqID)
	start := time.Now()
	err := a.fetchHostsHTTPClient.DoTimeout(request, response, 5*time.Second)
	cost := time.Now().Sub(start)
	if err != nil {
		metricsTags := []string{
			"type:fetch_host_fail",
			"project_id:" + a.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		logFormat := "[ByteplusSDK][Fetch] fetch host from server fail, project_id:%s, url:%s, cost:%dms, err:%v"
		metrics.Warn(reqID, logFormat, a.projectID, url, cost.Milliseconds(), err)
		logs.Warn("fetch host from server fail, url:%s cost:%dms err:%v", url, cost.Milliseconds(), err)
		return nil
	}
	if response.StatusCode() == fasthttp.StatusNotFound {
		metricsTags := []string{
			"type:fetch_host_status_400",
			"project_id:" + a.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		logFormat := "[ByteplusSDK][Fetch] fetch host from server return not found status, project_id:%s, cost:%dms"
		metrics.Warn(reqID, logFormat, a.projectID, cost.Milliseconds())
		logs.Warn("fetch host from server return not found status, cost:%dms", cost.Milliseconds())
		return map[string][]string{}
	}
	if response.StatusCode() != fasthttp.StatusOK {
		metricsTags := []string{
			"type:fetch_host_not_ok",
			"project_id:" + a.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		logFormat := "[ByteplusSDK][Fetch] fetch host from server return not ok, project_id:%s, status:%d, cost:%dms"
		metrics.Warn(reqID, logFormat, a.projectID, response.StatusCode(), cost.Milliseconds())
		logs.Warn("fetch host from server return not ok status:%d cost:%dms", response.StatusCode(),
			cost.Milliseconds())
		return nil
	}
	rspBytes := response.Body()
	metricsTags := []string{
		"project_id:" + a.projectID,
		"url:" + escapeMetricsTagValue(url),
	}
	metrics.Counter(metricsKeyRequestCount, 1, metricsTags...)
	metrics.Timer(metricsKeyRequestTotalCost, cost.Milliseconds(), metricsTags...)
	logFormat := "[ByteplusSDK][Fetch] fetch host from server, project_id:%s, cost:%dms, rsp:%s"
	metrics.Info(reqID, logFormat, a.projectID, cost.Milliseconds(), rspBytes)
	logs.Debug("fetch host from server, cost:%dms rsp:%s", cost.Milliseconds(), rspBytes)
	if len(rspBytes) > 0 {
		err = json.Unmarshal(rspBytes, &rspHostConfig)
		if err != nil {
			metricsTags = []string{
				"type:unmarshal_host_config_fail",
				"project_id:" + a.projectID,
				"url:" + escapeMetricsTagValue(url),
			}
			metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
			logFormat = "[ByteplusSDK][Fetch] unmarshal host config from host server fail, project_id:%s, url:%s, cost:%dms, err:%v"
			metrics.Error(reqID, logFormat, a.projectID, url, cost.Milliseconds(), err)
			logs.Warn("unmarshal host config from host server fail, url:%s cost:%dms err:%v",
				url, cost.Milliseconds(), err)
			return map[string][]string{}
		}
		return rspHostConfig
	}
	logs.Warn("hosts from server are empty")
	return map[string][]string{}
}

func (a *HostAvailablerBase) isServerHostsNotUpdated(newHostConfig map[string][]string) bool {
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

func (a *HostAvailablerBase) containsAll(hosts []string, hosts2 []string) bool {
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

func (a *HostAvailablerBase) GetHosts() []string {
	return a.distinctHosts(a.hostConfig)
}

func (a *HostAvailablerBase) GetHost(path string) string {
	pathHosts, exist := a.hostConfig[path]
	if exist && len(pathHosts) > 0 {
		return pathHosts[0]
	}
	return a.hostConfig["*"][0]
}

func (a *HostAvailablerBase) Shutdown() {
	if a.stop != nil {
		close(a.stop)
	}
}
