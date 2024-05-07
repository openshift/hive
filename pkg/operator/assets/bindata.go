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
// config/hiveadmission/sa-token-secret.yaml
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
        - name: TMPDIR
          value: /tmp
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        securityContext:
          privileged: false
          readOnlyRootFilesystem: true
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
      volumes:
      - name: tmp
        emptyDir: {}
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: clusterdeploymentvalidators.admission.hive.openshift.io
webhooks:
- name: clusterdeploymentvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: clusterimagesetvalidators.admission.hive.openshift.io
webhooks:
- name: clusterimagesetvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: clusterprovisionvalidators.admission.hive.openshift.io
webhooks:
- name: clusterprovisionvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
        ports:
        - containerPort: 9443
          protocol: TCP
        envFrom:
        - configMapRef:
            name: hive-feature-gates
        env:
        - name: TMPDIR
          value: /tmp
        volumeMounts:
        - mountPath: /var/serving-cert
          name: serving-cert
        - name: tmp
          mountPath: /tmp
        securityContext:
          privileged: false
          readOnlyRootFilesystem: true
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
      - name: tmp
        emptyDir: {}
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: dnszonevalidators.admission.hive.openshift.io
webhooks:
- name: dnszonevalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
- apiGroups:
  - flowcontrol.apiserver.k8s.io
  resources:
  - prioritylevelconfigurations
  - flowschemas
  verbs:
  - get
  - list
  - watch
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: machinepoolvalidators.admission.hive.openshift.io
webhooks:
- name: machinepoolvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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

var _configHiveadmissionSaTokenSecretYaml = []byte(`# Prior to k8s 1.24, this secret was generated automatically
apiVersion: v1
kind: Secret
metadata:
  name: hiveadmission-sa-token
  namespace: hive
  annotations:
    kubernetes.io/service-account.name: "hiveadmission"
type: kubernetes.io/service-account-token`)

func configHiveadmissionSaTokenSecretYamlBytes() ([]byte, error) {
	return _configHiveadmissionSaTokenSecretYaml, nil
}

