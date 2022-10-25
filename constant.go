package core

import "time"

const (
	// All requests will have a XXXResponse corresponding to them,
	// and a‘ll XXXResponses will contain a 'Status' field.
	// The status of this request can be determined by the value of `Status.Code`
	// Detail error code info：https://docs.byteplus.com/docs/error-code

	// StatusCodeSuccess The request was executed successfully without any exception
	StatusCodeSuccess = 0

	// StatusCodeIdempotent A Request with the same "Request-ID" was already received. This Request was rejected
	StatusCodeIdempotent = 409

	// StatusCodeOperationLoss Operation information is missing due to an unknown exception
	StatusCodeOperationLoss = 410

	// StatusCodeTooManyRequest The server hope slow down request frequency, and this request was rejected
	StatusCodeTooManyRequest = 429
)

const (
	// The default keepalive duration
	defaultKeepAliveDuration     = 60 * time.Second
	defaultKeepAlivePingInterval = 45 * time.Second
)

const (
	// Metrics Key
	metricsKeyCommonInfo       = "common.info"
	metricsKeyCommonWarn       = "common.warn"
	metricsKeyCommonError      = "common.err"
	metricsKeyRequestTotalCost = "request.total.cost"
	metricsKeyRequestCount     = "request.count"
	metricsKeyHeartbeatCount   = "heartbeat.count"
)
