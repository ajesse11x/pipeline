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

package main

import (
	"context"
	"encoding/base32"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"emperror.dev/emperror"
	"emperror.dev/errors"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/message"
	watermillMiddleware "github.com/ThreeDotsLabs/watermill/message/router/middleware"
	evbus "github.com/asaskevich/EventBus"
	bauth "github.com/banzaicloud/bank-vaults/pkg/sdk/auth"
	ginprometheus "github.com/banzaicloud/go-gin-prometheus"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-kit/kit/endpoint"
	kithttp "github.com/go-kit/kit/transport/http"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sagikazarmark/kitx/correlation"
	kitxendpoint "github.com/sagikazarmark/kitx/endpoint"
	kitxhttp "github.com/sagikazarmark/kitx/transport/http"
	"github.com/sagikazarmark/ocmux"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"logur.dev/logur"

	"github.com/banzaicloud/pipeline/api"
	"github.com/banzaicloud/pipeline/api/ark/backups"
	"github.com/banzaicloud/pipeline/api/ark/backupservice"
	"github.com/banzaicloud/pipeline/api/ark/buckets"
	"github.com/banzaicloud/pipeline/api/ark/restores"
	"github.com/banzaicloud/pipeline/api/ark/schedules"
	"github.com/banzaicloud/pipeline/api/cluster/namespace"
	"github.com/banzaicloud/pipeline/api/cluster/pke"
	cgroupAPI "github.com/banzaicloud/pipeline/api/clustergroup"
	"github.com/banzaicloud/pipeline/api/common"
	"github.com/banzaicloud/pipeline/auth"
	"github.com/banzaicloud/pipeline/auth/authadapter"
	"github.com/banzaicloud/pipeline/auth/authdriver"
	"github.com/banzaicloud/pipeline/auth/authgen"
	"github.com/banzaicloud/pipeline/cluster"
	"github.com/banzaicloud/pipeline/config"
	"github.com/banzaicloud/pipeline/dns"
	anchore2 "github.com/banzaicloud/pipeline/internal/anchore"
	"github.com/banzaicloud/pipeline/internal/app/frontend"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/api/middleware/audit"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/auth/token"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/auth/token/tokenadapter"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/auth/token/tokendriver"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/cap/capdriver"
	googleproject "github.com/banzaicloud/pipeline/internal/app/pipeline/cloud/google/project"
	googleprojectdriver "github.com/banzaicloud/pipeline/internal/app/pipeline/cloud/google/project/projectdriver"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/secrettype"
	"github.com/banzaicloud/pipeline/internal/app/pipeline/secrettype/secrettypedriver"
	arkClusterManager "github.com/banzaicloud/pipeline/internal/ark/clustermanager"
	arkEvents "github.com/banzaicloud/pipeline/internal/ark/events"
	arkSync "github.com/banzaicloud/pipeline/internal/ark/sync"
	"github.com/banzaicloud/pipeline/internal/cloudinfo"
	intCluster "github.com/banzaicloud/pipeline/internal/cluster"
	intClusterAuth "github.com/banzaicloud/pipeline/internal/cluster/auth"
	"github.com/banzaicloud/pipeline/internal/cluster/clustersecret"
	"github.com/banzaicloud/pipeline/internal/cluster/clustersecret/clustersecretadapter"
	"github.com/banzaicloud/pipeline/internal/cluster/endpoints"
	prometheusMetrics "github.com/banzaicloud/pipeline/internal/cluster/metrics/adapters/prometheus"
	"github.com/banzaicloud/pipeline/internal/clusterfeature"
	"github.com/banzaicloud/pipeline/internal/clusterfeature/clusterfeatureadapter"
	"github.com/banzaicloud/pipeline/internal/clusterfeature/clusterfeaturedriver"
	featureDns "github.com/banzaicloud/pipeline/internal/clusterfeature/features/dns"
	featureMonitoring "github.com/banzaicloud/pipeline/internal/clusterfeature/features/monitoring"
	"github.com/banzaicloud/pipeline/internal/clusterfeature/features/securityscan"
	"github.com/banzaicloud/pipeline/internal/clusterfeature/features/securityscan/securityscanadapter"
	featureVault "github.com/banzaicloud/pipeline/internal/clusterfeature/features/vault"
	"github.com/banzaicloud/pipeline/internal/clustergroup"
	cgroupAdapter "github.com/banzaicloud/pipeline/internal/clustergroup/adapter"
	"github.com/banzaicloud/pipeline/internal/clustergroup/deployment"
	"github.com/banzaicloud/pipeline/internal/common/commonadapter"
	"github.com/banzaicloud/pipeline/internal/dashboard"
	"github.com/banzaicloud/pipeline/internal/federation"
	"github.com/banzaicloud/pipeline/internal/global"
	"github.com/banzaicloud/pipeline/internal/helm"
	"github.com/banzaicloud/pipeline/internal/helm/helmadapter"
	cgFeatureIstio "github.com/banzaicloud/pipeline/internal/istio/istiofeature"
	"github.com/banzaicloud/pipeline/internal/monitor"
	"github.com/banzaicloud/pipeline/internal/platform/appkit"
	"github.com/banzaicloud/pipeline/internal/platform/buildinfo"
	"github.com/banzaicloud/pipeline/internal/platform/errorhandler"
	ginternal "github.com/banzaicloud/pipeline/internal/platform/gin"
	"github.com/banzaicloud/pipeline/internal/platform/gin/correlationid"
	"github.com/banzaicloud/pipeline/internal/platform/gin/ginauth"
	ginlog "github.com/banzaicloud/pipeline/internal/platform/gin/log"
	ginutils "github.com/banzaicloud/pipeline/internal/platform/gin/utils"
	"github.com/banzaicloud/pipeline/internal/platform/log"
	platformlog "github.com/banzaicloud/pipeline/internal/platform/log"
	"github.com/banzaicloud/pipeline/internal/platform/watermill"
	azurePKEAdapter "github.com/banzaicloud/pipeline/internal/providers/azure/pke/adapter"
	azurePKEDriver "github.com/banzaicloud/pipeline/internal/providers/azure/pke/driver"
	"github.com/banzaicloud/pipeline/internal/providers/google"
	"github.com/banzaicloud/pipeline/internal/providers/google/googleadapter"
	anchore "github.com/banzaicloud/pipeline/internal/security"
	pkgAuth "github.com/banzaicloud/pipeline/pkg/auth"
	"github.com/banzaicloud/pipeline/pkg/ctxutil"
	"github.com/banzaicloud/pipeline/pkg/k8sclient"
	"github.com/banzaicloud/pipeline/pkg/problems"
	"github.com/banzaicloud/pipeline/pkg/providers"
	"github.com/banzaicloud/pipeline/secret"
	"github.com/banzaicloud/pipeline/spotguide"
	"github.com/banzaicloud/pipeline/spotguide/scm"
)

