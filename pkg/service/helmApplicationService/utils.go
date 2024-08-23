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
package helmApplicationService

import (
	"errors"
	client "github.com/devtron-labs/kubelink/grpc"
	"github.com/devtron-labs/kubelink/pkg/k8sInformer"
	"strconv"
)

func getUniqueReleaseIdentifierName(releaseIdentifier *client.ReleaseIdentifier) string {
	return releaseIdentifier.ReleaseNamespace + "_" + releaseIdentifier.ReleaseName + "_" + strconv.Itoa(int(releaseIdentifier.ClusterConfig.ClusterId))
}

const (
	DirCreatingError = "err in creating dir"
)

func IsReleaseNotFoundInCacheError(err error) bool {
	return errors.Is(err, k8sInformer.ErrorCacheMissReleaseNotFound)
}
