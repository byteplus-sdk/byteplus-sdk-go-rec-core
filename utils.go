package core

import (
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"time"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/metrics"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/logs"
)

func AsyncExecute(runnable func()) {
	go func(run func()) {
		defer func() {
			if r := recover(); r != nil {
				logs.Error("async execute occur panic, "+
					"please feedback to bytedance, err:%v trace:\n%s", r, string(debug.Stack()))
			}
		}()
		run()
	}(runnable)
}

func DoWithRetry(maxRetryTimes int, runnable func() error) error {
	tryTimes := int(math.Max(0, float64(maxRetryTimes))) + 1
	var err = errors.New("")
	for i := 0; err != nil && i < tryTimes; i++ {
		err = runnable()
	}
	return err
}

func IsNetError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), netErrMark)
}

func IsTimeoutError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func buildURL(schema, host, path string) string {
	if strings.HasPrefix(path, "/") {
		return fmt.Sprintf("%s://%s%s", schema, host, path)
	}
	return fmt.Sprintf("%s://%s/%s", schema, host, path)
}

func Ping(projectID string, httpCli *fasthttp.Client, pingURLFormat,
	schema, host string, pingTimeout time.Duration) bool {
	request := fasthttp.AcquireRequest()
	response := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(request)
		fasthttp.ReleaseResponse(response)
	}()
	url := fmt.Sprintf(pingURLFormat, schema, host)
	request.SetRequestURI(url)
	request.Header.SetMethod(fasthttp.MethodGet)
	reqID := "ping_" + uuid.NewString()
	request.Header.Set("Request-Id", reqID)
	request.Header.Set("Project-Id", projectID)
	start := time.Now()
	err := httpCli.DoTimeout(request, response, pingTimeout)
	cost := time.Since(start)
	if err != nil {
		metrics.Warn(reqID, "[ByteplusSDK] ping find err, project_id:%s, host:%s, cost:%dms, err:%v",
			projectID, host, cost.Milliseconds(), err)
		logs.Warn("ping find err, host:%s cost:%dms err:%v", host, cost.Milliseconds(), err)
		return false
	}
	if IsPingSuccess(response) {
		metrics.Info(reqID, "[ByteplusSDK] ping success, project_id:%s, host:%s, cost:%dms",
			projectID, host, cost.Milliseconds())
		logs.Debug("ping success host:%s cost:%dms", host, cost.Milliseconds())
		return true
	}
	metrics.Warn(reqID, "[ByteplusSDK] ping fail, project_id:%s, host:%s, cost:%dms, status:%d",
		projectID, host, cost.Milliseconds(), response.StatusCode())
	logs.Warn("ping fail, host:%s cost:%dms status:%d", host, cost.Milliseconds(), response.StatusCode())
	return false
}

func IsPingSuccess(httpRsp *fasthttp.Response) bool {
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

func escapeMetricsTagValue(value string) string {
	value = strings.ReplaceAll(value, "?", "-qu-")
	value = strings.ReplaceAll(value, "&", "-and-")
	value = strings.ReplaceAll(value, "=", "-eq-")
	return value
}
