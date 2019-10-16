// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/pipeline/internal/clusterfeature/clusterfeatureadapter"
	"github.com/banzaicloud/pipeline/secret"
)

const (
	featureName         = "logging"
	providerAmazonS3    = "s3"
	providerGoogleGCS   = "gcs"
	providerAlibabaOSS  = "oss"
	providerAzure       = "azure"
	secretTag           = "feature:logging"
	fluentbitSecretName = "logging-operator-fluentbit-secret"
	fluentdSecretName   = "logging-operator-fluentd-secret"
	lokiReleaseName     = "loki"
	grafanaReleaseName  = "grafana"

	kubeCaCertKey  = "ca.crt"
	kubeTlsCertKey = "tls.crt"
	kubeTlsKeyKey  = "tls.key"
)

type obj = map[string]interface{}

func init() {
	//SchemeBuilder.Register(&ClusterOutput{}, &ClusterOutputList{})
}

func getTLSSecretName(clusterID uint) string {
	return fmt.Sprintf("logging-tls-%d", clusterID)
}

func getGrafanaReleaseSecretTag() string {
	return fmt.Sprintf("release:%s", grafanaReleaseName)
}

func getSecretClusterTags(cluster clusterfeatureadapter.Cluster) []string {
	clusterNameSecretTag := fmt.Sprintf("cluster:%s", cluster.GetName())
	clusterUidSecretTag := fmt.Sprintf("clusterUID:%s", cluster.GetUID())
	return []string{clusterNameSecretTag, clusterUidSecretTag}
}

func isSecretNotFoundError(err error) bool {
	errCause := errors.Cause(err)
	if errCause == secret.ErrSecretNotExists {
		return true
	}
	return false
}
