/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package error

import (
	"google.golang.org/grpc/codes"
)

// list of error strings from Helm. These are part of the errors we check for presence in Helm's error messages.
const (
	YAMLToJSONConversionError   = "error converting YAML to JSON"
	ClusterUnreachableErrorMsg  = "cluster unreachable"
	CrdPreconditionErrorMsg     = "ensure crds are installed first"
	ArrayStringMismatchErrorMsg = "got array expected string"
	NotFoundErrorMsg            = "not found" //this is a generic type constant, an error could be namespace "ns1" not found or service "ser1" not found.
	InvalidValueErrorMsg        = "invalid value"
	OperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
	ForbiddenErrorMsg           = "forbidden"
)

// list of internal errors, these errors are easy for the users to understand
const (
	InternalClusterUnreachableErrorMsg  = "cluster unreachable"
	InternalOperationInProgressErrorMsg = "another operation (install/upgrade/rollback) is in progress"
)

type errorGrpcCodeTuple struct {
	errorMsg string
	grpcCode codes.Code
}

var helmErrorInternalErrorMap = map[string]errorGrpcCodeTuple{
	ClusterUnreachableErrorMsg:  {errorMsg: InternalClusterUnreachableErrorMsg, grpcCode: codes.DeadlineExceeded},
	OperationInProgressErrorMsg: {errorMsg: InternalOperationInProgressErrorMsg, grpcCode: codes.FailedPrecondition},
}

var DynamicErrorMapping = map[string]codes.Code{
	NotFoundErrorMsg:            codes.NotFound,
	ForbiddenErrorMsg:           codes.PermissionDenied,
	InvalidValueErrorMsg:        codes.InvalidArgument,
	ArrayStringMismatchErrorMsg: codes.InvalidArgument,
	CrdPreconditionErrorMsg:     codes.FailedPrecondition,
}
