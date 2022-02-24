package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/valyala/fasthttp"
)

const (
	defaultPingURLFormat = "http://%s/predict/api/ping"
	defaultWindowSize    = 60
	defaultPingTimeout   = 300 * time.Millisecond
)

type PingHostAvailablerConfig struct {
	// host availabler used to test the latency, example {}://%s/predict/api/ping
	// {} will be replaced by schema which set in context
	// %s will be dynamically formatted by hosts
	PingUrlFormat string
	// record the window size of each host's test status
	WindowSize int
	// timeout for requesting hosts
	PingTimeout               time.Duration
	CloseFetchHostsFromServer bool
}

type pingHostAvailabler struct {
	*AbstractHostAvailabler
	config        *PingHostAvailablerConfig
	hostWindowMap map[string]*window
	httpCli       *fasthttp.Client
}

func NewPingHostAvailabler(hosts []string, config *PingHostAvailablerConfig) (HostAvailabler, error) {
	fillDefaultConfig(config)
	hostAvailabler := &pingHostAvailabler{
		config:        config,
		httpCli:       &fasthttp.Client{},
		hostWindowMap: make(map[string]*window, len(hosts)),
	}
	abstractHostAvailabler, err := NewAbstractHostAvailabler(hosts, hostAvailabler)
	if err != nil {
		return nil, err
	}
	hostAvailabler.AbstractHostAvailabler = abstractHostAvailabler
	return hostAvailabler, err
}

func NewPingHostAvailablerWithProjectID(hosts []string, projectID string,
	config *PingHostAvailablerConfig) (HostAvailabler, error) {
	fillDefaultConfig(config)
	hostAvailabler := &pingHostAvailabler{
		config:        config,
		httpCli:       &fasthttp.Client{},
		hostWindowMap: make(map[string]*window),
	}
	abstractHostAvailabler, err := NewAbstractHostAvailablerWithProjectID(
		hosts,
		projectID,
		config.CloseFetchHostsFromServer,
		hostAvailabler,
	)
	if err != nil {
		return nil, err
	}
	hostAvailabler.AbstractHostAvailabler = abstractHostAvailabler
	return hostAvailabler, nil
}

func fillDefaultConfig(config *PingHostAvailablerConfig) {
	if config.PingUrlFormat == "" {
		config.PingUrlFormat = defaultPingURLFormat
	}
	if config.PingTimeout <= 0 {
		config.PingTimeout = defaultPingTimeout
	}
	if config.WindowSize <= 0 {
		config.WindowSize = defaultWindowSize
	}
}

func (receiver *pingHostAvailabler) ScoreHosts(hosts []string) []*HostAvailabilityScore {
	logs.Debug("[ByteplusSDK] do score hosts:%v", hosts)
	result := make([]*HostAvailabilityScore, len(hosts))
	if len(hosts) == 1 {
		result[0] = &HostAvailabilityScore{Host: hosts[0], Score: 0.0}
		return result
	}
	for _, host := range hosts {
		window, exist := receiver.hostWindowMap[host]
		if !exist {
			window = newWindow(receiver.config.WindowSize)
			receiver.hostWindowMap[host] = window
		}
		window.put(receiver.doPing(host))
	}
	for i, host := range hosts {
		score := 1 - receiver.hostWindowMap[host].failureRate()
		result[i] = &HostAvailabilityScore{host, score}
	}
	return result
}

func (receiver *pingHostAvailabler) doPing(host string) bool {
	request := fasthttp.AcquireRequest()
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	url := fmt.Sprintf(receiver.config.PingUrlFormat, host)
	request.SetRequestURI(url)
	request.Header.SetMethod(fasthttp.MethodGet)
	start := time.Now()
	err := receiver.httpCli.DoTimeout(request, response, receiver.config.PingTimeout)
	cost := time.Now().Sub(start)
	if err != nil {
		logs.Warn("[ByteplusSDK] ping find err, host:%s cost:%s err:%s",
			host, cost, err.Error())
		return false
	}
	if receiver.isPingSuccess(response) {
		logs.Debug("[ByteplusSDK]ping success host:%s cost:%s", host, cost)
		return true
	}
	logs.Warn("[ByteplusSDK] ping fail, host:%s cost:%s status:%d", host, cost, response.StatusCode())
	return false
}

func (receiver *pingHostAvailabler) isPingSuccess(httpRsp *fasthttp.Response) bool {
	if httpRsp.StatusCode() != fasthttp.StatusOK {
		return false
	}
	rspBodyBytes := httpRsp.Body()
	if len(rspBodyBytes) == 0 {
		return false
	}
	rspStr := string(rspBodyBytes)
	return len(rspStr) < 20 && strings.Contains(rspStr, "pong")
}

func newWindow(size int) *window {
	result := &window{
		size:         size,
		items:        make([]bool, size),
		head:         size - 1,
		tail:         0,
		failureCount: 0,
	}
	for i := range result.items {
		result.items[i] = true
	}
	return result
}

type window struct {
	size         int
	items        []bool
	head         int
	tail         int
	failureCount float64
}

func (receiver *window) put(success bool) {
	if !success {
		receiver.failureCount++
	}
	receiver.head = (receiver.head + 1) % receiver.size
	receiver.items[receiver.head] = success
	receiver.tail = (receiver.tail + 1) % receiver.size
	removingItem := receiver.items[receiver.tail]
	if !removingItem {
		receiver.failureCount--
	}
}

func (receiver *window) failureRate() float64 {
	return receiver.failureCount / float64(receiver.size)
}

func (receiver *window) String() string {
	return fmt.Sprintf("%+v", *receiver)
}
