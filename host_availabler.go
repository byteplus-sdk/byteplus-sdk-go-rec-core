package core

import (
	"fmt"
	"sort"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/valyala/fasthttp"
)

type HostAvailabler interface {
	GetAvailableHosts() []string
	Hosts() []string
	HostHeader() string
	SetHosts(hosts []string)
	SetHostHeader(hostHeader string)
	GetHost() string
	Shutdown()
}

const (
	defaultPingURLFormat        = "http://%s/predict/api/ping"
	defaultPingInterval         = time.Second
	defaultWindowSize           = 60
	defaultFailureRateThreshold = 0.1
	defaultPingTimeout          = 200 * time.Millisecond
)

type PingHostAvailablerConfig struct {
	// host availabler used to test the latency, example {}://%s/predict/api/ping
	// {} will be replaced by schema which set in context
	// %s will be dynamically formatted by hosts
	PingUrlFormat string
	// host availabler test host time interval
	PingInterval time.Duration
	// record the window size of each host's test status
	WindowSize int
	// when the failure rate of a host window exceeds the threshold, the host will not be set
	FailureRateThreshold float64
	// timeout for requesting hosts
	PingTimeout time.Duration

	Hosts []string

	HostHeader string
}

func NewPingHostAvailabler(config *PingHostAvailablerConfig) HostAvailabler {
	availabler := &pingHostAvailabler{
		availableHosts: config.Hosts,
		config:         config,
	}
	if len(config.Hosts) <= 1 {
		return availabler
	}
	hostWindowMap := make(map[string]*window, len(config.Hosts))
	hostHttpCliMap := make(map[string]*fasthttp.HostClient, len(config.Hosts))
	for _, host := range config.Hosts {
		hostWindowMap[host] = newWindow(config.WindowSize)
		hostHttpCliMap[host] = &fasthttp.HostClient{Addr: host}
	}
	availabler.hostWindowMap = hostWindowMap
	availabler.hostHttpCliMap = hostHttpCliMap
	AsyncExecute(availabler.scheduleFunc())
	return availabler
}

type pingHostAvailabler struct {
	config         *PingHostAvailablerConfig
	abort          bool
	currentHost    string
	availableHosts []string
	hostWindowMap  map[string]*window
	hostHttpCliMap map[string]*fasthttp.HostClient
}

func (receiver *pingHostAvailabler) scheduleFunc() func() {
	return func() {
		ticker := time.NewTicker(receiver.config.PingInterval)
		for true {
			if receiver.abort {
				ticker.Stop()
				return
			}
			receiver.checkHost()
			<-ticker.C
		}
	}
}

func (receiver *pingHostAvailabler) checkHost() {
	availableHosts := make([]string, 0, len(receiver.config.Hosts))
	for _, host := range receiver.config.Hosts {
		winObj := receiver.hostWindowMap[host]
		winObj.put(receiver.ping(host))
		if winObj.failureRate() < receiver.config.FailureRateThreshold {
			availableHosts = append(availableHosts, host)
		}
	}
	receiver.availableHosts = availableHosts
	// Make sure that at least have host returns
	if len(availableHosts) < 1 {
		receiver.availableHosts = receiver.config.Hosts
		return
	}
	if len(availableHosts) == 1 {
		return
	}
	sort.Slice(availableHosts, func(i, j int) bool {
		failureRateI := receiver.hostWindowMap[availableHosts[i]].failureRate()
		failureRateJ := receiver.hostWindowMap[availableHosts[j]].failureRate()
		return failureRateI < failureRateJ
	})
}

func (receiver *pingHostAvailabler) ping(host string) bool {
	start := time.Now()
	request := fasthttp.AcquireRequest()
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	url := fmt.Sprintf(receiver.config.PingUrlFormat, host)
	request.SetRequestURI(url)
	request.Header.SetMethod(fasthttp.MethodGet)
	if len(receiver.config.HostHeader) > 0 {
		request.SetHost(receiver.config.HostHeader)
	}
	httpCli := receiver.hostHttpCliMap[host]
	err := httpCli.DoTimeout(request, response, receiver.config.PingTimeout)
	cost := time.Now().Sub(start)
	if err == nil && response.StatusCode() == fasthttp.StatusOK {
		logs.Trace("ping success host:'%s' cost:'%s'", host, cost)
		return true
	}
	logs.Warn("ping fail, host:%s cost:%s status:%d err:%v",
		host, cost, response.StatusCode(), err)
	return false
}

func (receiver *pingHostAvailabler) GetAvailableHosts() []string {
	return receiver.availableHosts
}

func (receiver *pingHostAvailabler) GetHost() string {
	return receiver.availableHosts[0]
}

func (receiver *pingHostAvailabler) Hosts() []string {
	return receiver.config.Hosts
}

func (receiver *pingHostAvailabler) HostHeader() string {
	return receiver.config.HostHeader
}

func (receiver *pingHostAvailabler) SetHosts(hosts []string) {
	receiver.config.Hosts = hosts
}

func (receiver *pingHostAvailabler) SetHostHeader(hostHeader string) {
	receiver.config.HostHeader = hostHeader
}

func (receiver *pingHostAvailabler) Shutdown() {
	receiver.abort = true
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
