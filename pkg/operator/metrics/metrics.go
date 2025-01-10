package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	ControllerName = hivev1.MetricsControllerName
)

var (
	// Capture the various conditions set on the HiveConfig
	metricHiveConfigConditions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_hiveconfig_conditions",
		Help: "HiveConfig with conditions.",
	}, []string{"condition", "reason"})

	// metricControllerReconcileTime tracks the length of time our reconcile loops take. controller-runtime
	// technically tracks this for us, but due to bugs currently also includes time in the queue, which leads to
	// extremely strange results. For now, track our own metric.
	metricControllerReconcileTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_operator_reconcile_seconds",
			Help:    "Distribution of the length of time each controllers reconcile loop takes.",
			Buckets: []float64{0.001, 0.01, 0.1, 1, 10, 30, 60, 120},
		},
		[]string{"controller", "outcome"},
	)
)

// ReconcileOutcome is used in controller "reconcile complete" log entries, and the metricControllerReconcileTime
// above for controllers where we would like to monitor performance for different types of Reconcile outcomes. To help with
// prometheus cardinality this set of outcomes should be kept small and only used for coarse and very high value
// categories. Controllers must report an outcome but can report unspecified if this categorization is not needed.
type ReconcileOutcome string

const (
	ReconcileOutcomeUnspecified        ReconcileOutcome = "unspecified"
	ReconcileOutcomeNoOp               ReconcileOutcome = "no-op"
	ReconcileOutcomeSkippedSync        ReconcileOutcome = "skipped-sync"
	ReconcileOutcomeFullSync           ReconcileOutcome = "full-sync"
	ReconcileOutcomeClusterSyncCreated ReconcileOutcome = "clustersync-created"
)

func init() {
	metrics.Registry.MustRegister(metricHiveConfigConditions)
	metrics.Registry.MustRegister(metricControllerReconcileTime)
}

// Add creates a new metrics Calculator and adds it to the Manager.
func Add(mgr manager.Manager) error {
	mc := &Calculator{
		Client:   mgr.GetClient(),
		Interval: 2 * time.Minute,
	}
	err := mgr.Add(mc)
	if err != nil {
		return err
	}
	return nil
}

// Calculator runs in a goroutine and periodically calculates and publishes
// Prometheus metrics which will be exposed at our /metrics endpoint. Note that this is not
// a standard controller watching Kube resources, it runs periodically and then goes to sleep.
//
// This should be used for metrics which do not fit well into controller reconcile loops,
// things that are calculated globally rather than metrics related to specific reconciliations.
type Calculator struct {
	Client client.Client

	// Interval is the length of time we sleep between metrics calculations.
	Interval time.Duration
}

// Start begins the metrics calculation loop.
func (mc *Calculator) Start(ctx context.Context) error {
	log.Info("started metrics calculator goroutine")

	// Run forever, sleep at the end:
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		mcLog := log.WithField("controller", "metrics")
		recobsrv := NewReconcileObserver(ControllerName, mcLog)
		defer recobsrv.ObserveControllerReconcileTime()

		mcLog.Info("calculating metrics for all Hive")
		// Load all ClusterDeployments so we can accumulate facts about them.
		hiveConfig := &hivev1.HiveConfig{}

		err := mc.Client.Get(context.TODO(), types.NamespacedName{Name: constants.HiveConfigName}, hiveConfig)
		if err != nil {
			mcLog.WithError(err).Error("error reading HiveConfig")
			return
		}

		// Clear the metricHiveConfigConditions for Ready condition before setting it, to ensure we are not reporting stale values during migration
		metricHiveConfigConditions.DeletePartialMatch(map[string]string{
			"condition": string(hivev1.HiveReadyCondition),
		})
		mc.setHiveConfigMetrics(hiveConfig)

	}, mc.Interval)

	return nil
}

func (mc *Calculator) setHiveConfigMetrics(config *hivev1.HiveConfig) {
	for _, condition := range config.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			metricHiveConfigConditions.WithLabelValues(string(condition.Type), condition.Reason).Set(1)
		} else {
			metricHiveConfigConditions.WithLabelValues(string(condition.Type), condition.Reason).Set(0)
		}
	}
}

// ReconcileObserver is used to track, log, and report metrics for controller reconcile time and outcome. Each
// controller should instantiate one near the start of the reconcile loop, and defer a call to
// ObserveControllerReconcileTime.
type ReconcileObserver struct {
	startTime      time.Time
	controllerName hivev1.ControllerName
	logger         log.FieldLogger
	outcome        ReconcileOutcome
}

func NewReconcileObserver(controllerName hivev1.ControllerName, logger log.FieldLogger) *ReconcileObserver {
	return &ReconcileObserver{
		startTime:      time.Now(),
		controllerName: controllerName,
		logger:         logger,
		outcome:        ReconcileOutcomeUnspecified,
	}
}

func (ro *ReconcileObserver) ObserveControllerReconcileTime() {
	dur := time.Since(ro.startTime)
	metricControllerReconcileTime.WithLabelValues(string(ro.controllerName), string(ro.outcome)).Observe(dur.Seconds())
	fields := log.Fields{"elapsedMillis": dur.Milliseconds(), "outcome": ro.outcome}
	// Add a log field to categorize request duration into buckets. We can't easily query > in Kibana in
	// OpenShift deployments due to logging limitations, so these constant buckets give us a small number of
	// constant search strings we can use to identify slow reconcile loops.
	for _, bucket := range elapsedDurationBuckets {
		if dur >= bucket {
			fields["elapsedMillisGT"] = bucket.Milliseconds()
			break
		}
	}
	ro.logger.WithFields(fields).Info("reconcile complete")
}

var elapsedDurationBuckets = []time.Duration{2 * time.Minute, time.Minute, 30 * time.Second, 10 * time.Second, 5 * time.Second, time.Second, 0}
