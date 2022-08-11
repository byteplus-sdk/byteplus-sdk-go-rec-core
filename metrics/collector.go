package metrics

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	core "github.com/byteplus-sdk/byteplus-sdk-go-rec-core"
	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/metrics/protocol"
	"github.com/valyala/fasthttp"
)

var (
	Collector *collector
)

type Config struct {
	// When metrics are enabled, monitoring metrics will be reported to the byteplus server during use.
	EnableMetrics bool
	// When metrics log is enabled, the log will be reported to the byteplus server during use.
	EnableMetricsLog bool
	// The address of the byteplus metrics service, will be consistent with the host maintained by hostAvailabler.
	Domain string
	// The prefix of the Metrics indicator, the default is byteplus.rec.sdk, do not modify.
	Prefix string
	// Use this httpSchema to report metrics to byteplus server, default is https.
	HTTPSchema string
	// The reporting interval, the default is 15s, if the QPS is high, the reporting interval can be reduced to prevent data loss.
	ReportInterval time.Duration
	// Timeout for request reporting.
	HTTPTimeout time.Duration
}

func NewConfig() *Config {
	return &Config{
		EnableMetrics:    false,
		EnableMetricsLog: false,
		Domain:           defaultMetricsDomain,
		Prefix:           defaultMetricsPrefix,
		HTTPSchema:       defaultMetricsHTTPSchema,
		ReportInterval:   defaultReportInterval,
		HTTPTimeout:      defaultHTTPTimeout,
	}
}

type collector struct {
	cfg                         *Config
	reporter                    *reporter
	metricsCollector            chan *protocol.Metric
	metricsLogCollector         chan *protocol.MetricLog
	cleaningMetricsCollector    bool
	cleaningMetricsLogCollector bool
	initialed                   bool
	hostAvailabler              core.HostAvailabler
	lock                        *sync.Mutex
}

func (c *collector) Init(cfg *Config, hostAvailabler core.HostAvailabler) {
	if c.initialed {
		return
	}
	if cfg == nil {
		cfg = NewConfig()
	}
	c.cfg = cfg
	c.hostAvailabler = hostAvailabler
	c.lock = &sync.Mutex{}
	c.doInit()
}

func (c *collector) doInit() {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.initialed {
		return
	}
	// initialize metrics reporter
	c.reporter = &reporter{
		httpCli:    &fasthttp.Client{},
		metricsCfg: c.cfg,
	}
	// initialize metrics collector
	c.metricsCollector = make(chan *protocol.Metric, maxMetricsSize)
	c.metricsLogCollector = make(chan *protocol.MetricLog, maxMetricsLogSize)
	if !c.isEnableMetrics() && !c.isEnableMetricsLog() {
		return
	}
	c.startReport()
	c.initialed = true
}

func (c *collector) isEnableMetrics() bool {
	if c.cfg == nil {
		return false
	}
	return c.cfg.EnableMetrics
}

func (c *collector) isEnableMetricsLog() bool {
	if c.cfg == nil {
		return false
	}
	return c.cfg.EnableMetricsLog
}

func (c *collector) EmitMetric(metricsType, name string, value int64, tagKvs ...string) {
	if !c.isEnableMetrics() {
		return
	}
	// spin when cleaning collector
	tryTimes := 0
	for c.cleaningMetricsCollector {
		if tryTimes >= maxSpinTimes {
			return
		}
		time.Sleep(5 * time.Millisecond)
		tryTimes += 1
	}
	metricsName := name
	if len(c.cfg.Prefix) > 0 {
		metricsName = fmt.Sprintf("%s.%s", c.cfg.Prefix, metricsName)
	}
	metric := &protocol.Metric{
		Name:      metricsName,
		Value:     float64(value),
		Type:      metricsType,
		Timestamp: currentTimeMillis(),
		Tags:      recoverTags(tagKvs...),
	}
	select {
	case c.metricsCollector <- metric:
	default:
		logs.Debug("[Metrics]: The number of metrics exceeds the limit, the metrics write is rejected")
	}
}

