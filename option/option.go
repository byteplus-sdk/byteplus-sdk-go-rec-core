package option

import (
	"time"
)

func Conv2Options(opts ...Option) *Options {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

type Option func(opts *Options)

// WithRequestID Specify the request_id manually. By default,
// the SDK generates a unique request_id for each request using the UUID
func WithRequestID(requestID string) Option {
	return func(options *Options) {
		options.RequestID = requestID
	}
}

// WithTimeout Specifies the timeout for this request
func WithTimeout(timeout time.Duration) Option {
	return func(options *Options) {
		options.Timeout = timeout
	}
}

// WithServerTimeout Specifies the maximum time it will take for
// the server to process the request. The server will try to return
// the result within this time, even if the task is not completed
func WithServerTimeout(timeout time.Duration) Option {
	return func(options *Options) {
		options.ServerTimeout = timeout
	}
}

// WithHTTPHeader Add an HTTP header to the request.
// In general, you do not need to care this.
func WithHTTPHeader(key, value string) Option {
	return func(options *Options) {
		if options.Headers == nil {
			options.Headers = make(map[string]string)
		}
		options.Headers[key] = value
	}
}

// WithHTTPQuery Add an HTTP query to the request.
// In general, you do not need to care this.
func WithHTTPQuery(key, value string) Option {
	return func(options *Options) {
		if options.Queries == nil {
			options.Queries = make(map[string]string)
		}
		options.Queries[key] = value
	}
}
