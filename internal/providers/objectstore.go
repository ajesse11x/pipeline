// Copyright © 2018 Banzai Cloud
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

package providers

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/config"
	"github.com/banzaicloud/pipeline/internal/objectstore"
	"github.com/banzaicloud/pipeline/internal/providers/alibaba"
	"github.com/banzaicloud/pipeline/internal/providers/amazon"
	"github.com/banzaicloud/pipeline/internal/providers/azure"
	"github.com/banzaicloud/pipeline/internal/providers/google"
	"github.com/banzaicloud/pipeline/internal/providers/oracle"
	pkgErrors "github.com/banzaicloud/pipeline/pkg/errors"
	"github.com/banzaicloud/pipeline/pkg/providers"
	"github.com/banzaicloud/pipeline/secret"
)

// ObjectStoreContext describes all parameters necessary to create a cloud provider agnostic object store instance.
type ObjectStoreContext struct {
	Provider     string
	Secret       *secret.SecretItemResponse
	Organization *auth.Organization

	// Location (or region) is used by some cloud providers to determine where the bucket should be created.
	Location string

	// Azure specific parameters
	ResourceGroup  string
	StorageAccount string

	// ForceOperation indicates whether the operation needs to be executed forcibly (some errors are ignored)
	ForceOperation bool
}

// NewObjectStore creates an object store client for the given cloud provider.
// The created object is initialized with the passed in secret and organization.
func NewObjectStore(ctx *ObjectStoreContext, logger logrus.FieldLogger) (objectstore.ObjectStoreService, error) {
	db := config.DB()

	switch ctx.Provider {
	case providers.Alibaba:
		return alibaba.NewObjectStore(ctx.Location, ctx.Secret, ctx.Organization, db, logger, ctx.ForceOperation)

	case providers.Amazon:
		return amazon.NewObjectStore(ctx.Location, ctx.Secret, ctx.Organization, db, logger, ctx.ForceOperation)

	case providers.Azure:
		return azure.NewObjectStore(ctx.Location, ctx.ResourceGroup, ctx.StorageAccount, ctx.Secret, ctx.Organization, db, logger, ctx.ForceOperation)

	case providers.Google:
		return google.NewObjectStore(ctx.Organization, ctx.Secret, ctx.Location, db, logger, ctx.ForceOperation)

	case providers.Oracle:
		return oracle.NewObjectStore(ctx.Location, ctx.Secret, ctx.Organization, db, logger, ctx.ForceOperation)

	default:
		return nil, pkgErrors.ErrorNotSupportedCloudType
	}
}

func GetBucketLocation(provider string, secret *secret.SecretItemResponse, bucketName string, orgID uint, log logrus.FieldLogger) (string, error) {

	switch provider {
	case providers.Alibaba:
		defaultRegion := viper.GetString(config.AlibabaInitializeRegionKey)
		return alibaba.GetBucketLocation(secret, bucketName, defaultRegion, orgID, log)

	case providers.Amazon:
		defaultRegion := viper.GetString(config.AmazonInitializeRegionKey)
		return amazon.GetBucketRegion(secret, bucketName, defaultRegion, orgID, log)

	default:
		return "", pkgErrors.ErrorNotSupportedCloudType
	}

}
