package core

import (
	"fmt"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/valyala/fasthttp"
)

const (
	defaultPingURLFormat     = "http://%s/predict/api/ping"
	defaultWindowSize        = 60
	defaultPingTimeout       = 300 * time.Millisecond
	defaultPingInterval      = time.Second
	defaultFetchHostInterval = 10 * time.Second
)

type PingHostAvailablerConfig struct {
	// host availabler used to test the latency, example {}://%s/predict/api/ping
	// {} will be replaced by schema which set in context
	// %s will be dynamically formatted by hosts
	PingUrlFormat string
	// record the window size of each host's test status
	WindowSize int
	// timeout for requesting hosts
	PingTimeout time.Duration
	// The time interval for pingHostAvailabler to do ping
	PingInterval time.Duration
	// Frequency of pulling hosts
	FetchHostInterval time.Duration
}

type pingHostAvailabler struct {
	*HostAvailablerBase
	config        *PingHostAvailablerConfig
	hostWindowMap map[string]*window
	httpCli       *fasthttp.Client
}

func NewPingHostAvailabler(hosts []string, projectID string,
	config *PingHostAvailablerConfig) (HostAvailabler, error) {
	hostAvailabler := &pingHostAvailabler{
		config: fillDefaultConfig(config),
		httpCli: &fasthttp.Client{
			MaxIdleConnDuration: defaultKeepAliveDuration,
		},
		hostWindowMap: make(map[string]*window, len(hosts)),
	}
	hostAvailablerBase, err := NewHostAvailablerBase(
		hosts,
		projectID,
		hostAvailabler,
		hostAvailabler.config.FetchHostInterval,
		hostAvailabler.config.PingInterval,
	)
	if err != nil {
		return nil, err
	}
	hostAvailabler.HostAvailablerBase = hostAvailablerBase
	return hostAvailabler, nil
}

func fillDefaultConfig(config *PingHostAvailablerConfig) *PingHostAvailablerConfig {
	if config == nil {
		config = &PingHostAvailablerConfig{}
	}
	if config.PingUrlFormat == "" {
		config.PingUrlFormat = defaultPingURLFormat
	}
	if config.PingTimeout <= 0 {
		config.PingTimeout = defaultPingTimeout
	}
	if config.WindowSize <= 0 {
		config.WindowSize = defaultWindowSize
	}
	if config.PingInterval <= 0 {
		config.PingInterval = defaultPingInterval
	}
	if config.FetchHostInterval <= 0 {
		config.FetchHostInterval = defaultFetchHostInterval
	}
	return config
}

func (receiver *pingHostAvailabler) ScoreHosts(hosts []string) []*HostAvailabilityScore {
	logs.Debug("do score hosts:%v", hosts)
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
		window.put(Ping(receiver.projectID, receiver.httpCli, receiver.config.PingUrlFormat,
			"http", host, receiver.config.PingTimeout))
	}
	for i, host := range hosts {
		score := 1 - receiver.hostWindowMap[host].failureRate()
		result[i] = &HostAvailabilityScore{host, score}
	}
	return result
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
