package core

import (
	"errors"
	"log"
	"math"
	"runtime/debug"
	"strings"
)

func AsyncExecute(runnable func()) {
	go func(run func()) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ByteplusSDK] async execute occur panic, "+
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
