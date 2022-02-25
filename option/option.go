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

func WithRequestID(requestID string) Option {
	return func(options *Options) {
		options.RequestID = requestID
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(options *Options) {
		options.Timeout = timeout
	}
}

func WithHeaders(headers map[string]string) Option {
	return func(options *Options) {
		options.Headers = headers
	}
}

func WithHeader(key, value string) Option {
	return func(options *Options) {
		if options.Headers == nil {
			options.Headers = make(map[string]string)
		}
		options.Headers[key] = value
	}
}

func WithServerTimeout(timeout time.Duration) Option {
	return func(options *Options) {
		options.ServerTimeout = timeout
	}
}

func WithQueries(queries map[string]string) Option {
	return func(options *Options) {
		options.Queries = queries
	}
}

func WithQuery(key, value string) Option {
	return func(options *Options) {
		if options.Queries == nil {
			options.Queries = make(map[string]string)
		}
		options.Queries[key] = value
	}
}