// Provisioned by ldflags
// nolint: gochecknoglobals
var (
	version    string
	commitHash string
	buildDate  string
)

func main() {
	v, p := viper.GetViper(), pflag.NewFlagSet(friendlyAppName, pflag.ExitOnError)

	configure(v, p)

	p.Bool("version", false, "Show version information")

	_ = p.Parse(os.Args[1:])

	if v, _ := p.GetBool("version"); v {
		fmt.Printf("%s version %s (%s) built on %s\n", friendlyAppName, version, commitHash, buildDate)

		os.Exit(0)
	}

	registerAliases(v)

	var conf configuration
	err := viper.Unmarshal(&conf)
	emperror.Panic(errors.Wrap(err, "failed to unmarshal configuration"))

	// Create logger (first thing after configuration loading)
	logger := log.NewLogger(conf.Log)

	// Legacy logger instance
	logrusLogger := config.Logger()

	// Provide some basic context to all log lines
	logger = log.WithFields(logger, map[string]interface{}{"application": appName})

	log.SetStandardLogger(logger)
	log.SetK8sLogger(logger)

	err = conf.Validate()
	if err != nil {
		logger.Error(err.Error())

		os.Exit(3)
	}

	errorHandler, err := errorhandler.New(conf.ErrorHandler, logger)
	if err != nil {
		logger.Error(err.Error())

		os.Exit(1)
	}
	defer errorHandler.Close()
	defer emperror.HandleRecover(errorHandler)
	global.SetErrorHandler(errorHandler)

	buildInfo := buildinfo.New(version, commitHash, buildDate)

	logger.Info("starting application", buildInfo.Fields())

	// Connect to database
	db := config.DB()
	cicdDB, err := config.CICDDB()
	emperror.Panic(err)

	commonLogger := commonadapter.NewContextAwareLogger(logger, appkit.ContextExtractor{})

	basePath := viper.GetString("pipeline.basepath")

	publisher, subscriber := watermill.NewPubSub(logger)
	defer publisher.Close()
	defer subscriber.Close()

	publisher, _ = message.MessageTransformPublisherDecorator(func(msg *message.Message) {
		if cid, ok := correlation.FromContext(msg.Context()); ok {
			watermillMiddleware.SetCorrelationID(cid, msg)
		}
	})(publisher)

	subscriber, _ = message.MessageTransformSubscriberDecorator(func(msg *message.Message) {
		if cid := watermillMiddleware.MessageCorrelationID(msg); cid != "" {
			msg.SetContext(correlation.ToContext(msg.Context(), cid))
		}
	})(subscriber)

	// Used internally to make sure every event/command bus uses the same one
	eventMarshaler := cqrs.JSONMarshaler{GenerateName: cqrs.StructName}

	secretStore := commonadapter.NewSecretStore(secret.Store, commonadapter.OrgIDContextExtractorFunc(auth.GetCurrentOrganizationID))

	organizationStore := authadapter.NewGormOrganizationStore(db)

	const organizationTopic = "organization"
	var organizationSyncer auth.OIDCOrganizationSyncer
	{
		eventBus, _ := cqrs.NewEventBus(
			publisher,
			func(eventName string) string { return organizationTopic },
			eventMarshaler,
		)
		eventDispatcher := authgen.NewOrganizationEventDispatcher(eventBus)

		roleBinder, err := auth.NewRoleBinder(conf.Auth.Role.Default, conf.Auth.Role.Binding)
		emperror.Panic(err)

		organizationSyncer = auth.NewOIDCOrganizationSyncer(
			auth.NewOrganizationSyncer(
				organizationStore,
				eventDispatcher,
				commonLogger.WithFields(map[string]interface{}{"component": "auth"}),
			),
			roleBinder,
		)
	}

	// Initialize auth
	tokenStore := bauth.NewVaultTokenStore("pipeline")
	tokenGenerator := pkgAuth.NewJWTTokenGenerator(
		conf.Auth.Token.Issuer,
		conf.Auth.Token.Audience,
		base32.StdEncoding.EncodeToString([]byte(conf.Auth.Token.SigningKey)),
	)
	tokenManager := pkgAuth.NewTokenManager(tokenGenerator, tokenStore)
	auth.Init(cicdDB, conf.Auth.Token.SigningKey, tokenStore, tokenManager, organizationSyncer)

	if viper.GetBool(config.DBAutoMigrateEnabled) {
		logger.Info("running automatic schema migrations")

		err = Migrate(db, logrusLogger, commonLogger)
		if err != nil {
			panic(err)
		}
	}

	// External DNS service
	dnsSvc, err := dns.GetExternalDnsServiceClient()
	if err != nil {
		emperror.Panic(errors.WithMessage(err, "getting external dns service client failed"))
	}

	if dnsSvc == nil {
		logger.Info("external dns service functionality is not enabled")
	}

	prometheus.MustRegister(cluster.NewExporter())

	clusterEventBus := evbus.New()
	clusterEvents := cluster.NewClusterEvents(clusterEventBus)
	clusters := intCluster.NewClusters(db)
	secretValidator := providers.NewSecretValidator(secret.Store)
	statusChangeDurationMetric := prometheusMetrics.MakePrometheusClusterStatusChangeDurationMetric()
	// Initialise cluster total metric
	clusterTotalMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pipeline",
		Name:      "cluster_total",
		Help:      "the number of clusters launched",
	},
		[]string{"provider", "location"},
	)
	type totalClusterMetric struct {
		Location string
		Cloud    string
		Count    int
	}
	totalClusters := make([]totalClusterMetric, 0)
	// SELECT count(id) as count, location, cloud FROM clusters GROUP BY location, cloud; (init values)
	if err := db.Raw("SELECT count(id) as count, location, cloud FROM clusters GROUP BY location, cloud").Scan(&totalClusters).Error; err != nil {
		logger.Error(err.Error())
		// TODO: emperror.Panic?
	}
	for _, row := range totalClusters {
		clusterTotalMetric.With(
			map[string]string{
				"location": row.Location,
				"provider": row.Cloud,
			}).Add(float64(row.Count))
	}
	prometheus.MustRegister(statusChangeDurationMetric, clusterTotalMetric)

	externalBaseURL := viper.GetString("pipeline.externalURL")
	if externalBaseURL == "" {
		externalBaseURL = "http://" + viper.GetString("pipeline.bindaddr")
		logger.Warn("no pipeline.external_url set, falling back to bind address", map[string]interface{}{
			"fallback": externalBaseURL,
		})
	}

	externalURLInsecure := viper.GetBool(config.PipelineExternalURLInsecure)

	oidcIssuerURL := viper.GetString(config.OIDCIssuerURL)

	workflowClient, err := config.CadenceClient()
	if err != nil {
		errorHandler.Handle(errors.WrapIf(err, "Failed to configure Cadence client"))
	}

	clusterManager := cluster.NewManager(clusters, secretValidator, clusterEvents, statusChangeDurationMetric, clusterTotalMetric, workflowClient, logrusLogger, errorHandler)
	commonClusterGetter := common.NewClusterGetter(clusterManager, logrusLogger, errorHandler)

	clusterTTLController := cluster.NewTTLController(clusterManager, clusterEventBus, logrusLogger.WithField("subsystem", "ttl-controller"), errorHandler)
	defer clusterTTLController.Stop()
	err = clusterTTLController.Start()
	emperror.Panic(err)

	if viper.GetBool(config.MonitorEnabled) {
		client, err := k8sclient.NewInClusterClient()
		if err != nil {
			errorHandler.Handle(errors.WrapIf(err, "failed to enable monitoring"))
		} else {
			dnsBaseDomain, err := dns.GetBaseDomain()
			if err != nil {
				errorHandler.Handle(errors.WrapIf(err, "failed to enable monitoring"))
			}

			monitorClusterSubscriber := monitor.NewClusterSubscriber(
				client,
				clusterManager,
				db,
				dnsBaseDomain,
				viper.GetString(config.ControlPlaneNamespace),
				viper.GetString(config.PipelineSystemNamespace),
				viper.GetString(config.MonitorConfigMap),
				viper.GetString(config.MonitorConfigMapPrometheusKey),
				viper.GetString(config.MonitorCertSecret),
				viper.GetString(config.MonitorCertMountPath),
				errorHandler,
			)
			monitorClusterSubscriber.Init()
			monitorClusterSubscriber.Register(monitor.NewClusterEvents(clusterEventBus))
		}
	}

	if viper.GetBool(config.SpotMetricsEnabled) {
		go monitor.NewSpotMetricsExporter(context.Background(), clusterManager, logrusLogger.WithField("subsystem", "spot-metrics-exporter")).Run(viper.GetDuration(config.SpotMetricsCollectionInterval))
	}

	cloudInfoEndPoint := viper.GetString(config.CloudInfoEndPoint)
	if cloudInfoEndPoint == "" {
		errorHandler.Handle(errors.New("missing CloudInfo endpoint"))
		return
	}
	cloudInfoClient := cloudinfo.NewClient(cloudInfoEndPoint, logrusLogger)

	gormAzurePKEClusterStore := azurePKEAdapter.NewGORMAzurePKEClusterStore(db, commonLogger)
	clusterCreators := api.ClusterCreators{
		PKEOnAzure: azurePKEDriver.MakeAzurePKEClusterCreator(
			azurePKEDriver.ClusterCreatorConfig{
				OIDCIssuerURL:               oidcIssuerURL,
				PipelineExternalURL:         externalBaseURL,
				PipelineExternalURLInsecure: externalURLInsecure,
			},
			logrusLogger,
			authdriver.NewOrganizationGetter(db),
			secret.Store,
			gormAzurePKEClusterStore,
			workflowClient,
		),
	}
	clusterDeleters := api.ClusterDeleters{
		PKEOnAzure: azurePKEDriver.MakeAzurePKEClusterDeleter(
			clusterEvents,
			clusterManager.GetKubeProxyCache(),
			logrusLogger,
			secret.Store,
			statusChangeDurationMetric,
			gormAzurePKEClusterStore,
			workflowClient,
		),
	}

	cgroupAdapter := cgroupAdapter.NewClusterGetter(clusterManager)
	clusterGroupManager := clustergroup.NewManager(cgroupAdapter, clustergroup.NewClusterGroupRepository(db, logrusLogger), logrusLogger, errorHandler)
	infraNamespace := viper.GetString(config.PipelineSystemNamespace)
	federationHandler := federation.NewFederationHandler(cgroupAdapter, infraNamespace, logrusLogger, errorHandler)
	deploymentManager := deployment.NewCGDeploymentManager(db, cgroupAdapter, logrusLogger, errorHandler)
	serviceMeshFeatureHandler := cgFeatureIstio.NewServiceMeshFeatureHandler(cgroupAdapter, logrusLogger, errorHandler)
	clusterGroupManager.RegisterFeatureHandler(federation.FeatureName, federationHandler)
	clusterGroupManager.RegisterFeatureHandler(deployment.FeatureName, deploymentManager)
	clusterGroupManager.RegisterFeatureHandler(cgFeatureIstio.FeatureName, serviceMeshFeatureHandler)
	clusterUpdaters := api.ClusterUpdaters{
		PKEOnAzure: azurePKEDriver.MakeAzurePKEClusterUpdater(
			logrusLogger,
			externalBaseURL,
			externalURLInsecure,
			secret.Store,
			gormAzurePKEClusterStore,
			workflowClient,
		),
	}

	clusterAPI := api.NewClusterAPI(clusterManager, commonClusterGetter, workflowClient, cloudInfoClient, clusterGroupManager, logrusLogger, errorHandler, externalBaseURL, externalURLInsecure, clusterCreators, clusterDeleters, clusterUpdaters)

	nplsApi := api.NewNodepoolManagerAPI(commonClusterGetter, logrusLogger, errorHandler)

	// Initialise Gin router
	engine := gin.New()

	router := mux.NewRouter()
	router.Use(ocmux.Middleware())

	// These two paths can contain sensitive information, so it is advised not to log them out.
	skipPaths := viper.GetStringSlice("audit.skippaths")
	engine.Use(correlationid.Middleware())
	engine.Use(ginlog.Middleware(logrusLogger, skipPaths...))

	// Add prometheus metric endpoint
	if viper.GetBool(config.MetricsEnabled) {
		p := ginprometheus.NewPrometheus("pipeline", []string{})
		p.SetListenAddress(viper.GetString(config.MetricsAddress) + ":" + viper.GetString(config.MetricsPort))
		p.Use(engine, "/metrics")
	}

	engine.Use(gin.Recovery())
	drainModeMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "pipeline",
		Name:      "drain_mode",
		Help:      "read only mode is on/off",
	})
	prometheus.MustRegister(drainModeMetric)
	engine.Use(ginternal.NewDrainModeMiddleware(drainModeMetric, errorHandler).Middleware)
	engine.Use(cors.New(config.GetCORS()))
	if viper.GetBool("audit.enabled") {
		logger.Info("Audit enabled, installing Gin audit middleware")
		engine.Use(audit.LogWriter(skipPaths, viper.GetStringSlice("audit.headers"), db, logrusLogger))
	}
	engine.Use(func(c *gin.Context) { // TODO: move to middleware
		c.Request = c.Request.WithContext(ctxutil.WithParams(c.Request.Context(), ginutils.ParamsToMap(c.Params)))
	})

	router.Path("/").Methods(http.MethodGet).Handler(http.RedirectHandler(viper.GetString("pipeline.uipath"), http.StatusTemporaryRedirect))
	engine.GET("/", gin.WrapH(router))

	base := engine.Group(basePath)
	router = router.PathPrefix(basePath).Subrouter()

	// Frontend service
	{
		err := frontend.RegisterApp(
			router.PathPrefix("/frontend").Subrouter(),
			conf.Frontend,
			db,
			buildInfo,
			auth.UserExtractor{},
			commonLogger,
			errorHandler,
		)
		emperror.Panic(err)

		// TODO: refactor authentication middleware
		base.Any("frontend/notifications", gin.WrapH(router))

		// TODO: return 422 unprocessable entity instead of 404
		if conf.Frontend.Issue.Enabled {
			base.Any("frontend/issues", auth.Handler, gin.WrapH(router))
		}
	}

	base.GET("version", gin.WrapH(buildinfo.Handler(buildInfo)))

	auth.Install(engine)
	auth.StartTokenStoreGC(tokenStore)

	enforcer := auth.NewRbacEnforcer(organizationStore, commonLogger)
	authorizationMiddleware := ginauth.NewMiddleware(enforcer, basePath, errorHandler)

	dashboardAPI := dashboard.NewDashboardAPI(clusterManager, clusterGroupManager, logrusLogger, errorHandler)
	dgroup := base.Group(path.Join("dashboard", "orgs"))
	dgroup.Use(auth.Handler)
	dgroup.Use(api.OrganizationMiddleware)
	dgroup.Use(authorizationMiddleware)
	dgroup.GET("/:orgid/clusters", dashboardAPI.GetDashboard)

	{
		// Cluster details dashboard
		dcGroup := dgroup.Group("/:orgid/clusters/:id")
		dcGroup.Use(cluster.NewClusterCheckMiddleware(clusterManager, errorHandler))
		dcGroup.GET("", dashboardAPI.GetClusterDashboard)
	}

	scmTokenStore := auth.NewSCMTokenStore(tokenStore, viper.GetBool("cicd.enabled"))

	domainAPI := api.NewDomainAPI(clusterManager, logrusLogger, errorHandler)
	organizationAPI := api.NewOrganizationAPI(organizationSyncer, auth.NewRefreshTokenStore(tokenStore))
	userAPI := api.NewUserAPI(db, scmTokenStore, logrusLogger, errorHandler)
	networkAPI := api.NewNetworkAPI(logrusLogger)

	switch viper.GetString(config.DNSBaseDomain) {
	case "", "example.com", "example.org":
		global.AutoDNSEnabled = false
	default:
		global.AutoDNSEnabled = true
	}

	var spotguideAPI *api.SpotguideAPI

	if viper.GetBool("cicd.enabled") {

		spotguidePlatformData := spotguide.PlatformData{
			AutoDNSEnabled: global.AutoDNSEnabled,
		}

		scmProvider := viper.GetString("cicd.scm")
		var scmToken string
		switch scmProvider {
		case "github":
			scmToken = viper.GetString("github.token")
		case "gitlab":
			scmToken = viper.GetString("gitlab.token")
		default:
			emperror.Panic(fmt.Errorf("Unknown SCM provider configured: %s", scmProvider))
		}

		scmFactory, err := scm.NewSCMFactory(scmProvider, scmToken, scmTokenStore)
		emperror.Panic(errors.WrapIf(err, "failed to create SCMFactory"))

		sharedSpotguideOrg, err := spotguide.EnsureSharedSpotguideOrganization(config.DB(), scmProvider, viper.GetString(config.SpotguideSharedLibraryGitHubOrganization))
		if err != nil {
			errorHandler.Handle(errors.WrapIf(err, "failed to create shared Spotguide organization"))
		}

		spotguideManager := spotguide.NewSpotguideManager(
			config.DB(),
			version,
			scmFactory,
			sharedSpotguideOrg,
			spotguidePlatformData,
		)

		// periodically sync shared spotguides
		if err := spotguide.ScheduleScrapingSharedSpotguides(workflowClient); err != nil {
			errorHandler.Handle(errors.WrapIf(err, "failed to schedule syncing shared spotguides"))
		}

		spotguideAPI = api.NewSpotguideAPI(logrusLogger, errorHandler, spotguideManager)
	}

	v1 := base.Group("api/v1")
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	{
		apiRouter.NotFoundHandler = problems.StatusProblemHandler(problems.NewStatusProblem(http.StatusNotFound))
		apiRouter.MethodNotAllowedHandler = problems.StatusProblemHandler(problems.NewStatusProblem(http.StatusMethodNotAllowed))

		v1.Use(auth.Handler)
		capdriver.RegisterHTTPHandler(mapCapabilities(conf), emperror.MakeContextAware(errorHandler), v1)
		v1.GET("/securityscan", api.SecurityScanEnabled)
		v1.GET("/me", userAPI.GetCurrentUser)
		v1.PATCH("/me", userAPI.UpdateCurrentUser)

		endpointMiddleware := []endpoint.Middleware{
			correlation.Middleware(),
		}

		httpServerOptions := []kithttp.ServerOption{
			kithttp.ServerErrorHandler(emperror.MakeContextAware(errorHandler)),
			kithttp.ServerErrorEncoder(appkit.ProblemErrorEncoder),
			kithttp.ServerBefore(correlation.HTTPToContext()),
		}

		orgs := v1.Group("/orgs")
		orgRouter := apiRouter.PathPrefix("/orgs/{orgId}").Subrouter()
		{
			orgs.Use(api.OrganizationMiddleware)
			orgs.Use(authorizationMiddleware)

			if viper.GetBool("cicd.enabled") {
				spotguides := orgs.Group("/:orgid/spotguides")
				spotguideAPI.Install(spotguides)
			}

			orgs.GET("/:orgid/domain", domainAPI.GetDomain)
			orgs.POST("/:orgid/clusters", clusterAPI.CreateCluster)
			// v1.GET("/status", api.Status)
			orgs.GET("/:orgid/clusters", clusterAPI.GetClusters)

			// cluster API
			cRouter := orgs.Group("/:orgid/clusters/:id")
			clusterRouter := orgRouter.PathPrefix("/clusters/{clusterId}").Subrouter()
			{
				logger := commonadapter.NewLogger(logger) // TODO: make this a context aware logger

				cRouter.Use(cluster.NewClusterCheckMiddleware(clusterManager, errorHandler))

				cRouter.GET("", clusterAPI.GetCluster)
				cRouter.GET("/pods", api.GetPodDetails)
				cRouter.GET("/bootstrap", clusterAPI.GetBootstrapInfo)
				cRouter.PUT("", clusterAPI.UpdateCluster)

				cRouter.PUT("/posthooks", clusterAPI.ReRunPostHooks)
				cRouter.POST("/secrets", api.InstallSecretsToCluster)
				cRouter.POST("/secrets/:secretName", api.InstallSecretToCluster)
				cRouter.PATCH("/secrets/:secretName", api.MergeSecretInCluster)
				cRouter.Any("/proxy/*path", clusterAPI.ProxyToCluster)
				cRouter.DELETE("", clusterAPI.DeleteCluster)
				cRouter.HEAD("", clusterAPI.ClusterCheck)
				cRouter.GET("/config", api.GetClusterConfig)
				cRouter.GET("/apiendpoint", api.GetApiEndpoint)
				cRouter.GET("/nodes", api.GetClusterNodes)
				cRouter.GET("/endpoints", api.MakeEndpointLister(logger).ListEndpoints)
				cRouter.GET("/secrets", api.ListClusterSecrets)
				cRouter.GET("/deployments", api.ListDeployments)
				cRouter.POST("/deployments", api.CreateDeployment)
				cRouter.GET("/deployments/:name", api.GetDeployment)
				cRouter.GET("/deployments/:name/resources", api.GetDeploymentResources)
				cRouter.GET("/hpa", api.GetHpaResource)
				cRouter.PUT("/hpa", api.PutHpaResource)
				cRouter.DELETE("/hpa", api.DeleteHpaResource)
				cRouter.HEAD("/deployments", api.GetTillerStatus)
				cRouter.DELETE("/deployments/:name", api.DeleteDeployment)
				cRouter.PUT("/deployments/:name", api.UpgradeDeployment)
				cRouter.HEAD("/deployments/:name", api.HelmDeploymentStatus)

				cRouter.GET("/images", api.ListImages)
				cRouter.GET("/images/:imageDigest/deployments", api.GetImageDeployments)
				cRouter.GET("/deployments/:name/images", api.GetDeploymentImages)
			}

			clusterSecretStore := clustersecret.NewStore(
				clustersecretadapter.NewClusterManagerAdapter(clusterManager),
				clustersecretadapter.NewSecretStore(secret.Store),
			)

			// Cluster Feature API
			{
				logger := commonadapter.NewLogger(logger) // TODO: make this a context aware logger
				featureRepository := clusterfeatureadapter.NewGormFeatureRepository(db, logger)
				clusterGetter := clusterfeatureadapter.MakeClusterGetter(clusterManager)
				orgDomainService := featureDns.NewOrgDomainService(clusterGetter, dnsSvc, logger)
				secretStore := commonadapter.NewSecretStore(secret.Store, commonadapter.OrgIDContextExtractorFunc(auth.GetCurrentOrganizationID))
				featureManagers := []clusterfeature.FeatureManager{
					featureDns.MakeFeatureManager(clusterGetter, logger, orgDomainService),
					securityscan.MakeFeatureManager(logger),
				}

				if conf.Cluster.Vault.Enabled {
					featureManagers = append(featureManagers, featureVault.MakeFeatureManager(clusterGetter, secretStore, conf.Cluster.Vault.Managed.Enabled, logger))
				}

				if conf.Cluster.Monitoring.Enabled {
					endpointManager := endpoints.NewEndpointManager(logger)
					helmService := helm.NewHelmService(helmadapter.NewClusterService(clusterManager), logger)
					monitoringConfig := featureMonitoring.NewFeatureConfiguration()
					featureManagers = append(featureManagers, featureMonitoring.MakeFeatureManager(clusterGetter, secretStore, endpointManager, helmService, monitoringConfig, logger))
				}

				if conf.Cluster.SecurityScan.Enabled {
					customAnchoreConfigProvider := securityscan.NewCustomAnchoreConfigProvider(
						featureRepository,
						secretStore,
						logger,
					)

					configProvider := anchore2.ConfigProviderChain{customAnchoreConfigProvider}

					if conf.Cluster.SecurityScan.Anchore.Enabled {
						configProvider = append(configProvider, securityscan.NewClusterAnchoreConfigProvider(
							conf.Cluster.SecurityScan.Anchore.Endpoint,
							securityscanadapter.NewUserNameGenerator(securityscanadapter.NewClusterService(clusterManager)),
							securityscanadapter.NewUserSecretStore(secretStore),
						))
					}

					imgScanSvc := anchore.NewImageScannerService(configProvider, logger)
					imageScanHandler := api.NewImageScanHandler(commonClusterGetter, imgScanSvc, logger)

					policySvc := anchore.NewPolicyService(configProvider, logger)
					policyHandler := api.NewPolicyHandler(commonClusterGetter, policySvc, logger)

					secErrorHandler := emperror.MakeContextAware(errorHandler)
					securityApiHandler := api.NewSecurityApiHandlers(commonClusterGetter, secErrorHandler, logger)

					anchoreProxy := api.NewAnchoreProxy(basePath, configProvider, secErrorHandler, logger)
					proxyHandler := anchoreProxy.Proxy()

					// forthcoming endpoint for all requests proxied to Anchore
					cRouter.Any("/anchore/*proxyPath", proxyHandler)

					// these are cluster resources
					cRouter.GET("/scanlog", securityApiHandler.ListScanLogs)
					cRouter.GET("/scanlog/:releaseName", securityApiHandler.GetScanLogs)

					cRouter.GET("/whitelists", securityApiHandler.GetWhiteLists)
					cRouter.POST("/whitelists", securityApiHandler.CreateWhiteList)
					cRouter.DELETE("/whitelists/:name", securityApiHandler.DeleteWhiteList)

					cRouter.GET("/policies", policyHandler.ListPolicies)
					cRouter.GET("/policies/:policyId", policyHandler.GetPolicy)
					cRouter.POST("/policies", policyHandler.CreatePolicy)
					cRouter.PUT("/policies/:policyId", policyHandler.UpdatePolicy)
					cRouter.DELETE("/policies/:policyId", policyHandler.DeletePolicy)

					cRouter.POST("/imagescan", imageScanHandler.ScanImages)
					cRouter.GET("/imagescan/:imagedigest", imageScanHandler.GetScanResult)
					cRouter.GET("/imagescan/:imagedigest/vuln", imageScanHandler.GetImageVulnerabilities)
				}

				featureManagerRegistry := clusterfeature.MakeFeatureManagerRegistry(featureManagers)
				featureOperationDispatcher := clusterfeatureadapter.MakeCadenceFeatureOperationDispatcher(workflowClient, logger)
				service := clusterfeature.MakeFeatureService(featureOperationDispatcher, featureManagerRegistry, featureRepository, logger)
				endpoints := clusterfeaturedriver.MakeEndpoints(
					service,
					kitxendpoint.Chain(endpointMiddleware...),
					appkit.EndpointLogger(commonLogger),
				)

				clusterfeaturedriver.RegisterHTTPHandlers(
					endpoints,
					clusterRouter.PathPrefix("/features").Subrouter(),
					errorHandler,
					kitxhttp.ServerOptions(httpServerOptions),
					kithttp.ServerErrorHandler(emperror.MakeContextAware(errorHandler)),
				)

				cRouter.Any("/features", gin.WrapH(router))
				cRouter.Any("/features/:featureName", gin.WrapH(router))
			}

			// ClusterGroupAPI
			cgroupsAPI := cgroupAPI.NewAPI(clusterGroupManager, deploymentManager, logrusLogger, errorHandler)
			cgroupsAPI.AddRoutes(orgs.Group("/:orgid/clustergroups"))

			cRouter.GET("/nodepools/labels", nplsApi.GetNodepoolLabelSets)
			cRouter.POST("/nodepools/labels", nplsApi.SetNodepoolLabelSets)

			namespaceAPI := namespace.NewAPI(commonClusterGetter, errorHandler)
			namespaceAPI.RegisterRoutes(cRouter.Group("/namespaces/:namespace"))

			pkeGroup := cRouter.Group("/pke")

			leaderRepository, err := pke.NewVaultLeaderRepository()
			emperror.Panic(errors.WrapIf(err, "failed to create Vault leader repository"))

			pkeAPI := pke.NewAPI(
				commonClusterGetter,
				errorHandler,
				auth.NewClusterTokenGenerator(tokenManager, tokenStore),
				externalBaseURL,
				workflowClient,
				leaderRepository,
			)
			pkeAPI.RegisterRoutes(pkeGroup)

			clusterAuthService, err := intClusterAuth.NewDexClusterAuthService(clusterSecretStore)
			emperror.Panic(errors.WrapIf(err, "failed to create DexClusterAuthService"))

			pipelineExternalURL, err := url.Parse(externalBaseURL)
			emperror.Panic(errors.WrapIf(err, "failed to parse pipeline externalBaseURL"))

			pipelineExternalURL.Path = "/auth/dex/cluster/callback"

			clusterAuthAPI, err := api.NewClusterAuthAPI(
				commonClusterGetter,
				clusterAuthService,
				conf.Auth.Token.SigningKey,
				oidcIssuerURL,
				viper.GetBool(config.OIDCIssuerInsecure),
				pipelineExternalURL.String(),
			)
			emperror.Panic(errors.WrapIf(err, "failed to create ClusterAuthAPI"))

			clusterAuthAPI.RegisterRoutes(cRouter, engine)

			orgs.GET("/:orgid/helm/repos", api.HelmReposGet)
			orgs.POST("/:orgid/helm/repos", api.HelmReposAdd)
			orgs.PUT("/:orgid/helm/repos/:name", api.HelmReposModify)
			orgs.PUT("/:orgid/helm/repos/:name/update", api.HelmReposUpdate)
			orgs.DELETE("/:orgid/helm/repos/:name", api.HelmReposDelete)
			orgs.GET("/:orgid/helm/charts", api.HelmCharts)
			orgs.GET("/:orgid/helm/chart/:reponame/:name", api.HelmChart)
			orgs.GET("/:orgid/secrets", api.ListSecrets)
			orgs.GET("/:orgid/secrets/:id", api.GetSecret)
			orgs.POST("/:orgid/secrets", api.AddSecrets)
			orgs.PUT("/:orgid/secrets/:id", api.UpdateSecrets)
			orgs.DELETE("/:orgid/secrets/:id", api.DeleteSecrets)
			orgs.GET("/:orgid/secrets/:id/validate", api.ValidateSecret)
			orgs.GET("/:orgid/secrets/:id/tags", api.GetSecretTags)
			orgs.PUT("/:orgid/secrets/:id/tags/*tag", api.AddSecretTag)
			orgs.DELETE("/:orgid/secrets/:id/tags/*tag", api.DeleteSecretTag)
			orgs.GET("/:orgid/users", userAPI.GetUsers)
			orgs.GET("/:orgid/users/:id", userAPI.GetUsers)

			orgs.GET("/:orgid/buckets", api.ListAllBuckets)
			orgs.POST("/:orgid/buckets", api.CreateBucket)
			orgs.HEAD("/:orgid/buckets/:name", api.CheckBucket)
			orgs.GET("/:orgid/buckets/:name", api.GetBucket)
			orgs.DELETE("/:orgid/buckets/:name", api.DeleteBucket)

			orgs.GET("/:orgid/networks", networkAPI.ListVPCNetworks)
			orgs.GET("/:orgid/networks/:id/subnets", networkAPI.ListVPCSubnets)
			orgs.GET("/:orgid/networks/:id/routeTables", networkAPI.ListRouteTables)

			orgs.GET("/:orgid/azure/resourcegroups", api.GetResourceGroups)
			orgs.POST("/:orgid/azure/resourcegroups", api.AddResourceGroups)

			{
				secretStore := googleadapter.NewSecretStore(secretStore)
				clientFactory := google.NewClientFactory(secretStore)

				service := googleproject.NewService(clientFactory)
				endpoints := googleprojectdriver.TraceEndpoints(googleprojectdriver.MakeEndpoints(
					service,
					kitxendpoint.Chain(endpointMiddleware...),
					appkit.EndpointLogger(commonLogger),
				))

				googleprojectdriver.RegisterHTTPHandlers(
					endpoints,
					orgRouter.PathPrefix("/cloud/google/projects").Subrouter(),
					kitxhttp.ServerOptions(httpServerOptions),
					kithttp.ServerErrorHandler(emperror.MakeContextAware(errorHandler)),
				)

				orgs.Any("/:orgid/cloud/google/projects", gin.WrapH(router))

				googleprojectdriver.RegisterHTTPHandlers(
					endpoints,
					orgRouter.PathPrefix("/google/projects").Subrouter(),
					kitxhttp.ServerOptions(httpServerOptions),
					kithttp.ServerErrorHandler(emperror.MakeContextAware(errorHandler)),
				)

				orgs.Any("/:orgid/google/projects", gin.WrapH(router))
			}

			orgs.GET("/:orgid", organizationAPI.GetOrganizations)
			orgs.DELETE("/:orgid", organizationAPI.DeleteOrganization)
		}
		v1.GET("/orgs", organizationAPI.GetOrganizations)
		v1.PUT("/orgs", organizationAPI.SyncOrganizations)

		{
			logger := commonLogger.WithFields(map[string]interface{}{"module": "auth"})
			errorHandler := emperror.MakeContextAware(emperror.WithDetails(errorHandler, "module", "auth"))

			service := token.NewService(
				auth.UserExtractor{},
				tokenadapter.NewBankVaultsStore(tokenStore),
				tokenGenerator,
			)
			service = tokendriver.AuthorizationMiddleware(auth.NewAuthorizer(db, organizationStore))(service)

			endpoints := tokendriver.TraceEndpoints(tokendriver.MakeEndpoints(
				service,
				kitxendpoint.Chain(endpointMiddleware...),
				appkit.EndpointLogger(logger),
			))

			tokendriver.RegisterHTTPHandlers(
				endpoints,
				apiRouter.PathPrefix("/tokens").Subrouter(),
				kitxhttp.ServerOptions(httpServerOptions),
				kithttp.ServerErrorHandler(errorHandler),
			)

			v1.Any("/tokens", gin.WrapH(router))
			v1.Any("/tokens/*path", gin.WrapH(router))
		}

		{
			logger := commonLogger.WithFields(map[string]interface{}{"module": "secret"})
			errorHandler := emperror.MakeContextAware(emperror.WithDetails(errorHandler, "module", "secret"))

			service := secrettype.NewTypeService()
			endpoints := secrettypedriver.TraceEndpoints(secrettypedriver.MakeEndpoints(
				service,
				kitxendpoint.Chain(endpointMiddleware...),
				appkit.EndpointLogger(logger),
			))

			secrettypedriver.RegisterHTTPHandlers(
				endpoints,
				apiRouter.PathPrefix("/secret-types").Subrouter(),
				kitxhttp.ServerOptions(httpServerOptions),
				kithttp.ServerErrorHandler(errorHandler),
			)

			v1.Any("/secret-types", gin.WrapH(router))
			v1.Any("/secret-types/*path", gin.WrapH(router))

			// Compatibility routes
			{
				secrettypedriver.RegisterHTTPHandlers(
					endpoints,
					apiRouter.PathPrefix("/allowed/secrets").Subrouter(),
					kitxhttp.ServerOptions(httpServerOptions),
					kithttp.ServerErrorHandler(errorHandler),
				)

				v1.GET("/allowed/secrets", gin.WrapH(router))
				v1.GET("/allowed/secrets/*path", gin.WrapH(router))
			}
		}

		backups.AddRoutes(orgs.Group("/:orgid/clusters/:id/backups"))
		backupservice.AddRoutes(orgs.Group("/:orgid/clusters/:id/backupservice"))
		restores.AddRoutes(orgs.Group("/:orgid/clusters/:id/restores"))
		schedules.AddRoutes(orgs.Group("/:orgid/clusters/:id/schedules"))
		buckets.AddRoutes(orgs.Group("/:orgid/backupbuckets"))
		backups.AddOrgRoutes(orgs.Group("/:orgid/backups"), clusterManager)
	}

	arkEvents.NewClusterEventHandler(arkEvents.NewClusterEvents(clusterEventBus), config.DB(), logrusLogger)
	if viper.GetBool(config.ARKSyncEnabled) {
		go arkSync.RunSyncServices(
			context.Background(),
			config.DB(),
			arkClusterManager.New(clusterManager),
			platformlog.NewLogrusLogger(platformlog.Config{
				Level:  viper.GetString(config.ARKLogLevel),
				Format: viper.GetString(conf.Log.Format),
			}).WithField("subsystem", "ark"),
			errorHandler,
			viper.GetDuration(config.ARKBucketSyncInterval),
			viper.GetDuration(config.ARKRestoreSyncInterval),
			viper.GetDuration(config.ARKBackupSyncInterval),
		)
	}

	base.GET("api", api.MetaHandler(engine, basePath+"/api"))

	internalBindAddr := viper.GetString("pipeline.internalBindAddr")
	logger.Info("Pipeline internal API listening", map[string]interface{}{"address": "http://" + internalBindAddr})

	go createInternalAPIRouter(skipPaths, db, basePath, clusterAPI, logger, logrusLogger).Run(internalBindAddr) // nolint: errcheck

	bindAddr := viper.GetString("pipeline.bindaddr")
	if port := viper.GetInt("pipeline.listenport"); port != 0 { // TODO: remove deprecated option
		host := strings.Split(bindAddr, ":")[0]
		bindAddr = fmt.Sprintf("%s:%d", host, port)
		logger.Warn(fmt.Sprintf(
			"pipeline.listenport=%d setting is deprecated! Falling back to pipeline.bindaddr=%s",
			port,
			bindAddr,
		))
	}
	certFile, keyFile := viper.GetString("pipeline.certfile"), viper.GetString("pipeline.keyfile")
	if certFile != "" && keyFile != "" {
		logger.Info("Pipeline API listening", map[string]interface{}{"address": "https://" + bindAddr})
		_ = engine.RunTLS(bindAddr, certFile, keyFile)
	} else {
		logger.Info("Pipeline API listening", map[string]interface{}{"address": "http://" + bindAddr})
		_ = engine.Run(bindAddr)
	}
}

func createInternalAPIRouter(skipPaths []string, db *gorm.DB, basePath string, clusterAPI *api.ClusterAPI, logger logur.Logger, logrusLogger logrus.FieldLogger) *gin.Engine {
	// Initialise Gin router for Internal API
	internalRouter := gin.New()
	internalRouter.Use(correlationid.Middleware())
	internalRouter.Use(ginlog.Middleware(logrusLogger, skipPaths...))
	internalRouter.Use(gin.Recovery())
	if viper.GetBool("audit.enabled") {
		logger.Info("Audit enabled, installing Gin audit middleware to internal router")
		internalRouter.Use(audit.LogWriter(skipPaths, viper.GetStringSlice("audit.headers"), db, logrusLogger))
	}
	internalGroup := internalRouter.Group(path.Join(basePath, "api", "v1/", "orgs"))
	internalGroup.Use(auth.InternalUserHandler)
	internalGroup.Use(api.OrganizationMiddleware)
	internalGroup.GET("/:orgid/clusters/:id/nodepools", api.GetNodePools)
	internalGroup.PUT("/:orgid/clusters/:id/nodepools", clusterAPI.UpdateNodePools)
	return internalRouter
}
