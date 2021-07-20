// Code generated for package assets by go-bindata DO NOT EDIT. (@generated)
// sources:
// config/clustersync/service.yaml
// config/clustersync/statefulset.yaml
// config/hiveadmission/apiservice.yaml
// config/hiveadmission/clusterdeployment-webhook.yaml
// config/hiveadmission/clusterimageset-webhook.yaml
// config/hiveadmission/clusterprovision-webhook.yaml
// config/hiveadmission/deployment.yaml
// config/hiveadmission/dnszones-webhook.yaml
// config/hiveadmission/hiveadmission_rbac_role.yaml
// config/hiveadmission/hiveadmission_rbac_role_binding.yaml
// config/hiveadmission/machinepool-webhook.yaml
// config/hiveadmission/selectorsyncset-webhook.yaml
// config/hiveadmission/service-account.yaml
// config/hiveadmission/service.yaml
// config/hiveadmission/syncset-webhook.yaml
// config/controllers/deployment.yaml
// config/controllers/hive_controllers_role.yaml
// config/controllers/hive_controllers_role_binding.yaml
// config/controllers/hive_controllers_serviceaccount.yaml
// config/controllers/service.yaml
// config/rbac/hive_admin_role.yaml
// config/rbac/hive_admin_role_binding.yaml
// config/rbac/hive_clusterpool_admin.yaml
// config/rbac/hive_frontend_role.yaml
// config/rbac/hive_frontend_role_binding.yaml
// config/rbac/hive_frontend_serviceaccount.yaml
// config/rbac/hive_reader_role.yaml
// config/rbac/hive_reader_role_binding.yaml
// config/configmaps/install-log-regexes-configmap.yaml
package assets

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _configClustersyncServiceYaml = []byte(`apiVersion: v1
kind: Service
metadata:
  name: hive-clustersync
  namespace: hive
  labels:
    control-plane: clustersync
    controller-tools.k8s.io: "1.0"
spec:
  selector:
    control-plane: clustersync
    controller-tools.k8s.io: "1.0"
  ports:
  - name: metrics
    port: 2112
    protocol: TCP
  # Expose 6060 for pprof data. Normally nothing listening here unless a developer has
  # compiled in pprof support. See Hive developer documentation for how to use.
  - name: profiling
    port: 6060
    protocol: TCP
`)

func configClustersyncServiceYamlBytes() ([]byte, error) {
	return _configClustersyncServiceYaml, nil
}

func configClustersyncServiceYaml() (*asset, error) {
	bytes, err := configClustersyncServiceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/clustersync/service.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configClustersyncStatefulsetYaml = []byte(`apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: hive-clustersync
  namespace: hive
  labels:
    control-plane: clustersync
    controller-tools.k8s.io: "1.0"
spec:
  selector:
    matchLabels:
      control-plane: clustersync
      controller-tools.k8s.io: "1.0"
  template:
    metadata:
      labels:
        control-plane: clustersync
        controller-tools.k8s.io: "1.0"
    spec:
      topologySpreadConstraints: # this forces the clustersync pods to be on separate nodes.
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            control-plane: clustersync
            controller-tools.k8s.io: "1.0"
      serviceAccount: hive-controllers
      serviceAccountName: hive-controllers
      containers:
      - name: clustersync
        resources:
          requests:
            cpu: 50m
            memory: 512Mi
        command:
        - "/opt/services/manager"
        args:
        - "--controllers"
        - "clustersync"
        envFrom:
        - configMapRef:
            name: hive-controllers-config
        env:
        - name: HIVE_NS
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: HIVE_CLUSTERSYNC_POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        - name: HIVE_SKIP_LEADER_ELECTION
          value: "true"
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /healthz
            port: 8080
            scheme: HTTP
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
        readinessProbe:
          failureThreshold: 3
          httpGet:
            path: /readyz
            port: 8080
            scheme: HTTP
          periodSeconds: 10
          successThreshold: 1
          timeoutSeconds: 1
`)

func configClustersyncStatefulsetYamlBytes() ([]byte, error) {
	return _configClustersyncStatefulsetYaml, nil
}

func configClustersyncStatefulsetYaml() (*asset, error) {
	bytes, err := configClustersyncStatefulsetYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/clustersync/statefulset.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionApiserviceYaml = []byte(`---
# register as aggregated apiserver; this has a number of benefits:
#
# - allows other kubernetes components to talk to the the admission webhook using the ` + "`" + `kubernetes.default.svc` + "`" + ` service
# - allows other kubernetes components to use their in-cluster credentials to communicate with the webhook
# - allows you to test the webhook using kubectl
# - allows you to govern access to the webhook using RBAC
# - prevents other extension API servers from leaking their service account tokens to the webhook
#
# for more information, see: https://kubernetes.io/blog/2018/01/extensible-admission-is-beta
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1.admission.hive.openshift.io
  annotations:
    service.alpha.openshift.io/inject-cabundle: "true"
spec:
  group: admission.hive.openshift.io
  groupPriorityMinimum: 1000
  versionPriority: 15
  service:
    name: hiveadmission
    namespace: hive
  version: v1
`)

func configHiveadmissionApiserviceYamlBytes() ([]byte, error) {
	return _configHiveadmissionApiserviceYaml, nil
}

func configHiveadmissionApiserviceYaml() (*asset, error) {
	bytes, err := configHiveadmissionApiserviceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/apiservice.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionClusterdeploymentWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: clusterdeploymentvalidators.admission.hive.openshift.io
webhooks:
- name: clusterdeploymentvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/clusterdeploymentvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    - DELETE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - clusterdeployments
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionClusterdeploymentWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionClusterdeploymentWebhookYaml, nil
}

func configHiveadmissionClusterdeploymentWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionClusterdeploymentWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/clusterdeployment-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionClusterimagesetWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: clusterimagesetvalidators.admission.hive.openshift.io
webhooks:
- name: clusterimagesetvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/clusterimagesetvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - clusterimagesets
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionClusterimagesetWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionClusterimagesetWebhookYaml, nil
}

func configHiveadmissionClusterimagesetWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionClusterimagesetWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/clusterimageset-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionClusterprovisionWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: clusterprovisionvalidators.admission.hive.openshift.io
webhooks:
- name: clusterprovisionvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/clusterprovisionvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - clusterprovisions
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionClusterprovisionWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionClusterprovisionWebhookYaml, nil
}

func configHiveadmissionClusterprovisionWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionClusterprovisionWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/clusterprovision-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionDeploymentYaml = []byte(`---
# to create the namespace-reservation-server
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: hive
  name: hiveadmission
  labels:
    app: hiveadmission
    hiveadmission: "true"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: hiveadmission
      hiveadmission: "true"
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      name: hiveadmission
      labels:
        app: hiveadmission
        hiveadmission: "true"
    spec:
      serviceAccountName: hiveadmission
      containers:
      - name: hiveadmission
        image: registry.ci.openshift.org/openshift/hive-v4.0:hive
        imagePullPolicy: Always
        command:
        - "/opt/services/hiveadmission"
        - "--secure-port=9443"
        - "--audit-log-path=-"
        - "--tls-cert-file=/var/serving-cert/tls.crt"
        - "--tls-private-key-file=/var/serving-cert/tls.key"
        - "--v=2"
        ports:
        - containerPort: 9443
          protocol: TCP
        envFrom:
        - configMapRef:
            name: hive-feature-gates
        volumeMounts:
        - mountPath: /var/serving-cert
          name: serving-cert
        readinessProbe:
          httpGet:
            path: /healthz
            port: 9443
            scheme: HTTPS
      volumes:
      - name: serving-cert
        secret:
          defaultMode: 420
          secretName: hiveadmission-serving-cert
`)

func configHiveadmissionDeploymentYamlBytes() ([]byte, error) {
	return _configHiveadmissionDeploymentYaml, nil
}

