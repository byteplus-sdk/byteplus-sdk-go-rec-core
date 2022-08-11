gen_sdk_metrics:
	protoc --go_out=metrics/protocol -I=metrics/protocol --go_opt=paths=source_relative sdk_metrics.proto