package core

import (
	"reflect"
	"sync"
	"testing"
)

func Test_newURLCenter(t *testing.T) {
	type args struct {
		schema string
		host   string
	}
	tests := []struct {
		name string
		args args
		want *urlCenter
	}{
		{
			name: "common",
			args: args{
				schema: "https",
				host:   "127.0.0.1",
			},
			want: &urlCenter{
				urlFormat:  "https://127.0.0.1",
				pathURLMap: make(map[string]string),
				lock:       &sync.RWMutex{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newURLCenter(tt.args.schema, tt.args.host); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newURLCenter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_urlCenterImpl_getURL(t *testing.T) {
	type fields struct {
		urlFormat  string
		pathUrlMap map[string]string
	}
	type args struct {
		path string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "hit_cache",
			fields: fields{
				urlFormat: "https://127.0.0.1",
				pathUrlMap: map[string]string{
					"RetailUser": "https://127.0.0.1/RetailUser",
				},
			},
			args: args{path: "RetailUser"},
			want: "https://127.0.0.1/RetailUser",
		},
		{
			name: "not_hit_cache",
			fields: fields{
				urlFormat:  "https://127.0.0.1",
				pathUrlMap: map[string]string{},
			},
			args: args{path: "RetailUser"},
			want: "https://127.0.0.1/RetailUser",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &urlCenter{
				urlFormat:  tt.fields.urlFormat,
				pathURLMap: tt.fields.pathUrlMap,
				lock:       &sync.RWMutex{},
			}
			if got := u.getURL(tt.args.path); got != tt.want {
				t.Errorf("getURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLCenterInstanceAndGetUrl(t *testing.T) {
	type args struct {
		schema string
		host   string
		path   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "common",
			args: args{
				schema: "https",
				host:   "127.0.0.1",
				path:   "/Retail/User",
			},
			want: "https://127.0.0.1/Retail/User",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := urlCenterInstance(tt.args.schema, tt.args.host).getURL(tt.args.path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("URLCenterInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}
