// Code generated by mga tool. DO NOT EDIT.
package clusterfeaturedriver

import (
	"github.com/banzaicloud/pipeline/internal/clusterfeature"
	"github.com/go-kit/kit/endpoint"
	kitoc "github.com/go-kit/kit/tracing/opencensus"
	kitxendpoint "github.com/sagikazarmark/kitx/endpoint"
)

// Endpoints collects all of the endpoints that compose the underlying service. It's
// meant to be used as a helper struct, to collect all of the endpoints into a
// single parameter.
type Endpoints struct {
	Activate   endpoint.Endpoint
	Deactivate endpoint.Endpoint
	Details    endpoint.Endpoint
	List       endpoint.Endpoint
	Update     endpoint.Endpoint
}

// MakeEndpoints returns an Endpoints struct where each endpoint invokes
// the corresponding method on the provided service.
func MakeEndpoints(service clusterfeature.Service, middleware ...endpoint.Middleware) Endpoints {
	mw := kitxendpoint.Chain(middleware...)

	return Endpoints{
		Activate:   mw(MakeActivateEndpoint(service)),
		Deactivate: mw(MakeDeactivateEndpoint(service)),
		Details:    mw(MakeDetailsEndpoint(service)),
		List:       mw(MakeListEndpoint(service)),
		Update:     mw(MakeUpdateEndpoint(service)),
	}
}

// TraceEndpoints returns an Endpoints struct where each endpoint is wrapped with a tracing middleware.
func TraceEndpoints(endpoints Endpoints) Endpoints {
	return Endpoints{
		Activate:   kitoc.TraceEndpoint("clusterfeature.Activate")(endpoints.Activate),
		Deactivate: kitoc.TraceEndpoint("clusterfeature.Deactivate")(endpoints.Deactivate),
		Details:    kitoc.TraceEndpoint("clusterfeature.Details")(endpoints.Details),
		List:       kitoc.TraceEndpoint("clusterfeature.List")(endpoints.List),
		Update:     kitoc.TraceEndpoint("clusterfeature.Update")(endpoints.Update),
	}
}