func configHiveadmissionSaTokenSecretYaml() (*asset, error) {
	bytes, err := configHiveadmissionSaTokenSecretYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/hiveadmission/sa-token-secret.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configHiveadmissionSelectorsyncsetWebhookYaml = []byte(`---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: selectorsyncsetvalidators.admission.hive.openshift.io
webhooks:
- name: selectorsyncsetvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
  name: syncsetvalidators.admission.hive.openshift.io
webhooks:
- name: syncsetvalidators.admission.hive.openshift.io
  admissionReviewVersions:
  - v1beta1
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
      - name: tmp
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
        - name: tmp
          mountPath: /tmp
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: CLI_CACHE_DIR
          value: /var/cache/kubectl
        - name: HIVE_NS
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: TMPDIR
          value: /tmp
        securityContext:
          privileged: false
          readOnlyRootFilesystem: true
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
  - coordination.k8s.io
  resources:
  - leases
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
  labels:
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
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
  - clusterdeploymentcustomizations
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
  - clusterdeploymentcustomizations
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
  - clusterdeprovisions
  verbs:
  - get
  - list
  - watch
  - update
  - patch
  - delete
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
  labels:
    rbac.authorization.k8s.io/aggregate-to-view: "true"
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
  - clusterdeploymentcustomizations
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
    # AWS Specific:
    - name: AWSInsufficientCapacity
      searchRegexStrings:
      - "Error: .*InsufficientInstanceCapacity.* Our system will be working on provisioning additional capacity"
      installFailingReason: AWSInsufficientCapacity
      installFailingMessage: AWS currently does not have sufficient capacity to provision the requested EC2 instances in the specified Availability Zone. Please try again later or in a different Availability Zone.
    - name: AWSEC2QuotaExceeded
      searchRegexStrings:
      - "failed to generate asset.*Platform Quota Check.*MissingQuota.*ec2"
      installFailingReason: AWSEC2QuotaExceeded
      installFailingMessage: AWS EC2 Quota Exceeded
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
    - name: LoadBalancerLimitExceeded
      searchRegexStrings:
      - "TooManyLoadBalancers: Exceeded quota of account"
      installFailingReason: LoadBalancerLimitExceeded
      installFailingMessage: AWS Load Balancer Limit Exceeded
    - name: EIPAddressLimitExceeded
      searchRegexStrings:
      - "EIP: AddressLimitExceeded"
      installFailingReason: EIPAddressLimitExceeded
      installFailingMessage: EIP Address limit exceeded
    - name: AWSSubnetInsufficientIPSpace
      searchRegexStrings:
      - "InvalidSubnet: Not enough IP space available in"
      installFailingReason: AWSSubnetInsufficientIPSpace
      installFailingMessage: Insufficient IP space available in subnet
    - name: MissingPublicSubnetForZone
      searchRegexStrings:
      - "No public subnet provided for zone"
      installFailingReason: MissingPublicSubnetForZone
      installFailingMessage: No public subnet provided for at least one zone
    - name: PrivateSubnetInMultipleZones
      searchRegexStrings:
      - "private subnet .* is also in zone"
      installFailingReason: PrivateSubnetInMultipleZones
      installFailingMessage: Same private subnet used in multiple zones
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
    - name: TooManyRoute53Zones
      searchRegexStrings:
      - "error creating Route53 Hosted Zone: TooManyHostedZones: Limits Exceeded"
      installFailingReason: TooManyRoute53Zones
      installFailingMessage: Route53 hosted zone limit exceeded
    - name: MultipleRoute53ZonesFound
      searchRegexStrings:
        - "Error: multiple Route53Zone found"
      installFailingReason: MultipleRoute53ZonesFound
      installFailingMessage: Multiple Route53 zones found
    - name: DefaultEbsKmsKeyInsufficientPermissions
      searchRegexStrings:
        - "Client.InternalError: Client error on launch"
        - "Client.InvalidKMSKey.InvalidState: The KMS key provided is in an incorrect state"
      installFailingReason: DefaultEbsKmsKeyInsufficientPermissions
      installFailingMessage: Default KMS key for EBS encryption has insufficient permissions to launch EC2 instances
    - name: SimulatorThrottling
      searchRegexStrings:
      - "validate AWS credentials: checking install permissions: error simulating policy: Throttling: Rate exceeded"
      installFailingReason: AWSAPIRateLimitExceeded
      installFailingMessage: AWS API rate limit exceeded while simulating policy
    - name: S3AccessControlListNotSupported
      searchRegexStrings:
      - "error creating S3 bucket ACL for.*AccessControlListNotSupported: The bucket does not allow ACLs"
      installFailingReason: S3AccessControlListNotSupported
      installFailingMessage: S3AccessControlListNotSupported
    - name: GeneralThrottling
      searchRegexStrings:
      - "Throttling: Rate exceeded"
      installFailingReason: AWSAPIRateLimitExceeded
      installFailingMessage: AWS API rate limit exceeded
    # This issue is caused by AWS throttling the CreateHostedZone request. The terraform provider is not properly
    # handling the throttling response and gets stuck in a state where it does not retry the request. Eventually,
    # the terraform provider times out claiming that it is waiting for the hosted zone to be INSYNC.
    - name: AWSRoute53Timeout
      searchRegexStrings:
      - "error waiting for Route53 Hosted Zone .* creation: timeout while waiting for state to become 'INSYNC'"
      installFailingReason: AWSRoute53Timeout
      installFailingMessage: AWS Route53 timeout while waiting for INSYNC. This is usually caused by Route53 rate limiting.
    - name: InvalidCredentials
      searchRegexStrings:
      - "InvalidClientTokenId: The security token included in the request is invalid."
      installFailingReason: InvalidCredentials
      installFailingMessage: Credentials are invalid
    - name: InvalidAWSTags
      searchRegexStrings:
      - "platform\\.aws\\.userTags.*: Invalid value:.*value contains invalid characters"
      installFailingReason: InvalidAWSTags
      installFailingMessage: You have specified an invalid AWS tag value. Verify that your tags meet AWS requirements and try again.
    - name: ErrorDeletingIAMRole
      searchRegexStrings:
        - "Error deleting IAM Role .* DeleteConflict: Cannot delete entity, must detach all policies first."
      installFailingReason: ErrorDeletingIAMRole
      installFailingMessage: The cluster installer was not able to delete the roles it used during the installation. Ensure that no policies are added to new roles by default and try again.
    - name: AWSSubnetDoesNotExist
      searchRegexStrings:
      - "The subnet ID .* does not exist"
      installFailingReason: AWSSubnetDoesNotExist
      installFailingMessage: AWS Subnet Does Not Exist
    # iam:CreateServiceLinkedRole is a super powerful permission that we don't give to STS clusters. We require it's done as a one-time prereq.
    # This is the error we see when the prereq step was missed.
    - name: NATGatewayFailed
      searchRegexStrings:
      - "Error waiting for NAT Gateway (.*) to become available"
      installFailingReason: NATGatewayFailed
      installFailingMessage: Error waiting for NAT Gateway to become available.
    - name: AWSAccessDeniedSLR
      searchRegexStrings:
      - "Error creating network Load Balancer: AccessDenied.*iam:CreateServiceLinkedRole"
      installFailingReason: AWSAccessDeniedSLR
      installFailingMessage: Missing prerequisite service role for load balancer
    - name: AWSInsufficientPermissions
      searchRegexStrings:
      - "current credentials insufficient for performing cluster installation"
      - "UnauthorizedOperation: You are not authorized to perform this operation. Encoded authorization failure message"
      installFailingReason: AWSInsufficientPermissions
      installFailingMessage: AWS credentials are insufficient for performing cluster installation
    - name: AWSDeniedBySCP
      searchRegexStrings:
      - "AccessDenied: .* with an explicit deny in a service control policy"
      - "UnauthorizedOperation: .* with an explicit deny in a service control policy"
      installFailingReason: AWSDeniedBySCP
      installFailingMessage: "A service control policy (SCP) is too restrictive for performing cluster installation"
    - name: VcpuLimitExceeded
      searchRegexStrings:
      - "VcpuLimitExceeded"
      installFailingReason: VcpuLimitExceeded
      installFailingMessage: The install requires more vCPU capacity than your current vCPU limit
    - name: Gp3VolumeLimitExceeded
      searchRegexStrings:
      - "VolumeLimitExceeded: You have exceeded your maximum gp3 storage limit"
      installFailingReason: Gp3VolumeLimitExceeded
      installFailingMessage: "The installation failed due to insufficient gp3 storage quota in the region (QuotaCode L-7A658B76)"
    - name: UserInitiatedShutdown
      searchRegexStrings:
      - "Error waiting for instance .* to become ready .* User initiated shutdown"
      installFailingReason: UserInitiatedShutdown
      installFailingMessage: User initiated shutdown of instances as the install was running
    # openshift-installer intermittent failure on AWS with Error: Provider produced inconsistent result after apply
    - name: InconsistentTerraformResult
      searchRegexStrings:
      - "Error: Provider produced inconsistent result after apply"
      installFailingReason: InconsistentTerraformResult
      installFailingMessage: Inconsistent result after Terraform apply
    - name: AWSVPCDoesNotExist
      searchRegexStrings:
      - "The vpc ID .* does not exist"
      installFailingReason: AWSVPCDoesNotExist
      installFailingMessage: The AWS VPC does not exist
    - name: TargetGroupNotFound
    # https://bugzilla.redhat.com/show_bug.cgi?id=1898265
      searchRegexStrings:
      - "TargetGroupNotFound"
      installFailingReason: TargetGroupNotFound
      installFailingMessage: Target Group cannot be found
    - name: ErrorCreatingNetworkLoadBalancer
      searchRegexStrings:
      - "Error creating network Load Balancer: InternalFailure: "
      installFailingReason: ErrorCreatingNetworkLoadBalancer
      installFailingMessage: AWS network load balancer creation encountered an error during cluster installation
    - name: TerraformFailedToDeleteResources
      searchRegexStrings:
        - "terraform destroy: failed to destroy using Terraform"
        - "terraform destroy: failed doing terraform destroy"
      installFailingReason: InstallerFailedToDestroyResources
      installFailingMessage: The installer failed to destroy installation resources
    - name: AWSAccountBlocked
      searchRegexStrings:
        - "Blocked: This account is currently blocked and not recognized as a valid account."
      installFailingReason: AWSAccountIsBlocked
      installFailingMessage: "AWS account is currently blocked and not recognized as a valid account. Please contact aws-verification@amazon.com if you have questions."


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
    - name: GCPComputeQuota
      searchRegexStrings:
      - "compute\\.googleapis\\.com/cpus is not available in [a-z0-9-]* because the required number of resources \\([0-9]*\\) is more than"
      installFailingReason: GCPComputeQuotaExceeded
      installFailingMessage: GCP CPUs quota exceeded
    - name: GCPServiceAccountQuota
      searchRegexStrings:
      - "iam\\.googleapis\\.com/quota/service-account-count is not available in global because the required number of resources \\([0-9]*\\) is more than remaining quota"
      installFailingReason: GCPServiceAccountQuotaExceeded
      installFailingMessage: GCP Service Account quota exceeded


    # Bare Metal
    - name: LibvirtSSHKeyPermissionDenied
      searchRegexStrings:
      - "platform.baremetal.libvirtURI: Internal error: could not connect to libvirt: virError.Code=38, Domain=7, Message=.Cannot recv data: Permission denied"
      installFailingReason: LibvirtSSHKeyPermissionDenied
      installFailingMessage: "Permission denied connecting to libvirt host, check SSH key configuration and pass phrase"
    - name: LibvirtConnectionFailed
      searchRegexStrings:
      - "could not connect to libvirt"
      installFailingReason: LibvirtConnectionFailed
      installFailingMessage: "Could not connect to libvirt host"


    # Proxy-enabled clusters
    - name: ProxyTimeout
      searchRegexStrings:
      - "error pinging docker registry .+ proxyconnect tcp: dial tcp [^ ]+: i/o timeout"
      - "error pinging docker registry .+ proxyconnect tcp: dial tcp [^ ]+: connect: connection refused"
      - "error pinging docker registry .+ proxyconnect tcp: dial tcp [^ ]+: connect: no route to host"
      installFailingReason: ProxyTimeout
      installFailingMessage: The cluster is installing via a proxy, however the proxy server is refusing or timing out connections. Verify that the proxy is running and would be accessible from the cluster's private subnet(s).
    - name: ProxyInvalidCABundle
      searchRegexStrings:
      - "error pinging docker registry .+ proxyconnect tcp: x509: certificate signed by unknown authority"
      installFailingReason: ProxyInvalidCABundle
      installFailingMessage: The cluster is installing via a proxy, but does not trust the signing certificate the proxy is presenting. Verify that the Certificate Authority certificate(s) to verify proxy communications have been supplied at installation time.


    # Generic OpenShift Install

    - name: KubeAPIWaitTimeout
      searchRegexStrings:
      - "waiting for Kubernetes API: context deadline exceeded"
      installFailingReason: KubeAPIWaitTimeout
      installFailingMessage: Timeout waiting for the Kubernetes API to begin responding
    - name: KubeAPIWaitFailed
      searchRegexStrings:
      - "Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane"
      installFailingReason: KubeAPIWaitFailed
      installFailingMessage: Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane
    - name: BootstrapFailed
      searchRegexStrings:
      - "Failed to wait for bootstrapping to complete. This error usually happens when there is a problem with control plane hosts that prevents the control plane operators from creating the control plane."
      installFailingReason: BootstrapFailed
      installFailingMessage: Failed to wait for bootstrapping to complete. This error usually happens when there is a problem with control plane hosts that prevents the control plane operators from creating the control plane. Verify the networking configuration and account permissions and try again.
    - name: GenericBootstrapFailed
      searchRegexStrings:
      - "Bootstrap failed to complete"
      installFailingReason: GenericBootstrapFailed
      installFailingMessage: Installation Bootstrap failed to complete. Verify the networking configuration and account permissions and try again.
    - name: MonitoringOperatorStillUpdating
      searchRegexStrings:
      - "failed to initialize the cluster: Cluster operator monitoring is still updating"
      installFailingReason: MonitoringOperatorStillUpdating
      installFailingMessage: Timeout waiting for the monitoring operator to become ready
    - name: NoWorkerNodesReady
      searchRegexStrings:
      - "Got 0 worker nodes, \\d+ master nodes.*none are schedulable or ready for ingress pods"
      installFailingReason: NoWorkerNodesReady
      installFailingMessage: 0 worker nodes have joined the cluster
    - name: IngressOperatorDegraded 
      searchRegexStrings:
      - "Cluster operator ingress Degraded is True"
      installFailingReason: IngressOperatorDegraded
      installFailingMessage: Timeout waiting for the ingress operator to become ready
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
    - name: InstallConfigNetworkAuthFail
      searchRegexStrings:
      - "failed to create install config: failed to create a network client: Authentication failed"
      installFailingReason: InstallConfigNetworkAuthFail
      installFailingMessage: Authentication failure attempting to create a network client - check credentials and certificates
    - name: InstallConfigNetworkBadCACert
      searchRegexStrings:
      - "failed to create install config: failed to create a network client: Error parsing CA Cert from"
      installFailingReason: InstallConfigNetworkBadCACert
      installFailingMessage: Failure attempting to create a network client - invalid CA certificate

    # Keep these at the bottom so that they're only hit if nothing above matches.
    # We don't want to show these to users unless it's a last resort. It's barely better than "unknown error".
    # These are clues to SRE that they need to add more specific regexps to this file.
    - name: FallbackQuotaExceeded
      searchRegexStrings:
      - "Quota '[A-Z_]*' exceeded"
      installFailingReason: FallbackQuotaExceeded
      installFailingMessage: Unknown quota exceeded - couldn't parse a specific resource type
    - name: FallbackResourceLimitExceeded
      searchRegexStrings:
      - "LimitExceeded"
      installFailingReason: FallbackResourceLimitExceeded
      installFailingMessage: Unknown resource limit exceeded - couldn't parse a specific resource type
    - name: FallbackInvalidInstallConfig
      searchRegexStrings:
      - "failed to load asset \\\"Install Config\\\""
      installFailingReason: FallbackInvalidInstallConfig
      installFailingMessage: Unknown error - installer failed to load install config
    - name: FallbackInstancesFailedToBecomeReady
      searchRegexStrings:
      - "Error waiting for instance .* to become ready"
      installFailingReason: FallbackInstancesFailedToBecomeReady
      installFailingMessage: Unknown error - instances failed to become ready
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
	"config/hiveadmission/sa-token-secret.yaml":                 configHiveadmissionSaTokenSecretYaml,
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
//
//	data/
//	  foo.txt
//	  img/
//	    a.png
//	    b.png
//
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
			"sa-token-secret.yaml":                 {configHiveadmissionSaTokenSecretYaml, map[string]*bintree{}},
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
