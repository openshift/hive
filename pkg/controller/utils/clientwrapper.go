package utils

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

var (
	metricKubeClientRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_kube_client_requests_total",
		Help: "Counter incremented for each kube client request.",
	},
		[]string{"controller", "method", "resource", "remote", "status"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricKubeClientRequests)
}

// NewClientWithMetricsOrDie creates a new controller-runtime client with a wrapper which increments
// metrics for requests by controller name, HTTP method, URL path, and whether or not the request was
// to a remote cluster.. The client will re-use the managers cache. This should be used in
// all Hive controllers.
func NewClientWithMetricsOrDie(mgr manager.Manager, ctrlrName string) client.Client {
	// Copy the rest config as we want our round trippers to be controller specific.
	cfg := rest.CopyConfig(mgr.GetConfig())
	AddControllerMetricsTransportWrapper(cfg, ctrlrName, false)

	options := client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}
	c, err := client.New(cfg, options)
	if err != nil {
		log.WithError(err).Fatal("unable to initialize metrics wrapped client")
	}

	return &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  mgr.GetCache(),
			ClientReader: c,
		},
		Writer:       c,
		StatusClient: &hiveStatusClient{c},
	}
}

// AddControllerMetricsTransportWrapper adds a transport wrapper to the given rest config which
// exposes metrics based on the requests being made.
func AddControllerMetricsTransportWrapper(cfg *rest.Config, controllerName string, remote bool) {
	// If the restConfig already has a transport wrapper, wrap it.
	if cfg.WrapTransport != nil {
		origFunc := cfg.WrapTransport
		cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			return &ControllerMetricsTripper{
				RoundTripper: origFunc(rt),
				Controller:   controllerName,
				Remote:       remote,
			}
		}
	}

	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &ControllerMetricsTripper{
			RoundTripper: rt,
			Controller:   controllerName,
			Remote:       remote,
		}
	}
}

// ControllerMetricsTripper is a RoundTripper implementation which tracks our metrics for client requests.
type ControllerMetricsTripper struct {
	http.RoundTripper
	Controller string
	Remote     bool
}

// RoundTrip implements the http RoundTripper interface.
func (cmt *ControllerMetricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	remoteStr := "false"
	if cmt.Remote {
		remoteStr = "true"
	}
	path, pathErr := parsePath(req.URL.Path)
	// Call the nested RoundTripper.
	resp, err := cmt.RoundTripper.RoundTrip(req)
	if err == nil && pathErr == nil {
		metricKubeClientRequests.WithLabelValues(cmt.Controller, req.Method, path, remoteStr, resp.Status).Inc()
	}

	return resp, err
}

// parsePath returns a group/version/resource string from the given path. Used to avoid per cluster metrics
// for cardinality reason. We do not return metrics for all paths however and will return an error if we're unable
// to parse a resource from a path.
func parsePath(path string) (string, error) {
	tokens := strings.Split(path[1:], "/")
	if tokens[0] == "api" {
		// Handle core resources:
		if len(tokens) == 3 || len(tokens) == 4 {
			return strings.Join([]string{"core", tokens[1], tokens[2]}, "/"), nil
		}
		// Handle operators on direct namespaced resources:
		if len(tokens) > 4 && tokens[2] == "namespaces" {
			return strings.Join([]string{"core", tokens[1], tokens[4]}, "/"), nil
		}
	} else if tokens[0] == "apis" {
		// Handle resources with apigroups:
		if len(tokens) == 4 || len(tokens) == 5 {
			return strings.Join([]string{tokens[1], tokens[2], tokens[3]}, "/"), nil
		}
		if len(tokens) > 5 && tokens[3] == "namespaces" {
			return strings.Join([]string{tokens[1], tokens[2], tokens[5]}, "/"), nil
		}

	}
	return "", fmt.Errorf("unable to parse path for client metrics: %s", path)
}

// hiveStatusClient is a wrapper around a client.StatusClient that returns
// a hiveStatusWriter wrapped around the client.StatusWriter returned by the
// client.StatusClient.
type hiveStatusClient struct {
	client.StatusClient
}

// Status implements StatusClient.Status
func (sc *hiveStatusClient) Status() client.StatusWriter {
	return &hiveStatusWriter{sc.StatusClient.Status()}
}

// hiveStatusWriter is a wrapper around a client.StatusWrapper that massages
// the statuses of clustedeployments so that they pass validation.
type hiveStatusWriter struct {
	client.StatusWriter
}

// Update implements StatusWriter.Update
func (sw *hiveStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOptionFunc) error {
	switch t := obj.(type) {
	case *hivev1.ClusterDeployment:
		// Fetching clusterVersion object can result in nil clusterVersion.Status.AvailableUpdates
		// Place an empty list if needed to satisfy the object validation.
		if t.Status.ClusterVersionStatus.AvailableUpdates == nil {
			t.Status.ClusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
		}
	}
	return sw.StatusWriter.Update(ctx, obj, opts...)
}