func configHiveadmissionDeploymentYaml() (*asset, error) {
	bytes, err := configHiveadmissionDeploymentYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/deployment.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionDnszonesWebhookYaml = []byte(`---
# register to intercept DNSZone object creates and updates
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: dnszonevalidators.admission.hive.openshift.io
webhooks:
- name: dnszonevalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/dnszonevalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - dnszones
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionDnszonesWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionDnszonesWebhookYaml, nil
}

func configHiveadmissionDnszonesWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionDnszonesWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/dnszones-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionHiveadmission_rbac_roleYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
  name: system:openshift:hive:hiveadmission
rules:
- apiGroups:
  - admission.hive.openshift.io
  resources:
  - dnszones
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - validatingwebhookconfigurations
  - mutatingwebhookconfigurations
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create

`)

func configHiveadmissionHiveadmission_rbac_roleYamlBytes() ([]byte, error) {
	return _configHiveadmissionHiveadmission_rbac_roleYaml, nil
}

func configHiveadmissionHiveadmission_rbac_roleYaml() (*asset, error) {
	bytes, err := configHiveadmissionHiveadmission_rbac_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/hiveadmission_rbac_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionHiveadmission_rbac_role_bindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hiveadmission-hive-hiveadmission
roleRef:
  kind: ClusterRole
  apiGroup: rbac.authorization.k8s.io
  name: system:openshift:hive:hiveadmission
subjects:
- kind: ServiceAccount
  namespace: hive
  name: hiveadmission
`)

func configHiveadmissionHiveadmission_rbac_role_bindingYamlBytes() ([]byte, error) {
	return _configHiveadmissionHiveadmission_rbac_role_bindingYaml, nil
}

func configHiveadmissionHiveadmission_rbac_role_bindingYaml() (*asset, error) {
	bytes, err := configHiveadmissionHiveadmission_rbac_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/hiveadmission_rbac_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionMachinepoolWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: machinepoolvalidators.admission.hive.openshift.io
webhooks:
- name: machinepoolvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/machinepoolvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - machinepools
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionMachinepoolWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionMachinepoolWebhookYaml, nil
}

func configHiveadmissionMachinepoolWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionMachinepoolWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/machinepool-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionSelectorsyncsetWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: selectorsyncsetvalidators.admission.hive.openshift.io
webhooks:
- name: selectorsyncsetvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/selectorsyncsetvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - selectorsyncsets
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionSelectorsyncsetWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionSelectorsyncsetWebhookYaml, nil
}

func configHiveadmissionSelectorsyncsetWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionSelectorsyncsetWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/selectorsyncset-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionServiceAccountYaml = []byte(`---
# to be able to assign powers to the hiveadmission process
apiVersion: v1
kind: ServiceAccount
metadata:
  name: hiveadmission
`)

func configHiveadmissionServiceAccountYamlBytes() ([]byte, error) {
	return _configHiveadmissionServiceAccountYaml, nil
}

func configHiveadmissionServiceAccountYaml() (*asset, error) {
	bytes, err := configHiveadmissionServiceAccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/service-account.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionServiceYaml = []byte(`---
apiVersion: v1
kind: Service
metadata:
  namespace: hive
  name: hiveadmission
  annotations:
    service.alpha.openshift.io/serving-cert-secret-name: hiveadmission-serving-cert
spec:
  selector:
    app: hiveadmission
  ports:
  - port: 443
    targetPort: 9443
    protocol: TCP
`)

func configHiveadmissionServiceYamlBytes() ([]byte, error) {
	return _configHiveadmissionServiceYaml, nil
}

func configHiveadmissionServiceYaml() (*asset, error) {
	bytes, err := configHiveadmissionServiceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/service.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionSyncsetWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: syncsetvalidators.admission.hive.openshift.io
webhooks:
- name: syncsetvalidators.admission.hive.openshift.io
  clientConfig:
    service:
      # reach the webhook via the registered aggregated API
      namespace: default
      name: kubernetes
      path: /apis/admission.hive.openshift.io/v1/syncsetvalidators
  rules:
  - operations:
    - CREATE
    - UPDATE
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - syncsets
  failurePolicy: Fail
  sideEffects: None
`)

func configHiveadmissionSyncsetWebhookYamlBytes() ([]byte, error) {
	return _configHiveadmissionSyncsetWebhookYaml, nil
}

func configHiveadmissionSyncsetWebhookYaml() (*asset, error) {
	bytes, err := configHiveadmissionSyncsetWebhookYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/syncset-webhook.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configControllersDeploymentYaml = []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: hive-controllers
  namespace: hive
  labels:
    control-plane: controller-manager
    controller-tools.k8s.io: "1.0"
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
      controller-tools.k8s.io: "1.0"
  replicas: 1
  revisionHistoryLimit: 4
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        control-plane: controller-manager
        controller-tools.k8s.io: "1.0"
    spec:
      serviceAccountName: hive-controllers
      volumes:
      - name: kubectl-cache
        emptyDir: {}
      containers:
      # By default we will use the latest CI images published from hive master:
      - image: registry.ci.openshift.org/openshift/hive-v4.0:hive
        imagePullPolicy: Always
        name: manager
        resources:
          requests:
            cpu: 50m
            memory: 512Mi
        command:
          - /opt/services/manager
        envFrom:
          - configMapRef:
              name: hive-controllers-config
        volumeMounts:
        - name: kubectl-cache
          mountPath: /var/cache/kubectl
        env:
        - name: CLI_CACHE_DIR
          value: /var/cache/kubectl
        - name: HIVE_NS
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
      terminationGracePeriodSeconds: 10
`)

func configControllersDeploymentYamlBytes() ([]byte, error) {
	return _configControllersDeploymentYaml, nil
}

func configControllersDeploymentYaml() (*asset, error) {
	bytes, err := configControllersDeploymentYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/controllers/deployment.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configControllersHive_controllers_roleYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  creationTimestamp: null
  name: hive-controllers
rules:
- apiGroups:
  - hive.openshift.io
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - hiveinternal.openshift.io
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - extensions.hive.openshift.io
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - serviceaccounts
  - secrets
  - configmaps
  - events
  - persistentvolumeclaims
  - namespaces
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - apps
  resources:
  - statefulsets
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - rbac.authorization.k8s.io
  resources:
  - roles
  - rolebindings
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - velero.io
  resources:
  - backups
  verbs:
  - create
- apiGroups:
  - cluster.x-k8s.io
  resources:
  - machinesets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
`)

func configControllersHive_controllers_roleYamlBytes() ([]byte, error) {
	return _configControllersHive_controllers_roleYaml, nil
}

func configControllersHive_controllers_roleYaml() (*asset, error) {
	bytes, err := configControllersHive_controllers_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/controllers/hive_controllers_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configControllersHive_controllers_role_bindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: null
  name: hive-controllers
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: hive-controllers
subjects:
- kind: ServiceAccount
  name: hive-controllers
  namespace: system
`)

func configControllersHive_controllers_role_bindingYamlBytes() ([]byte, error) {
	return _configControllersHive_controllers_role_bindingYaml, nil
}

func configControllersHive_controllers_role_bindingYaml() (*asset, error) {
	bytes, err := configControllersHive_controllers_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/controllers/hive_controllers_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configControllersHive_controllers_serviceaccountYaml = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: hive-controllers
  namespace: hive
`)

func configControllersHive_controllers_serviceaccountYamlBytes() ([]byte, error) {
	return _configControllersHive_controllers_serviceaccountYaml, nil
}

func configControllersHive_controllers_serviceaccountYaml() (*asset, error) {
	bytes, err := configControllersHive_controllers_serviceaccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/controllers/hive_controllers_serviceaccount.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configControllersServiceYaml = []byte(`apiVersion: v1
kind: Service
metadata:
  name: hive-controllers
  namespace: hive
  labels:
    control-plane: controller-manager
    controller-tools.k8s.io: "1.0"
spec:
  selector:
    control-plane: controller-manager
    controller-tools.k8s.io: "1.0"
  ports:
  - name: metrics
    port: 2112
    protocol: TCP
  # Expose 6060 for pprof data. Normally nothing listening here unless a developer has
  # compiled in pprof support. See Hive developer documentation for how to use.
  - name: profiling
    port: 6060
    protocol: TCP
`)

func configControllersServiceYamlBytes() ([]byte, error) {
	return _configControllersServiceYaml, nil
}

func configControllersServiceYaml() (*asset, error) {
	bytes, err := configControllersServiceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/controllers/service.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_admin_roleYaml = []byte(`# hive-admin is a role intended for hive administrators who need to be able to debug
# cluster installations, and modify hive configuration.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hive-admin
rules:
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterdeployments
  - clusterprovisions
  - dnszones
  - machinepools
  - machinepoolnameleases
  - selectorsyncidentityproviders
  - syncidentityproviders
  - syncsets
  - syncsetinstances
  - clusterdeprovisions
  # TODO: remove once v1alpha1 compat removed
  - clusterdeprovisionrequests
  - clusterstates
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterimagesets
  - hiveconfigs
  - selectorsyncsets
  - selectorsyncidentityproviders
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - hiveinternal.openshift.io
  resources:
  - clustersyncs
  - clustersyncleases
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - admission.hive.openshift.io
  resources:
  - clusterdeployments
  - clusterimagesets
  - clusterprovisions
  - dnszones
  - machinepools
  - selectorsyncsets
  - syncsets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - apiextensions.k8s.io
  resources:
  - customresourcedefinitions
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - apiregistration.k8s.io
  resources:
  - apiservices
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - admissionregistration.k8s.io
  resources:
  - mutatingwebhookconfigurations
  - validatingwebhookconfigurations
  verbs:
  - get
  - list
  - watch
`)

func configRbacHive_admin_roleYamlBytes() ([]byte, error) {
	return _configRbacHive_admin_roleYaml, nil
}

func configRbacHive_admin_roleYaml() (*asset, error) {
	bytes, err := configRbacHive_admin_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_admin_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_admin_role_bindingYaml = []byte(`# NOTE: This binding uses the openshift apigroup as it is the only way to link
# to an openshift user group. This will not work if running hive on vanilla Kube,
# but the Hive operator will detect this and skip creation of the binding.
apiVersion: authorization.openshift.io/v1
kind: ClusterRoleBinding
metadata:
  name: hive-admin
roleRef:
  name: hive-admin
groupNames:
- hive-admins
subjects:
- kind: Group
  name: hive-admins
`)

func configRbacHive_admin_role_bindingYamlBytes() ([]byte, error) {
	return _configRbacHive_admin_role_bindingYaml, nil
}

func configRbacHive_admin_role_bindingYaml() (*asset, error) {
	bytes, err := configRbacHive_admin_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_admin_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_clusterpool_adminYaml = []byte(`# hive-cluster-pool-admin is a role intended for cluster pool administrators who need to be able to debug
# cluster installations for the pool.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hive-cluster-pool-admin
rules:
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  - configmaps
  - secrets
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterdeployments
  - clusterprovisions
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterpools
  - clusterclaims
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
`)

func configRbacHive_clusterpool_adminYamlBytes() ([]byte, error) {
	return _configRbacHive_clusterpool_adminYaml, nil
}

func configRbacHive_clusterpool_adminYaml() (*asset, error) {
	bytes, err := configRbacHive_clusterpool_adminYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_clusterpool_admin.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_frontend_roleYaml = []byte(`# hive-frontend is a role intended for integrating applications acting as a frontend
# to Hive. These applications will need quite powerful permissions in the Hive cluster
# to create namespaces to organize clusters, as well as all the required objects in those
# clusters.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hive-frontend
rules:
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - secrets
  - configmaps
  - events
  - namespaces
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterdeployments
  - clusterprovisions
  - dnszones
  - machinepools
  - selectorsyncidentityproviders
  - syncidentityproviders
  - selectorsyncsets
  - syncsets
  - clusterdeprovisions
  # TODO: remove once v1alpha1 compat removed
  - clusterdeprovisionrequests
  - clusterstates
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterimagesets
  - hiveconfigs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hiveinternal.openshift.io
  resources:
  - clustersyncs
  - clustersyncleases
  verbs:
  - get
  - list
  - watch
`)

func configRbacHive_frontend_roleYamlBytes() ([]byte, error) {
	return _configRbacHive_frontend_roleYaml, nil
}

func configRbacHive_frontend_roleYaml() (*asset, error) {
	bytes, err := configRbacHive_frontend_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_frontend_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_frontend_role_bindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: null
  name: hive-frontend
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: hive-frontend
subjects:
- kind: ServiceAccount
  name: hive-frontend
  namespace: hive
- kind: Group
  name: hive-frontend
`)

func configRbacHive_frontend_role_bindingYamlBytes() ([]byte, error) {
	return _configRbacHive_frontend_role_bindingYaml, nil
}

func configRbacHive_frontend_role_bindingYaml() (*asset, error) {
	bytes, err := configRbacHive_frontend_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_frontend_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_frontend_serviceaccountYaml = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: hive-frontend
  namespace: hive
`)

func configRbacHive_frontend_serviceaccountYamlBytes() ([]byte, error) {
	return _configRbacHive_frontend_serviceaccountYaml, nil
}

func configRbacHive_frontend_serviceaccountYaml() (*asset, error) {
	bytes, err := configRbacHive_frontend_serviceaccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_frontend_serviceaccount.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_reader_roleYaml = []byte(`# hive-admin is a role intended for hive administrators who need to be able to debug
# cluster installations, and modify hive configuration.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: hive-reader
rules:
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - pods
  - pods/log
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterdeployments
  - clusterprovisions
  - dnszones
  - machinepools
  - selectorsyncidentityproviders
  - selectorsyncsets
  - syncidentityproviders
  - syncsets
  - syncsetinstances
  - clusterdeprovisions
  # TODO: remove once v1alpha1 compat removed
  - clusterdeprovisionrequests
  - clusterstates
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterimagesets
  - hiveconfigs
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - hiveinternal.openshift.io
  resources:
  - clustersyncs
  - clustersyncleases
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - apiextensions.k8s.io
  resources:
  - customresourcedefinitions
  verbs:
  - get
  - list
  - watch
`)

func configRbacHive_reader_roleYamlBytes() ([]byte, error) {
	return _configRbacHive_reader_roleYaml, nil
}

func configRbacHive_reader_roleYaml() (*asset, error) {
	bytes, err := configRbacHive_reader_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_reader_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configRbacHive_reader_role_bindingYaml = []byte(`# NOTE: This binding uses the openshift apigroup as it is the only way to link
# to an openshift user group. This will not work if running hive on vanilla Kube,
# but the Hive operator will detect this and skip creation of the binding.
apiVersion: authorization.openshift.io/v1
kind: ClusterRoleBinding
metadata:
  name: hive-reader
roleRef:
  name: hive-reader
groupNames:
- hive-readers
subjects:
- kind: Group
  name: hive-readers
`)

func configRbacHive_reader_role_bindingYamlBytes() ([]byte, error) {
	return _configRbacHive_reader_role_bindingYaml, nil
}

func configRbacHive_reader_role_bindingYaml() (*asset, error) {
	bytes, err := configRbacHive_reader_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/rbac/hive_reader_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configConfigmapsInstallLogRegexesConfigmapYaml = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: install-log-regexes
  namespace: hive
data:
  regexes: |
    # AWS Specific
    - name: AWSNATGatewayLimitExceeded
      searchRegexStrings:
      - "NatGatewayLimitExceeded"
      installFailingReason: AWSNATGatewayLimitExceeded
      installFailingMessage: AWS NAT gateway limit exceeded
    - name: AWSVPCLimitExceeded
      searchRegexStrings:
      - "VpcLimitExceeded"
      installFailingReason: AWSVPCLimitExceeded
      installFailingMessage: AWS VPC limit exceeded
    - name: S3BucketsLimitExceeded
      searchRegexStrings:
       - "TooManyBuckets"
      installFailingReason: S3BucketsLimitExceeded
      installFailingMessage: S3 Buckets Limit Exceeded
    - name: EIPAddressLimitExceeded
      searchRegexStrings:
      - "EIP: AddressLimitExceeded"
      installFailingReason: EIPAddressLimitExceeded
      installFailingMessage: EIP Address limit exceeded
    - name: LimitExceeded
      searchRegexStrings:
      - "LimitExceeded"
      installFailingReason: ResourceLimitExceeded
      installFailingMessage: Resource limit exceeded
    - name: InvalidInstallConfigSubnet
      searchRegexStrings:
      - "CIDR range start.*is outside of the specified machine networks"
      installFailingReason: InvalidInstallConfigSubnet
      installFailingMessage: Invalid subnet in install config. Subnet's CIDR range start is outside of the specified machine networks
    # https://bugzilla.redhat.com/show_bug.cgi?id=1844320
    - name: AWSUnableToFindMatchingRouteTable
      searchRegexStrings:
      - "Error: Unable to find matching route for Route Table"
      installFailingReason: AWSUnableToFindMatchingRouteTable
      installFailingMessage: Unable to find matching route for route table
    - name: DNSAlreadyExists
      searchRegexStrings:
      - "aws_route53_record.*Error building changeset:.*Tried to create resource record set.*but it already exists"
      installFailingReason: DNSAlreadyExists
      installFailingMessage: DNS record already exists
    - name: PendingVerification
      searchRegexStrings:
      - "PendingVerification: Your request for accessing resources in this region is being validated"
      installFailingReason: PendingVerification
      installFailingMessage: Account pending verification for region
    - name: NoMatchingRoute53Zone
      searchRegexStrings:
      - "data.aws_route53_zone.public: no matching Route53Zone found"
      installFailingReason: NoMatchingRoute53Zone
      installFailingMessage: No matching Route53Zone found
    - name: SimulatorThrottling
      searchRegexStrings:
      - "validate AWS credentials: checking install permissions: error simulating policy: Throttling: Rate exceeded"
      installFailingReason: AWSAPIRateLimitExceeded
      installFailingMessage: AWS API rate limit exceeded while simulating policy
    - name: GeneralThrottling
      searchRegexStrings:
      - "Throttling: Rate exceeded"
      installFailingReason: AWSAPIRateLimitExceeded
      installFailingMessage: AWS API rate limit exceeded
    - name: InvalidCredentials
      searchRegexStrings:
      - "InvalidClientTokenId: The security token included in the request is invalid."
      installFailingReason: InvalidCredentials
      installFailingMessage: Credentials are invalid
    # GCP Specific
    - name: GCPInvalidProjectID
      searchRegexStrings:
      - "platform.gcp.project.* invalid project ID"
      installFailingReason: GCPInvalidProjectID
      installFailingMessage: Invalid GCP project ID
    - name: GCPInstanceTypeNotFound
      searchRegexStrings:
      - "platform.gcp.type: Invalid value:.* instance type.* not found]"
      installFailingReason: GCPInstanceTypeNotFound
      installFailingMessage: GCP instance type not found
    - name: GCPPreconditionFailed
      searchRegexStrings:
      - "googleapi: Error 412"
      installFailingReason: GCPPreconditionFailed
      installFailingMessage: GCP Precondition Failed
    - name: GCPQuotaSSDTotalGBExceeded
      searchRegexStrings:
      - "Quota \'SSD_TOTAL_GB\' exceeded"
      installFailingReason: GCPQuotaSSDTotalGBExceeded
      installFailingMessage: GCP quota SSD_TOTAL_GB exceeded
    # Bare Metal
    - name: LibvirtSSHKeyPermissionDenied
      searchRegexStrings:
      - "platform.baremetal.libvirtURI: Internal error: could not connect to libvirt: virError.Code=38, Domain=7, Message=.Cannot recv data: Permission denied"
      installFailingReason: LibvirtSSHKeyPermissionDenied
      installFailingMessage: "Permission denied connecting to libvirt host, check SSH key configuration and pass phrase"
    # Generic OpenShift Install
    - name: KubeAPIWaitTimeout
      searchRegexStrings:
      - "waiting for Kubernetes API: context deadline exceeded"
      installFailingReason: KubeAPIWaitTimeout
      installFailingMessage: Timeout waiting for the Kubernetes API to begin responding
    - name: MonitoringOperatorStillUpdating
      searchRegexStrings:
      - "failed to initialize the cluster: Cluster operator monitoring is still updating"
      installFailingReason: MonitoringOperatorStillUpdating
      installFailingMessage: Timeout waiting for the monitoring operator to become ready
    - name: AuthenticationOperatorDegraded
      searchRegexStrings:
      - "Cluster operator authentication Degraded is True"
      installFailingReason: AuthenticationOperatorDegraded
      installFailingMessage: Timeout waiting for the authentication operator to become ready
    - name: GeneralOperatorDegraded
      searchRegexStrings:
      - "Cluster operator.*Degraded is True"
      installFailingReason: GeneralOperatorDegraded
      installFailingMessage: Timeout waiting for an operator to become ready
    - name: GeneralClusterOperatorsStillUpdating
      searchRegexStrings:
      - "failed to initialize the cluster: Some cluster operators are still updating:"
      installFailingReason: GeneralClusterOperatorsStillUpdating
      installFailingMessage: Timeout waiting for all cluster operators to become ready
    - name: KubeAPIWaitFailed
      searchRegexStrings:
      - "Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane"
      installFailingReason: KubeAPIWaitFailed
      installFailingMessage: Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane
    # Processing stops at the first match, so this more generic
    # message about the connection failure must always come after the
    # more specific message for LibvirtSSHKeyPermissionDenied.
    - name: InvalidInstallConfig
      searchRegexStrings:
      - "failed to load asset \\\"Install Config\\\""
      installFailingReason: InvalidInstallConfig
      installFailingMessage: Installer failed to load install config
    - name: LibvirtConnectionFailed
      searchRegexStrings:
      - "could not connect to libvirt"
      installFailingReason: LibvirtConnectionFailed
      installFailingMessage: "Could not connect to libvirt host"
    - name: GeneralQuota
      searchRegexStrings:
      - "Quota '[A-Z_]*' exceeded"
      installFailingReason: GeneralQuotaExceeded
      installFailingMessage: Quota exceeded
`)

func configConfigmapsInstallLogRegexesConfigmapYamlBytes() ([]byte, error) {
	return _configConfigmapsInstallLogRegexesConfigmapYaml, nil
}

func configConfigmapsInstallLogRegexesConfigmapYaml() (*asset, error) {
	bytes, err := configConfigmapsInstallLogRegexesConfigmapYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/configmaps/install-log-regexes-configmap.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"config/clustersync/service.yaml":                           configClustersyncServiceYaml,
	"config/clustersync/statefulset.yaml":                       configClustersyncStatefulsetYaml,
	"config/hiveadmission/apiservice.yaml":                      configHiveadmissionApiserviceYaml,
	"config/hiveadmission/clusterdeployment-webhook.yaml":       configHiveadmissionClusterdeploymentWebhookYaml,
	"config/hiveadmission/clusterimageset-webhook.yaml":         configHiveadmissionClusterimagesetWebhookYaml,
	"config/hiveadmission/clusterprovision-webhook.yaml":        configHiveadmissionClusterprovisionWebhookYaml,
	"config/hiveadmission/deployment.yaml":                      configHiveadmissionDeploymentYaml,
	"config/hiveadmission/dnszones-webhook.yaml":                configHiveadmissionDnszonesWebhookYaml,
	"config/hiveadmission/hiveadmission_rbac_role.yaml":         configHiveadmissionHiveadmission_rbac_roleYaml,
	"config/hiveadmission/hiveadmission_rbac_role_binding.yaml": configHiveadmissionHiveadmission_rbac_role_bindingYaml,
	"config/hiveadmission/machinepool-webhook.yaml":             configHiveadmissionMachinepoolWebhookYaml,
	"config/hiveadmission/selectorsyncset-webhook.yaml":         configHiveadmissionSelectorsyncsetWebhookYaml,
	"config/hiveadmission/service-account.yaml":                 configHiveadmissionServiceAccountYaml,
	"config/hiveadmission/service.yaml":                         configHiveadmissionServiceYaml,
	"config/hiveadmission/syncset-webhook.yaml":                 configHiveadmissionSyncsetWebhookYaml,
	"config/controllers/deployment.yaml":                        configControllersDeploymentYaml,
	"config/controllers/hive_controllers_role.yaml":             configControllersHive_controllers_roleYaml,
	"config/controllers/hive_controllers_role_binding.yaml":     configControllersHive_controllers_role_bindingYaml,
	"config/controllers/hive_controllers_serviceaccount.yaml":   configControllersHive_controllers_serviceaccountYaml,
	"config/controllers/service.yaml":                           configControllersServiceYaml,
	"config/rbac/hive_admin_role.yaml":                          configRbacHive_admin_roleYaml,
	"config/rbac/hive_admin_role_binding.yaml":                  configRbacHive_admin_role_bindingYaml,
	"config/rbac/hive_clusterpool_admin.yaml":                   configRbacHive_clusterpool_adminYaml,
	"config/rbac/hive_frontend_role.yaml":                       configRbacHive_frontend_roleYaml,
	"config/rbac/hive_frontend_role_binding.yaml":               configRbacHive_frontend_role_bindingYaml,
	"config/rbac/hive_frontend_serviceaccount.yaml":             configRbacHive_frontend_serviceaccountYaml,
	"config/rbac/hive_reader_role.yaml":                         configRbacHive_reader_roleYaml,
	"config/rbac/hive_reader_role_binding.yaml":                 configRbacHive_reader_role_bindingYaml,
	"config/configmaps/install-log-regexes-configmap.yaml":      configConfigmapsInstallLogRegexesConfigmapYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"config": {nil, map[string]*bintree{
		"clustersync": {nil, map[string]*bintree{
			"service.yaml":     {configClustersyncServiceYaml, map[string]*bintree{}},
			"statefulset.yaml": {configClustersyncStatefulsetYaml, map[string]*bintree{}},
		}},
		"configmaps": {nil, map[string]*bintree{
			"install-log-regexes-configmap.yaml": {configConfigmapsInstallLogRegexesConfigmapYaml, map[string]*bintree{}},
		}},
		"controllers": {nil, map[string]*bintree{
			"deployment.yaml":                      {configControllersDeploymentYaml, map[string]*bintree{}},
			"hive_controllers_role.yaml":           {configControllersHive_controllers_roleYaml, map[string]*bintree{}},
			"hive_controllers_role_binding.yaml":   {configControllersHive_controllers_role_bindingYaml, map[string]*bintree{}},
			"hive_controllers_serviceaccount.yaml": {configControllersHive_controllers_serviceaccountYaml, map[string]*bintree{}},
			"service.yaml":                         {configControllersServiceYaml, map[string]*bintree{}},
		}},
		"hiveadmission": {nil, map[string]*bintree{
			"apiservice.yaml":                      {configHiveadmissionApiserviceYaml, map[string]*bintree{}},
			"clusterdeployment-webhook.yaml":       {configHiveadmissionClusterdeploymentWebhookYaml, map[string]*bintree{}},
			"clusterimageset-webhook.yaml":         {configHiveadmissionClusterimagesetWebhookYaml, map[string]*bintree{}},
			"clusterprovision-webhook.yaml":        {configHiveadmissionClusterprovisionWebhookYaml, map[string]*bintree{}},
			"deployment.yaml":                      {configHiveadmissionDeploymentYaml, map[string]*bintree{}},
			"dnszones-webhook.yaml":                {configHiveadmissionDnszonesWebhookYaml, map[string]*bintree{}},
			"hiveadmission_rbac_role.yaml":         {configHiveadmissionHiveadmission_rbac_roleYaml, map[string]*bintree{}},
			"hiveadmission_rbac_role_binding.yaml": {configHiveadmissionHiveadmission_rbac_role_bindingYaml, map[string]*bintree{}},
			"machinepool-webhook.yaml":             {configHiveadmissionMachinepoolWebhookYaml, map[string]*bintree{}},
			"selectorsyncset-webhook.yaml":         {configHiveadmissionSelectorsyncsetWebhookYaml, map[string]*bintree{}},
			"service-account.yaml":                 {configHiveadmissionServiceAccountYaml, map[string]*bintree{}},
			"service.yaml":                         {configHiveadmissionServiceYaml, map[string]*bintree{}},
			"syncset-webhook.yaml":                 {configHiveadmissionSyncsetWebhookYaml, map[string]*bintree{}},
		}},
		"rbac": {nil, map[string]*bintree{
			"hive_admin_role.yaml":              {configRbacHive_admin_roleYaml, map[string]*bintree{}},
			"hive_admin_role_binding.yaml":      {configRbacHive_admin_role_bindingYaml, map[string]*bintree{}},
			"hive_clusterpool_admin.yaml":       {configRbacHive_clusterpool_adminYaml, map[string]*bintree{}},
			"hive_frontend_role.yaml":           {configRbacHive_frontend_roleYaml, map[string]*bintree{}},
			"hive_frontend_role_binding.yaml":   {configRbacHive_frontend_role_bindingYaml, map[string]*bintree{}},
			"hive_frontend_serviceaccount.yaml": {configRbacHive_frontend_serviceaccountYaml, map[string]*bintree{}},
			"hive_reader_role.yaml":             {configRbacHive_reader_roleYaml, map[string]*bintree{}},
			"hive_reader_role_binding.yaml":     {configRbacHive_reader_role_bindingYaml, map[string]*bintree{}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
