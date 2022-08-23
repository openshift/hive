package utils

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

var (
	metricKubeClientRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_kube_client_requests_total",
		Help: "Counter incremented for each kube client request.",
	},
		[]string{"controller", "method", "resource", "remote", "status"},
	)
	metricKubeClientRequestSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_kube_client_request_seconds",
		Help:    "Length of time for kubernetes client requests.",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120},
	},
		[]string{"controller", "method", "resource", "remote", "status"},
	)
	metricKubeClientRequestsCancelled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_kube_client_requests_cancelled_total",
		Help: "Counter incremented for each kube client request cancelled.",
	},
		[]string{"controller", "method", "resource", "remote"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricKubeClientRequests)
	metrics.Registry.MustRegister(metricKubeClientRequestSeconds)
	metrics.Registry.MustRegister(metricKubeClientRequestsCancelled)
}

// NewClientsWithMetricsOrDie returns two clients:
// - The data plane client. In scale mode, this points to the API service denoted by the data-plane-kubeconfig
//   secret in the target namespace of this deployment. In normal mode, it is identical to...
// - The control plane client, which points to the API service of the controller manager.
// ...and a bool indicating whether we are in scale mode (true) or not (false).
func NewClientsWithMetricsOrDie(mgr manager.Manager, ctrlrName hivev1.ControllerName, rateLimiter *flowcontrol.RateLimiter) (client.Client, client.Client, bool) {
	// Control plane client.
	// Copy the rest config as we want our round trippers to be controller specific.
	cpClient := newClientWithMetricsOrDie(rest.CopyConfig(mgr.GetConfig()), mgr, ctrlrName, rateLimiter)

	var dpClient client.Client

	// In scale mode?
	dpRestConfig := LoadDataPlaneKubeConfigOrDie()
	if dpRestConfig == nil {
		// Standard mode: the data plane and control plane are the same. NOTE: This is not a copy.
		return cpClient, cpClient, false
	}

	// Scale mode: build the client from the data plane REST config
	dpClient = newClientWithMetricsOrDie(dpRestConfig, mgr, ctrlrName, rateLimiter)

	return dpClient, cpClient, true
}

func LoadDataPlaneKubeConfigOrDie() *rest.Config {
	dpkcPath := os.Getenv(constants.DataPlaneKubeconfigEnvVar)
	if dpkcPath == "" {
		// Not in scale mode
		return nil
	}

	// Scale mode: load the mounted kubeconfig indicated by the env var
	kcData, err := os.ReadFile(dpkcPath)
	if err != nil {
		log.WithError(err).WithField("path", dpkcPath).Fatal("unable to read data plane kubeconfig file")
	}
	dpConfig, err := clientcmd.Load(kcData)
	if err != nil {
		log.WithError(err).Fatal("unable to load data plane kubeconfig")
	}
	dpRestConfig, err := clientcmd.NewDefaultClientConfig(*dpConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.WithError(err).Fatal("failed to parse data plane kubeconfig")
	}

	log.Info("Loaded data plane kubeconfig")
	return dpRestConfig
}

// newClientWithMetricsOrDie creates a new controller-runtime client with a wrapper which increments
// metrics for requests by controller name, HTTP method, URL path, and whether or not the request was
// to a remote cluster.. The client will re-use the managers cache. This should be used in
// all Hive controllers.
func newClientWithMetricsOrDie(cfg *rest.Config, mgr manager.Manager, ctrlrName hivev1.ControllerName, rateLimiter *flowcontrol.RateLimiter) client.Client {
	if rateLimiter != nil {
		cfg.RateLimiter = *rateLimiter
	}
	AddControllerMetricsTransportWrapper(cfg, ctrlrName, false)

	options := client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
	}
	c, err := client.New(cfg, options)
	if err != nil {
		log.WithError(err).Fatal("unable to initialize metrics wrapped client")
	}

	dc, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader: mgr.GetCache(),
		Client:      c,
	})
	if err != nil {
		log.WithError(err).Fatal("unable to initialize metrics wrapped client")
	}

	return dc
}

// AddControllerMetricsTransportWrapper adds a transport wrapper to the given rest config which
// exposes metrics based on the requests being made.
func AddControllerMetricsTransportWrapper(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool) {
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
	Controller hivev1.ControllerName
	Remote     bool
}

func (cmt *ControllerMetricsTripper) CancelRequest(req *http.Request) {
	// "CancelRequest not implemented by *utils.ControllerMetricsTripper" was seen during simulated scale
	// testing as the cluster was falling over. We will now implement this and track a metric for requests
	// being cancelled as this could prove valuable in tracking when we have a performance issue.
	remoteStr := strconv.FormatBool(cmt.Remote)
	path, _ := parsePath(req.URL.Path)
	metricKubeClientRequestsCancelled.WithLabelValues(cmt.Controller.String(), req.Method, path, remoteStr).Inc()
	log.WithFields(log.Fields{
		"controller": cmt.Controller.String(),
		"method":     req.Method,
		"path":       path,
		"remote":     remoteStr,
	}).Warn("cancelled request")
}

// RoundTrip implements the http RoundTripper interface.
func (cmt *ControllerMetricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()
	remoteStr := "false"
	if cmt.Remote {
		remoteStr = "true"
	}
	path, pathErr := parsePath(req.URL.Path)

	// Call the nested RoundTripper.
	resp, err := cmt.RoundTripper.RoundTrip(req)
	applyTime := metav1.Now().Sub(startTime)
	if err == nil && pathErr == nil {
		metricKubeClientRequests.WithLabelValues(cmt.Controller.String(), req.Method, path, remoteStr, resp.Status).Inc()
		metricKubeClientRequestSeconds.WithLabelValues(cmt.Controller.String(), req.Method, path, remoteStr, resp.Status).Observe(applyTime.Seconds())
		if applyTime >= 5*time.Second {
			log.WithFields(log.Fields{
				"controller":    cmt.Controller.String(),
				"method":        req.Method,
				"path":          path,
				"remote":        remoteStr,
				"status":        resp.Status,
				"elapsedMillis": applyTime.Milliseconds(), // millis for consistency with how we log controller reconcile time
			}).Warn("slow client request")
		}
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
