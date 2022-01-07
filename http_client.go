package core

import (
	"errors"
	"fmt"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/option"
	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"
)

type HTTPClient struct {
	cli            *httpCaller
	hostAvailabler HostAvailabler
	schema         string
}

func (h *HTTPClient) DoJsonRequest(path string, request interface{},
	response proto.Message, options *option.Options) error {
	host := h.hostAvailabler.GetHost()
	url := urlCenterInstance(h.schema, host).getURL(path)
	return h.cli.doJsonRequest(url, request, response, options)
}

func (h *HTTPClient) DoPbRequest(path string, request proto.Message,
	response proto.Message, options *option.Options) error {
	host := h.hostAvailabler.GetHost()
	url := urlCenterInstance(h.schema, host).getURL(path)
	return h.cli.doPbRequest(url, request, response, options)
}

func (h *HTTPClient) Shutdown() {
	h.hostAvailabler.Shutdown()
}

type httpClientBuilder struct {
	tenantID       string
	token          string
	ak             string
	sk             string
	authService    string
	useAirAuth     bool
	schema         string
	hostHeader     string
	hosts          []string
	headers        map[string]string
	region         Region
	hostAvailabler HostAvailabler
}

func NewHTTPClientBuilder() *httpClientBuilder {
	return &httpClientBuilder{}
}

func (receiver *httpClientBuilder) TenantID(tenantID string) *httpClientBuilder {
	receiver.tenantID = tenantID
	return receiver
}

func (receiver *httpClientBuilder) Token(token string) *httpClientBuilder {
	receiver.token = token
	return receiver
}

func (receiver *httpClientBuilder) AK(ak string) *httpClientBuilder {
	receiver.ak = ak
	return receiver
}

func (receiver *httpClientBuilder) SK(sk string) *httpClientBuilder {
	receiver.sk = sk
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

func (receiver *httpClientBuilder) HostHeader(hostHeader string) *httpClientBuilder {
	receiver.hostHeader = hostHeader
	return receiver
}

func (receiver *httpClientBuilder) Hosts(hosts []string) *httpClientBuilder {
	receiver.hosts = hosts
	return receiver
}

func (receiver *httpClientBuilder) Headers(headers map[string]string) *httpClientBuilder {
	receiver.headers = headers
	return receiver
}

func (receiver *httpClientBuilder) Region(region Region) *httpClientBuilder {
	receiver.region = region
	return receiver
}

func (receiver *httpClientBuilder) HostAvailabler(hostAvailabler HostAvailabler) *httpClientBuilder {
	receiver.hostAvailabler = hostAvailabler
	return receiver
}

func (receiver *httpClientBuilder) Build() (*HTTPClient, error) {
	err := receiver.checkRequiredField()
	if err != nil {
		return nil, err
	}
	receiver.fillHosts()
	receiver.fillDefault()
	return &HTTPClient{
		cli:            receiver.newHTTPCaller(),
		hostAvailabler: receiver.hostAvailabler,
		schema:         receiver.schema,
	}, nil
}

func (receiver *httpClientBuilder) checkRequiredField() error {
	if receiver.tenantID == "" {
		return errors.New("tenant id is null")
	}
	if err := receiver.checkAuthRequiredField(); err != nil {
		return err
	}
	if receiver.region == regionUnknown {
		return errors.New("region is null")
	}
	// check if the region is registered
	if getRegionConfig(receiver.region) == nil {
		return errors.New(fmt.Sprintf("region(%s) is not support", receiver.region))
	}
	return nil
}

func (receiver *httpClientBuilder) checkAuthRequiredField() error {
	if receiver.useAirAuth && receiver.token == "" {
		return errors.New("token is null")
	}
	if !receiver.useAirAuth && (receiver.ak == "" || receiver.sk == "") {
		return errors.New("ak or sk is null")
	}
	return nil
}

func (receiver *httpClientBuilder) fillHosts() {
	if len(receiver.hosts) > 0 {
		return
	}
	receiver.hosts = getRegionHosts(receiver.region)
}

func (receiver *httpClientBuilder) fillDefault() {
	if receiver.schema == "" {
		receiver.schema = "https"
	}
	if receiver.hostAvailabler == nil {
		hostAvailablerConfig := &PingHostAvailablerConfig{
			PingUrlFormat:        defaultPingURLFormat,
			PingInterval:         defaultPingInterval,
			PingTimeout:          defaultPingTimeout,
			WindowSize:           defaultWindowSize,
			FailureRateThreshold: defaultFailureRateThreshold,
			Hosts:                receiver.hosts,
			HostHeader:           receiver.hostHeader,
		}
		receiver.hostAvailabler = NewPingHostAvailabler(hostAvailablerConfig)
	}
	if len(receiver.hostAvailabler.Hosts()) == 0 {
		receiver.hostAvailabler.SetHosts(receiver.hosts)
	}
	if receiver.hostAvailabler.HostHeader() == "" {
		receiver.hostAvailabler.SetHostHeader(receiver.hostHeader)
	}
}

func (receiver *httpClientBuilder) newHTTPCaller() *httpCaller {
	cred := credential{
		accessKeyID:     receiver.ak,
		secretAccessKey: receiver.sk,
		service:         receiver.authService,
		region:          getVolcCredentialRegion(receiver.region),
	}
	mHTTPCaller := &httpCaller{
		tenantID:        receiver.tenantID,
		useAirAuth:      receiver.useAirAuth,
		volcCredentials: cred,
		token:           receiver.token,
		hostHeader:      receiver.hostHeader,
	}
	if receiver.hostHeader != "" {
		mHTTPCaller.hostHTTPCli = &fasthttp.HostClient{Addr: receiver.hosts[0]}
	} else {
		mHTTPCaller.defaultHTTPCli = &fasthttp.Client{}
	}
	return mHTTPCaller
}
