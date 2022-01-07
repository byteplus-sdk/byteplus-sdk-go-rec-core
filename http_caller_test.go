package core

import (
	"testing"

	"github.com/byteplus-sdk/byteplus-sdk-go-rec-core/option"
)

func TestHttpCaller_withOptionQueries(t *testing.T) {
	type args struct {
		options *option.Options
		url     string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "empty_with_stage",
			args: args{
				options: &option.Options{
					Queries: map[string]string{
						"stage": "pre",
					},
				},
				url: "https://www.bytedance.com",
			},
			want: "https://www.bytedance.com?stage=pre",
		},
		{
			name: "exist_with_stage",
			args: args{
				options: &option.Options{
					Queries: map[string]string{
						"stage": "pre",
					},
				},
				url: "https://www.bytedance.com?query1=value1",
			},
			want: "https://www.bytedance.com?query1=value1&stage=pre",
		},
		{
			name: "exist_with_empty",
			args: args{
				options: &option.Options{},
				url:     "https://www.bytedance.com?query1=value1",
			},
			want: "https://www.bytedance.com?query1=value1",
		},
		{
			name: "empty",
			args: args{
				options: &option.Options{},
				url:     "https://www.bytedance.com",
			},
			want: "https://www.bytedance.com",
		},
		{
			name: "exist_with_query",
			args: args{
				options: &option.Options{
					Queries: map[string]string{
						"query2": "value2",
					},
				},
				url: "https://www.bytedance.com?query1=value1",
			},
			want: "https://www.bytedance.com?query1=value1&query2=value2",
		},
		{
			name: "exist_with_stage",
			args: args{
				options: &option.Options{
					Queries: map[string]string{
						"stage": "pre",
					},
				},
				url: "https://www.bytedance.com?query1=value1",
			},
			want: "https://www.bytedance.com?query1=value1&stage=pre",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &httpCaller{}
			if got := c.withOptionQueries(tt.args.options, tt.args.url); got != tt.want {
				t.Errorf("withOptionQueries() = %v, want %v", got, tt.want)
			}
		})
	}
}
