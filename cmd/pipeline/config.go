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

package main

import (
	"net/url"
	"os"

	"emperror.dev/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/internal/anchore"
	"github.com/banzaicloud/pipeline/internal/app/frontend"
	"github.com/banzaicloud/pipeline/internal/platform/errorhandler"
	"github.com/banzaicloud/pipeline/internal/platform/log"
	"github.com/banzaicloud/pipeline/pkg/viperx"
)

// configuration holds any kind of configuration that comes from the outside world and
// is necessary for running the application.
type configuration struct {
	// Log configuration
	Log log.Config

	// ErrorHandler configuration
	ErrorHandler errorhandler.Config

	// Auth configuration
	Auth authConfig

	// Frontend configuration
	Frontend frontend.Config

	// Cluster configuration
	Cluster clusterConfig
}

// Validate validates the configuration.
func (c configuration) Validate() error {
	if err := c.ErrorHandler.Validate(); err != nil {
		return err
	}

	if err := c.Auth.Validate(); err != nil {
		return err
	}

	if err := c.Frontend.Validate(); err != nil {
		return err
	}

	return nil
}

// authConfig contains auth configuration.
type authConfig struct {
	Token authTokenConfig
	Role  authRoleConfig
}

// Validate validates the configuration.
func (c authConfig) Validate() error {
	if err := c.Token.Validate(); err != nil {
		return err
	}

	if err := c.Role.Validate(); err != nil {
		return err
	}

	return nil
}

// authRoleConfig contains role based authorization configuration.
type authRoleConfig struct {
	Default string
	Binding map[string]string
}

// Validate validates the configuration.
func (c authRoleConfig) Validate() error {
	if c.Default == "" {
		return errors.New("auth role default is required")
	}

	return nil
}

// authTokenConfig contains auth configuration.
type authTokenConfig struct {
	SigningKey string
	Issuer     string
	Audience   string
}

// Validate validates the configuration.
func (c authTokenConfig) Validate() error {
	if c.SigningKey == "" {
		return errors.New("auth token signing key is required")
	}

	if len(c.SigningKey) < 32 {
		return errors.New("auth token signing key must be at least 32 characters")
	}

	return nil
}

// clusterConfig contains cluster configuration.
type clusterConfig struct {
	Vault        clusterVaultConfig
	Monitoring   clusterMonitorConfig
	SecurityScan clusterSecurityScanConfig
}

// Validate validates the configuration.
func (c clusterConfig) Validate() error {
	if c.SecurityScan.Enabled {
		if err := c.SecurityScan.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// clusterVaultConfig contains cluster vault configuration.
type clusterVaultConfig struct {
	Enabled bool
	Managed clusterVaultManagedConfig
}

// clusterVaultManagedConfig contains cluster vault configuration.
type clusterVaultManagedConfig struct {
	Enabled bool
}

// clusterMonitorConfig contains cluster vault configuration.
type clusterMonitorConfig struct {
	Enabled bool
}

// clusterSecurityScanConfig contains cluster security scan configuration.
type clusterSecurityScanConfig struct {
	Enabled bool
	Anchore clusterSecurityScanAnchoreConfig
}

func (c clusterSecurityScanConfig) Validate() error {
	if c.Anchore.Enabled {
		if err := c.Anchore.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// clusterSecurityScanAnchoreConfig contains cluster security scan anchore configuration.
type clusterSecurityScanAnchoreConfig struct {
	Enabled bool

	anchore.Config `mapstructure:",squash"`
}

func (c clusterSecurityScanAnchoreConfig) Validate() error {
	if c.Enabled {
		if _, err := url.Parse(c.Endpoint); err != nil {
			return errors.Wrap(err, "anchore endpoint must be a valid URL")
		}

		if c.User == "" {
			return errors.New("anchore user is required")
		}

		if c.Password == "" {
			return errors.New("anchore password is required")
		}
	}

	return nil
}

// configure configures some defaults in the Viper instance.
func configure(v *viper.Viper, _ *pflag.FlagSet) {
	// Application constants
	v.Set("appName", appName)
	v.Set("appVersion", version)

	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		v.SetDefault("no_color", true)
	}

	// Log configuration
	v.SetDefault("logging.logformat", "text")
	v.SetDefault("logging.loglevel", "debug")
	v.RegisterAlias("log.format", "logging.logformat") // TODO: deprecate the above
	v.RegisterAlias("log.level", "logging.loglevel")
	v.RegisterAlias("log.noColor", "no_color")

	// ErrorHandler configuration
	v.RegisterAlias("errorHandler.serviceName", "appName")
	v.RegisterAlias("errorHandler.serviceVersion", "appVersion")

	// Auth configuration
	v.SetDefault("auth.token.issuer", "https://banzaicloud.com/")
	v.SetDefault("auth.token.audience", "https://pipeline.banzaicloud.com")

	v.SetDefault("auth.role.default", auth.RoleAdmin)
	v.SetDefault("auth.role.binding", map[string]string{
		auth.RoleAdmin:  ".*",
		auth.RoleMember: "",
	})

	v.SetDefault("frontend.issue.enabled", false)
	v.SetDefault("frontend.issue.driver", "github")
	v.SetDefault("frontend.issue.labels", []string{"community"})

	v.RegisterAlias("frontend.issue.github.token", "github.token")
	v.SetDefault("frontend.issue.github.owner", "banzaicloud")
	v.SetDefault("frontend.issue.github.repository", "pipeline-issues")

	v.SetDefault("cluster.vault.enabled", true)
	v.SetDefault("cluster.vault.managed.enabled", false)
	v.SetDefault("cluster.monitoring.enabled", true)
	v.SetDefault("cluster.securityScan.enabled", true)
	v.SetDefault("cluster.securityScan.anchore.enabled", false)
	v.SetDefault("cluster.securityScan.anchore.endpoint", "")
	v.SetDefault("cluster.securityScan.anchore.user", "")
	v.SetDefault("cluster.securityScan.anchore.password", "")
}

func registerAliases(v *viper.Viper) {
	// Auth configuration
	viperx.RegisterAlias(v, "auth.tokensigningkey", "auth.token.signingKey")
	viperx.RegisterAlias(v, "auth.jwtissuer", "auth.token.issuer")
	viperx.RegisterAlias(v, "auth.jwtaudience", "auth.token.audience")

	// Frontend configuration
	viperx.RegisterAlias(v, "issue.type", "frontend.issue.driver")
	viperx.RegisterAlias(v, "issue.githubLabels", "frontend.issue.labels")

	viperx.RegisterAlias(v, "issue.githubOwner", "frontend.issue.github.owner")
	viperx.RegisterAlias(v, "issue.githubRepository", "frontend.issue.github.repository")
}
