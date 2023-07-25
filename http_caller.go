package core

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/metrics"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/option"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"
)

const (
	netErrMark                     = "[netErr]"
	defaultTimeout                 = 5 * time.Second
	defaultHTTPCallerPingURLFormat = "%s://%s/predict/api/ping"
	defaultHTTPCallerPingTimeout   = 500 * time.Millisecond
)

type CallerConfig struct {
	KeepAliveDuration     time.Duration
	KeepAlivePingInterval time.Duration
	MaxConnections        int
	MaxConnWaitTimeout    time.Duration
}

func fillDefaultCallerConfig(callerConfig *CallerConfig) *CallerConfig {
	if callerConfig.KeepAliveDuration <= 0 {
		callerConfig.KeepAliveDuration = defaultKeepAliveDuration
	}
	if callerConfig.KeepAlivePingInterval <= 0 {
		callerConfig.KeepAlivePingInterval = defaultKeepAlivePingInterval
	}
	if callerConfig.MaxConnections <= 0 {
		callerConfig.MaxConnections = fasthttp.DefaultMaxConnsPerHost
	}
	return callerConfig
}

type httpCaller struct {
	projectID      string
	tenantID       string
	useAirAuth     bool
	airAuthToken   string
	credentials    credential
	hostAvailabler HostAvailabler
	config         *CallerConfig
	schema         string
	keepAlive      bool
	httpCli        *fasthttp.Client
	stop           chan bool
}

func newHTTPCaller(projectID, tenantID string, useAirAuth bool, airAuthToken string,
	credentials credential, hostAvailabler HostAvailabler, config *CallerConfig,
	schema string, keepAlive bool) *httpCaller {
	config = fillDefaultCallerConfig(config)
	mHTTPCaller := &httpCaller{
		projectID:      projectID,
		tenantID:       tenantID,
		useAirAuth:     useAirAuth,
		airAuthToken:   airAuthToken,
		credentials:    credentials,
		hostAvailabler: hostAvailabler,
		config:         config,
		schema:         schema,
		keepAlive:      keepAlive,
		httpCli: &fasthttp.Client{
			MaxIdleConnDuration: config.KeepAliveDuration,
			MaxConnsPerHost:     config.MaxConnections,
			MaxConnWaitTimeout:  config.MaxConnWaitTimeout,
		},
	}
	if keepAlive {
		mHTTPCaller.initHeartbeatExecutor()
	}
	return mHTTPCaller
}

func (c *httpCaller) initHeartbeatExecutor() {
	AsyncExecute(func() {
		ticker := time.NewTicker(c.config.KeepAlivePingInterval)
		for {
			select {
			case <-c.stop:
				ticker.Stop()
				return
			case <-ticker.C:
				c.heartbeat()
			}
		}
	})
}

func (c *httpCaller) heartbeat() {
	for _, host := range c.hostAvailabler.GetHosts() {
		metricsTags := []string{
			"from:http_caller",
			"project_id:" + c.projectID,
			"host:" + escapeMetricsTagValue(host),
		}
		metrics.Counter(metricsKeyHeartbeatCount, 1, metricsTags...)
		Ping(c.projectID, c.httpCli, defaultHTTPCallerPingURLFormat, c.schema, host, defaultHTTPCallerPingTimeout)
	}
}

