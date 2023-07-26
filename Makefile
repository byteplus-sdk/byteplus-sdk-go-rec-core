gen_sdk_metrics:
	protoc --go_out=metrics/protocol -I=metrics/protocol --go_opt=paths=source_relative byteplus_rec_sdk_metrics.proto