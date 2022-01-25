package core

import (
	"fmt"
	"strings"
	"sync"
)

var hostURLCenterMap = make(map[string]*urlCenter)
var urlCenterLock = &sync.RWMutex{}

func urlCenterInstance(schema, host string) *urlCenter {
	key := fmt.Sprintf("%s_%s", schema, host)
	urlCenterLock.RLock()
	mURLCenter, exist := hostURLCenterMap[key]
	urlCenterLock.RUnlock()
	if exist {
		return mURLCenter
	}
	urlCenterLock.Lock()
	_, exist = hostURLCenterMap[key]
	if !exist {
		mURLCenter = newURLCenter(schema, host)
		hostURLCenterMap[key] = mURLCenter
	}
	urlCenterLock.Unlock()
	return mURLCenter
}

func newURLCenter(schema, host string) *urlCenter {
	return &urlCenter{
		urlFormat:  fmt.Sprintf("%s://%s", schema, host),
		pathURLMap: make(map[string]string),
		lock:       &sync.RWMutex{},
	}
}

type urlCenter struct {
	urlFormat  string
	pathURLMap map[string]string
	lock       *sync.RWMutex
}

// example path: /Retail/User
// will build url to schema://host/Retail/User
func (u *urlCenter) getURL(path string) string {
	path = strings.TrimPrefix(path, "/")
	u.lock.RLock()
	url, exist := u.pathURLMap[path]
	u.lock.RUnlock()
	if exist {
		return url
	}
	u.lock.Lock()
	_, exist = u.pathURLMap[path]
	if !exist {
		url = fmt.Sprintf("%s/%s", u.urlFormat, path)
		u.pathURLMap[path] = url
	}
	u.lock.Unlock()
	return url
}