func (c *collector) EmitLog(logID, message, logLevel string, timestamp int64) {
	if !c.isEnableMetricsLog() {
		return
	}
	// spin when cleaning collector
	tryTimes := 0
	for c.cleaningMetricsLogCollector {
		if tryTimes >= maxSpinTimes {
			return
		}
		time.Sleep(5 * time.Millisecond)
		tryTimes += 1
	}
	metricLog := &protocol.MetricLog{
		Id:        logID,
		Message:   message,
		Level:     logLevel,
		Timestamp: currentTimeMillis(),
	}
	select {
	case c.metricsLogCollector <- metricLog:
	default:
		logs.Debug("[Metrics]: The number of metrics logs exceeds the limit, the metrics write is rejected")
	}
}

func (c *collector) startReport() {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logs.Error("metrics report encounter panic:%+v, stack:%s", err, string(debug.Stack()))
			}
		}()
		ticker := time.NewTicker(c.cfg.ReportInterval)
		for range ticker.C {
			c.report()
		}
	}()
}

func (c *collector) report() {
	if c.isEnableMetrics() {
		c.reportMetrics()
	}
	if c.isEnableMetricsLog() {
		c.reportMetricsLog()
	}
}

func (c *collector) reportMetrics() {
	metricsLen := len(c.metricsCollector)
	if metricsLen == 0 {
		return
	}
	metrics := make([]*protocol.Metric, 0, metricsLen)
	c.cleaningMetricsCollector = true
	for i := 0; i < metricsLen; i++ {
		metric := <-c.metricsCollector
		metrics = append(metrics, metric)
	}
	c.cleaningMetricsCollector = false
	c.doReportMetrics(metrics)
}

func (c *collector) getDomain(path string) string {
	if c.hostAvailabler == nil {
		return c.cfg.Domain
	}
	return c.hostAvailabler.GetHost(path)
}

func (c *collector) doReportMetrics(metrics []*protocol.Metric) {
	url := fmt.Sprintf(metricsURLFormat, c.cfg.HTTPSchema, c.getDomain(metricsPath))
	metricMessage := &protocol.MetricMessage{
		Metrics: metrics,
	}
	err := c.reporter.reportMetrics(metricMessage, url)
	if err != nil {
		logs.Error("[Metrics] report metrics fail, err:%v, url:%s", err, url)
	}
}

func (c *collector) reportMetricsLog() {
	metricsLogLen := len(c.metricsLogCollector)
	if metricsLogLen == 0 {
		return
	}
	metricLogs := make([]*protocol.MetricLog, 0, metricsLogLen)
	c.cleaningMetricsLogCollector = true
	for i := 0; i < metricsLogLen; i++ {
		metricLog := <-c.metricsLogCollector
		metricLogs = append(metricLogs, metricLog)
	}
	c.cleaningMetricsLogCollector = false
	c.doReportMetricsLogs(metricLogs)
}

func (c *collector) doReportMetricsLogs(metricLogs []*protocol.MetricLog) {
	url := fmt.Sprintf(metricsLogURLFormat, c.cfg.HTTPSchema, c.getDomain(metricsLogPath))
	metricLogMessage := &protocol.MetricLogMessage{
		MetricLogs: metricLogs,
	}
	err := c.reporter.reportMetricsLog(metricLogMessage, url)
	if err != nil {
		logs.Error("[Metrics] report metrics log fail, err:%v, url:%s", err, url)
	}
}

// recover tagStrings to origin Tags map
func recoverTags(tagKvs ...string) map[string]string {
	tagKvMap := make(map[string]string)
	for _, kv := range tagKvs {
		res := strings.Split(kv, ":")
		if len(res) != 2 {
			continue
		}
		tagKvMap[res[0]] = res[1]
	}
	return tagKvMap
}

func currentTimeMillis() int64 {
	return time.Now().UnixNano() / 1e6
}
