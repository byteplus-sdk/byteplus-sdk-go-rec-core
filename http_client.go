package core

import (
	"errors"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/option"
	"github.com/valyala/fasthttp"
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
}

type httpClientBuilder struct {
	tenantID       string
	projectID      string
	useAirAuth     bool
	airAuthToken   string
	authAK         string
	authSK         string
	authService    string
	schema         string
	hosts          []string
	region         IRegion
	hostAvailabler HostAvailabler
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

func (receiver *httpClientBuilder) Region(region IRegion) *httpClientBuilder {
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
	receiver.fillDefault()
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
	if receiver.hostAvailabler == nil {
		if len(receiver.hosts) > 0 {
			receiver.hostAvailabler, _ = NewPingHostAvailabler(receiver.hosts, "", nil)
		} else {
			receiver.hostAvailabler, _ = NewPingHostAvailabler(
				receiver.region.GetHosts(),
				receiver.projectID,
				nil,
			)
		}
	}
}

func (receiver *httpClientBuilder) newHTTPCaller() *httpCaller {
	authRegion := receiver.region.GetAuthRegion()
	cred := credential{
		accessKeyID:     receiver.authAK,
		secretAccessKey: receiver.authSK,
		service:         receiver.authService,
		region:          authRegion,
	}
	mHTTPCaller := &httpCaller{
		tenantID:     receiver.tenantID,
		useAirAuth:   receiver.useAirAuth,
		credentials:  cred,
		airAuthToken: receiver.airAuthToken,
		httpCli:      &fasthttp.Client{},
	}
	return mHTTPCaller
}