func (c *httpCaller) doJSONRequest(url string, request interface{},
	response interface{}, options *option.Options) error {
	reqBytes, err := json.Marshal(request)
	headers := c.buildHeaders(options, "application/json")
	reqID := headers["Request-Id"]
	if err != nil {
		metricsTags := []string{
			"type:marshal_json_request_fail",
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(reqID, "[ByteplusSDK] marshal json request fail, project_id:%s, url:%s err:%v",
			c.projectID, url, err)
		logs.Error("json marshal request fail, err:%v url:%s", err, url)
		return err
	}
	url = c.withOptionQueries(options, url)
	rspBytes, err := c.doHTTPRequest(reqID, url, headers, reqBytes, options.Timeout)
	if err != nil {
		return err
	}
	err = json.Unmarshal(rspBytes, &response)
	if err != nil {
		metricsTags := []string{
			"type:unmarshal_json_response_fail",
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(reqID, "[ByteplusSDK] unmarshal json response fail, project_id:%s, url:%s err:%v",
			c.projectID, url, err)
		logs.Error("unmarshal response fail, err:%v url:%s", err, url)
		return err
	}
	return nil
}

func (c *httpCaller) doPBRequest(url string, request proto.Message,
	response proto.Message, options *option.Options) error {
	reqBytes, err := proto.Marshal(request)
	headers := c.buildHeaders(options, "application/x-protobuf")
	reqID := headers["Request-Id"]
	if err != nil {
		metricsTags := []string{
			"type:marshal_pb_request_fail",
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(reqID, "[ByteplusSDK] marshal pb request fail, project_id:%s, url:%s err:%v",
			c.projectID, url, err)
		logs.Error("marshal request fail, err:%v url:%s", err, url)
		return err
	}
	url = c.withOptionQueries(options, url)
	rspBytes, err := c.doHTTPRequest(reqID, url, headers, reqBytes, options.Timeout)
	if err != nil {
		return err
	}
	err = proto.Unmarshal(rspBytes, response)
	if err != nil {
		metricsTags := []string{
			"type:unmarshal_pb_response_fail",
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(reqID, "[ByteplusSDK] unmarshal pb response fail, project_id:%s, url:%s err:%v",
			c.projectID, url, err)
		logs.Error("unmarshal response fail, err:%v url:%s", err, url)
		return err
	}
	return nil
}

func (c *httpCaller) buildHeaders(options *option.Options, contentType string) map[string]string {
	headers := make(map[string]string)
	headers["Content-Encoding"] = "gzip"
	headers["Accept-Encoding"] = "gzip"
	headers["Content-Type"] = contentType
	headers["Accept"] = contentType
	headers["Tenant-Id"] = c.tenantID
	headers["Project-Id"] = c.projectID
	c.withOptionHeaders(headers, options)
	return headers
}

func (c *httpCaller) withOptionHeaders(headers map[string]string, options *option.Options) {
	if len(options.RequestID) == 0 {
		requestID := uuid.NewString()
		logs.Info("requestID is generated by sdk: '%s' ", requestID)
		headers["Request-Id"] = requestID
	} else {
		headers["Request-Id"] = options.RequestID
	}
	if options.ServerTimeout > 0 {
		headers["Timeout-Millis"] = strconv.Itoa(int(options.ServerTimeout.Milliseconds()))
	}
	for k, v := range options.Headers {
		headers[k] = v
	}
}

func (c *httpCaller) withAuthHeaders(req *fasthttp.Request, reqBytes []byte) {
	if c.useAirAuth {
		c.withAirAuthHeaders(req, reqBytes)
		return
	}
	sign(req, c.credentials)
}

func (c *httpCaller) withAirAuthHeaders(req *fasthttp.Request, reqBytes []byte) {
	var (
		// Gets the second-level timestamp of the current time.
		// The server only supports the second-level timestamp.
		// The 'ts' must be the current time.
		// When current time exceeds a certain time, such as 5 seconds, of 'ts',
		// the signature will be invalid and cannot pass authentication
		ts = strconv.FormatInt(time.Now().Unix(), 10)
		// Use sub string of UUID as "nonce",  too long will be wasted.
		// You can also use 'ts' as' nonce'
		nonce = uuid.NewString()[:8]
		// calculate the authentication signature
		signature = c.calSignature(reqBytes, ts, nonce)
	)
	req.Header.Set("Tenant-Ts", ts)
	req.Header.Set("Tenant-Nonce", nonce)
	req.Header.Set("Tenant-Signature", signature)
}

func (c *httpCaller) calSignature(reqBytes []byte, ts, nonce string) string {
	var (
		token    = c.airAuthToken
		tenantID = c.tenantID
	)
	// Splice in the order of "token", "HTTPBody", "tenant_id", "ts", and "nonce".
	// The order must not be mistaken.
	// String need to be encoded as byte arrays by UTF-8
	shaHash := sha256.New()
	shaHash.Write([]byte(token))
	shaHash.Write(reqBytes)
	shaHash.Write([]byte(tenantID))
	shaHash.Write([]byte(ts))
	shaHash.Write([]byte(nonce))
	return fmt.Sprintf("%x", shaHash.Sum(nil))
}

func (c *httpCaller) withOptionQueries(options *option.Options, url string) string {
	var queriesParts []string
	for name, value := range options.Queries {
		queriesParts = append(queriesParts, name+"="+value)
	}
	optionQuery := strings.Join(queriesParts, "&")
	if optionQuery == "" {
		return url
	}
	if strings.Contains(url, "?") {
		url = url + "&" + optionQuery
	} else {
		url = url + "?" + optionQuery
	}
	return url
}

func (c *httpCaller) doHTTPRequest(reqID, url string, headers map[string]string,
	reqBytes []byte, timeout time.Duration) ([]byte, error) {
	reqBytes = fasthttp.AppendGzipBytes(nil, reqBytes)

	request := c.acquireRequest(url, headers, reqBytes)
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	c.withAuthHeaders(request, reqBytes)
	start := time.Now()
	logs.Trace("http request header:\n%s", &request.Header)
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	err := c.httpCli.DoTimeout(request, response, timeout)
	cost := time.Now().Sub(start)
	defer func() {
		metricsTags := []string{
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Timer(metricsKeyRequestTotalCost, cost.Milliseconds(), metricsTags...)
		metrics.Counter(metricsKeyRequestCount, 1, metricsTags...)
		metrics.Info(reqID, "[ByteplusSDK] http request success project_id:%s, http url:%s, cost:%dms",
			c.projectID, url, cost.Milliseconds())
		logs.Debug("http url:%s, cost:%dms", url, cost.Milliseconds())
	}()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			metricsTags := []string{
				"type:request_timeout",
				"project_id:" + c.projectID,
				"url:" + escapeMetricsTagValue(url),
			}
			metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
			metrics.Error(reqID, "[ByteplusSDK] do http request timeout, project_id:%s, url:%s, cost:%dms, err:%v",
				c.projectID, url, cost.Milliseconds(), err)
			logs.Error("do http request timeout, err:%v url:%s cost:%s", err, url, cost)
			return nil, errors.New(netErrMark + " timeout")
		}
		metricsTags := []string{
			"type:request_occur_err",
			"project_id:" + c.projectID,
			"url:" + escapeMetricsTagValue(url),
		}
		metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
		metrics.Error(reqID, "[ByteplusSDK] do http request occur err, project_id:%s, url:%s, err:%v",
			c.projectID, url, err)
		logs.Error("do http request occur error, err:%v url:%s", err, url)
		return nil, err
	}
	logs.Trace("http response url:%s headers:\n%s", url, &response.Header)
	if response.StatusCode() != fasthttp.StatusOK {
		c.logFailureStatus(reqID, url, response)
		return nil, errors.New(netErrMark + "http status not 200")
	}
	return decompressResponse(url, response)
}

func (c *httpCaller) acquireRequest(url string,
	headers map[string]string, reqBytes []byte) *fasthttp.Request {
	request := fasthttp.AcquireRequest()
	request.Header.SetMethod(fasthttp.MethodPost)
	request.SetRequestURI(url)
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	request.SetBodyRaw(reqBytes)
	return request
}

func (c *httpCaller) logFailureStatus(reqID, url string, response *fasthttp.Response) {
	metricsTags := []string{
		"type:rsp_status_not_ok",
		"project_id:" + c.projectID,
		"url:" + escapeMetricsTagValue(url),
		"status:" + strconv.Itoa(response.StatusCode()),
	}
	metrics.Counter(metricsKeyCommonError, 1, metricsTags...)
	rspBytes, _ := decompressResponse(url, response)
	if len(rspBytes) > 0 {
		logFormat := "[ByteplusSDK] http status not 200, project_id:%s, url:%s, code:%d, headers:\n%s, body:\n%s"
		metrics.Error(reqID, logFormat,
			c.projectID, url, response.StatusCode(), &response.Header, string(rspBytes))
		logs.Error("http status not 200, url:%s code:%d headers:\n%s body:\n%s",
			url, response.StatusCode(), &response.Header, string(rspBytes))
		return
	}
	metrics.Error(reqID, "[ByteplusSDK] http status not 200, project_id:%s, url:%s, code:%d, headers:\\n%s",
		c.projectID, url, response.StatusCode(), &response.Header)
	logs.Error("http status not 200, url:%s code:%d headers:\n%s\n",
		url, response.StatusCode(), &response.Header)
}

func decompressResponse(url string, response *fasthttp.Response) ([]byte, error) {
	contentEncoding := strings.ToLower(strings.TrimSpace(string(response.Header.Peek("Content-Encoding"))))
	switch contentEncoding {
	case "gzip":
		respBodyBytes, err := response.BodyGunzip()
		if err != nil {
			logs.Error("decompress gzip resp occur error, msg:%v url:%s header:\n%s",
				err, url, &response.Header)
			return nil, err
		}
		return respBodyBytes, nil
	case "":
		return response.Body(), nil
	default:
		logs.Error("receive unsupported response content encoding:%s url:%s header:\n%s",
			contentEncoding, url, &response.Header)
		err := errors.New("unsupported resp content encoding:" + contentEncoding)
		return nil, err
	}
}

func (c *httpCaller) shutdown() {
	if c.stop != nil {
		close(c.stop)
	}
}
