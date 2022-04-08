package option

import "time"

type Options struct {
	Timeout       time.Duration
	RequestID     string
	Headers       map[string]string
	Queries       map[string]string
	ServerTimeout time.Duration
}
