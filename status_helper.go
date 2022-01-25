package core

func IsUploadSuccess(code int32) bool {
	// It is still considered as success, which is rejected for idempotent
	return code == StatusCodeSuccess || code == StatusCodeIdempotent
}

func IsSuccess(code int32) bool {
	return code == StatusCodeSuccess || code == 200
}

func IsServerOverload(code int32) bool {
	return code == StatusCodeTooManyRequest
}

func IsLossOperation(code int32) bool {
	return code == StatusCodeOperationLoss
}
