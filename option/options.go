package option

import "time"

type Options struct {
	Timeout       time.Duration
	RequestId     string
	Headers       map[string]string
	ServerTimeout time.Duration
	Queries       map[string]string
}
