package core

import (
	"errors"
	"sync"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/metrics"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/option"
	"google.golang.org/protobuf/proto"
)

type HTTPClient struct {
	cli            *httpCaller
	hostAvailabler HostAvailabler
	schema         string
	projectID      string
}

func (h *HTTPClient) DoJSONRequest(path string, request interface{},
	response proto.Message, options *option.Options) error {
	url := buildURL(h.schema, h.hostAvailabler.GetHost(path), path)
	return h.cli.doJSONRequest(url, request, response, options)
}

func (h *HTTPClient) DoPBRequest(path string, request proto.Message,
	response proto.Message, options *option.Options) error {
	url := buildURL(h.schema, h.hostAvailabler.GetHost(path), path)
	return h.cli.doPBRequest(url, request, response, options)
}

func (h *HTTPClient) Shutdown() {
	h.hostAvailabler.Shutdown()
	h.cli.shutdown()
}

type httpClientBuilder struct {
	tenantID              string
	projectID             string
	useAirAuth            bool
	airAuthToken          string
	authAK                string
	authSK                string
	authService           string
	schema                string
	mainHost              string
	hosts                 []string
	region                IRegion
	keepAlive             bool
	hostAvailablerFactory HostAvailablerFactory
	callerConfig          *CallerConfig
	hostAvailabler        HostAvailabler
	metricsCfg            *metrics.Config
}

func NewHTTPClientBuilder() *httpClientBuilder {
	return &httpClientBuilder{}
}

func (receiver *httpClientBuilder) TenantID(tenantID string) *httpClientBuilder {
	receiver.tenantID = tenantID
	return receiver
}

func (receiver *httpClientBuilder) ProjectID(projectID string) *httpClientBuilder {
	receiver.projectID = projectID
	return receiver
}

func (receiver *httpClientBuilder) AirAuthToken(airAuthToken string) *httpClientBuilder {
	receiver.airAuthToken = airAuthToken
	return receiver
}

func (receiver *httpClientBuilder) AuthAK(authAK string) *httpClientBuilder {
	receiver.authAK = authAK
	return receiver
}

func (receiver *httpClientBuilder) AuthSK(authSK string) *httpClientBuilder {
	receiver.authSK = authSK
	return receiver
}

func (receiver *httpClientBuilder) AuthService(authService string) *httpClientBuilder {
	receiver.authService = authService
	return receiver
}

func (receiver *httpClientBuilder) UseAirAuth(useAirAuth bool) *httpClientBuilder {
	receiver.useAirAuth = useAirAuth
	return receiver
}

func (receiver *httpClientBuilder) Schema(schema string) *httpClientBuilder {
	receiver.schema = schema
	return receiver
}

func (receiver *httpClientBuilder) Hosts(hosts []string) *httpClientBuilder {
	receiver.hosts = hosts
	return receiver
}

func (receiver *httpClientBuilder) MainHost(host string) *httpClientBuilder {
	receiver.mainHost = host
	return receiver
}

func (receiver *httpClientBuilder) Region(region IRegion) *httpClientBuilder {
	receiver.region = region
	return receiver
}

func (receiver *httpClientBuilder) HostAvailablerFactory(
	hostAvailablerFactory HostAvailablerFactory) *httpClientBuilder {
	receiver.hostAvailablerFactory = hostAvailablerFactory
	return receiver
}

func (receiver *httpClientBuilder) KeepAlive(keepAlive bool) *httpClientBuilder {
	receiver.keepAlive = keepAlive
	return receiver
}

func (receiver *httpClientBuilder) CallerConfig(callerConfig *CallerConfig) *httpClientBuilder {
	receiver.callerConfig = callerConfig
	return receiver
}

func (receiver *httpClientBuilder) HostAvailabler(hostAvailabler HostAvailabler) *httpClientBuilder {
	receiver.hostAvailabler = hostAvailabler
	return receiver
}

func (receiver *httpClientBuilder) MetricsCfg(metricsConfig *metrics.Config) *httpClientBuilder {
	receiver.metricsCfg = metricsConfig
	return receiver
}

var (
	globalHostAvailablerLock                = &sync.Mutex{}
	globalHostAvailabler     HostAvailabler = nil
)

func (receiver *httpClientBuilder) Build() (*HTTPClient, error) {
	err := receiver.checkRequiredField()
	if err != nil {
		return nil, err
	}
	receiver.fillDefault()
	if !metrics.Collector.IsInitialed() && receiver.metricsCfg != nil {
		if receiver.metricsCfg.EnableMetrics || receiver.metricsCfg.EnableMetricsLog {
			receiver.initGlobalHostAvailabler()
		}
	}
	metrics.Collector.Init(receiver.metricsCfg, globalHostAvailabler)
	return &HTTPClient{
		cli:            receiver.newHTTPCaller(),
		hostAvailabler: receiver.hostAvailabler,
		schema:         receiver.schema,
		projectID:      receiver.projectID,
	}, nil
}

func (receiver *httpClientBuilder) checkRequiredField() error {
	if receiver.tenantID == "" {
		return errors.New("tenant id is null")
	}
	if err := receiver.checkAuthRequiredField(); err != nil {
		return err
	}
	if receiver.region == nil {
		return errors.New("region is null")
	}
	return nil
}

func (receiver *httpClientBuilder) checkAuthRequiredField() error {
	if receiver.useAirAuth && receiver.airAuthToken == "" {
		return errors.New("token cannot be null")
	}
	if !receiver.useAirAuth && (receiver.authAK == "" || receiver.authSK == "") {
		return errors.New("ak and sk cannot be null")
	}
	return nil
}

func (receiver *httpClientBuilder) fillDefault() {
	if receiver.schema == "" {
		receiver.schema = "https"
	}
	// fill hostAvailabler.
	if receiver.hostAvailablerFactory == nil {
		receiver.hostAvailablerFactory = &HostAvailablerFactoryBase{}
	}
	receiver.hostAvailabler, _ = receiver.newHostAvailabler()

	// fill default caller config.
	if receiver.callerConfig == nil {
		receiver.callerConfig = fillDefaultCallerConfig(&CallerConfig{})
	}
}

func (receiver *httpClientBuilder) newHostAvailabler() (HostAvailabler, error) {
	// if '.hosts' is set, then skip fetch hosts from server
	if len(receiver.hosts) > 0 {
		return receiver.hostAvailablerFactory.NewHostAvailabler(receiver.projectID, receiver.hosts, receiver.mainHost, true)
	}
	return receiver.hostAvailablerFactory.NewHostAvailabler(receiver.projectID, receiver.region.GetHosts(), receiver.mainHost, false)
}

func (receiver *httpClientBuilder) initGlobalHostAvailabler() {
	globalHostAvailablerLock.Lock()
	defer globalHostAvailablerLock.Unlock()
	if globalHostAvailabler != nil {
		return
	}
	globalHostAvailabler, _ = receiver.newHostAvailabler()
}

func (receiver *httpClientBuilder) newHTTPCaller() *httpCaller {
	authRegion := receiver.region.GetAuthRegion()
	cred := credential{
		accessKeyID:     receiver.authAK,
		secretAccessKey: receiver.authSK,
		service:         receiver.authService,
		region:          authRegion,
	}
	mHTTPCaller := newHTTPCaller(
		receiver.projectID,
		receiver.tenantID,
		receiver.useAirAuth,
		receiver.airAuthToken,
		cred,
		receiver.hostAvailabler,
		receiver.callerConfig,
		receiver.schema,
		receiver.keepAlive,
	)
	return mHTTPCaller
}
