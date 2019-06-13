package utils

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricKubeClientRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_kube_client_requests_total",
		Help: "Counter incremented for each kube client request.",
	},
		[]string{"controller", "method", "resource", "remote"},
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
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &ControllerMetricsTripper{
			RoundTripper: rt,
			Controller:   ctrlrName,
			Remote:       false, // this is a local client
		}
	}

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
		StatusClient: c,
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
	path, err := parsePath(req.URL.Path)
	if err == nil {
		metricKubeClientRequests.WithLabelValues(cmt.Controller, req.Method, path, remoteStr).Inc()
	}
	// Call the nested RoundTripper.
	resp, err := cmt.RoundTripper.RoundTrip(req)
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
