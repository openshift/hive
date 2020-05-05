// Code generated for package assets by go-bindata DO NOT EDIT. (@generated)
// sources:
// config/apiserver/apiservice.yaml
// config/apiserver/deployment.yaml
// config/apiserver/hiveapi_rbac_role.yaml
// config/apiserver/hiveapi_rbac_role_binding.yaml
// config/apiserver/service-account.yaml
// config/apiserver/service.yaml
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
// config/rbac/hive_frontend_role.yaml
// config/rbac/hive_frontend_role_binding.yaml
// config/rbac/hive_frontend_serviceaccount.yaml
// config/rbac/hive_reader_role.yaml
// config/rbac/hive_reader_role_binding.yaml
// config/crds/hive_v1_checkpoint.yaml
// config/crds/hive_v1_clusterdeployment.yaml
// config/crds/hive_v1_clusterdeprovision.yaml
// config/crds/hive_v1_clusterimageset.yaml
// config/crds/hive_v1_clusterprovision.yaml
// config/crds/hive_v1_clusterstate.yaml
// config/crds/hive_v1_dnszone.yaml
// config/crds/hive_v1_hiveconfig.yaml
// config/crds/hive_v1_machinepool.yaml
// config/crds/hive_v1_machinepoolnamelease.yaml
// config/crds/hive_v1_selectorsyncidentityprovider.yaml
// config/crds/hive_v1_selectorsyncset.yaml
// config/crds/hive_v1_syncidentityprovider.yaml
// config/crds/hive_v1_syncset.yaml
// config/crds/hive_v1_syncsetinstance.yaml
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

var _configApiserverApiserviceYaml = []byte(`---
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1alpha1.hive.openshift.io
  labels:
    api: hiveapi
    apiserver: "true"
  annotations:
    service.alpha.openshift.io/inject-cabundle: "true"
spec:
  group: hive.openshift.io
  groupPriorityMinimum: 2000
  service:
    name: hiveapi
    namespace: hive
  version: v1alpha1
  versionPriority: 10
`)

func configApiserverApiserviceYamlBytes() ([]byte, error) {
	return _configApiserverApiserviceYaml, nil
}

func configApiserverApiserviceYaml() (*asset, error) {
	bytes, err := configApiserverApiserviceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/apiservice.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configApiserverDeploymentYaml = []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: hiveapi
  namespace: hive
  labels:
    api: hiveapi
    apiserver: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      api: hiveapi
      apiserver: "true"
  template:
    metadata:
      labels:
        api: hiveapi
        apiserver: "true"
    spec:
      containers:
      # By default we will use the latest CI images published from hive master:
      - image: registry.svc.ci.openshift.org/openshift/hive-v4.0:hive
        name: apiserver
        resources:
          requests:
            cpu: 100m
            memory: 400Mi
          limits:
            cpu: 100m
            memory: 600Mi
        command:
          - "/opt/services/hive-apiserver"
        args:
          - "start"
          - "--v=2"
          - "--secure-port=10443"
          - "--logtostderr"
          - "--tls-cert-file=/apiserver.local.config/certificates/tls.crt"
          - "--tls-private-key-file=/apiserver.local.config/certificates/tls.key"
        volumeMounts:
        - name: apiserver-certs
          mountPath: /apiserver.local.config/certificates
          readOnly: true
      serviceAccountName: hiveapi-sa
      volumes:
        - name: apiserver-certs
          secret:
            secretName: hiveapi-serving-cert
`)

func configApiserverDeploymentYamlBytes() ([]byte, error) {
	return _configApiserverDeploymentYaml, nil
}

func configApiserverDeploymentYaml() (*asset, error) {
	bytes, err := configApiserverDeploymentYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/deployment.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configApiserverHiveapi_rbac_roleYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
  name: system:openshift:hive:hiveapi
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get"]
  - apiGroups: ["authorization.k8s.io"]
    resources: ["subjectaccessreviews"]
    verbs: ["create"]
  - apiGroups: ["hive.openshift.io"]
    resources: ["*"]
    verbs: ["*"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["*"]
`)

func configApiserverHiveapi_rbac_roleYamlBytes() ([]byte, error) {
	return _configApiserverHiveapi_rbac_roleYaml, nil
}

func configApiserverHiveapi_rbac_roleYaml() (*asset, error) {
	bytes, err := configApiserverHiveapi_rbac_roleYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/hiveapi_rbac_role.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configApiserverHiveapi_rbac_role_bindingYaml = []byte(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: hiveapi-hive-hiveapi
roleRef:
  kind: ClusterRole
  apiGroup: rbac.authorization.k8s.io
  name: system:openshift:hive:hiveapi
subjects:
  - kind: ServiceAccount
    namespace: hive
    name: hiveapi-sa
`)

func configApiserverHiveapi_rbac_role_bindingYamlBytes() ([]byte, error) {
	return _configApiserverHiveapi_rbac_role_bindingYaml, nil
}

func configApiserverHiveapi_rbac_role_bindingYaml() (*asset, error) {
	bytes, err := configApiserverHiveapi_rbac_role_bindingYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/hiveapi_rbac_role_binding.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configApiserverServiceAccountYaml = []byte(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  namespace: hive
  name: hiveapi-sa
`)

func configApiserverServiceAccountYamlBytes() ([]byte, error) {
	return _configApiserverServiceAccountYaml, nil
}

func configApiserverServiceAccountYaml() (*asset, error) {
	bytes, err := configApiserverServiceAccountYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/service-account.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configApiserverServiceYaml = []byte(`---
apiVersion: v1
kind: Service
metadata:
  annotations:
    service.alpha.openshift.io/serving-cert-secret-name: hiveapi-serving-cert
  labels:
    api: hiveapi
    apiserver: "true"
  name: hiveapi
  namespace: hive
spec:
  ports:
  - port: 443
    protocol: TCP
    targetPort: 10443
  selector:
    api: hiveapi
    apiserver: "true"
`)

func configApiserverServiceYamlBytes() ([]byte, error) {
	return _configApiserverServiceYaml, nil
}

func configApiserverServiceYaml() (*asset, error) {
	bytes, err := configApiserverServiceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/apiserver/service.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
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
    apiGroups:
    - hive.openshift.io
    apiVersions:
    - v1
    resources:
    - clusterdeployments
  failurePolicy: Fail
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
        image: registry.svc.ci.openshift.org/openshift/hive-v4.0:hive
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
      - image: registry.svc.ci.openshift.org/openshift/hive-v4.0:hive
        imagePullPolicy: Always
        name: manager
        resources:
          requests:
            cpu: 50m
            memory: 512Mi
        command:
          - /opt/services/manager
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
  - pods
  - namespaces
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

var _configCrdsHive_v1_checkpointYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: checkpoints.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: Checkpoint
    plural: checkpoints
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: Checkpoint is the Schema for the backup of Hive objects.
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: CheckpointSpec defines the metadata around the Hive objects
            state in the namespace at the time of the last backup.
          type: object
          required:
          - lastBackupChecksum
          - lastBackupRef
          - lastBackupTime
          properties:
            lastBackupChecksum:
              description: LastBackupChecksum is the checksum of all Hive objects
                in the namespace at the time of the last backup.
              type: string
            lastBackupRef:
              description: LastBackupRef is a reference to last backup object created
              type: object
              required:
              - name
              - namespace
              properties:
                name:
                  type: string
                namespace:
                  type: string
            lastBackupTime:
              description: LastBackupTime is the last time we performed a backup of
                the namespace
              type: string
              format: date-time
        status:
          description: CheckpointStatus defines the observed state of Checkpoint
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_checkpointYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_checkpointYaml, nil
}

func configCrdsHive_v1_checkpointYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_checkpointYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_checkpoint.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_clusterdeploymentYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusterdeployments.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.clusterName
    name: ClusterName
    type: string
  - JSONPath: .metadata.labels.hive\.openshift\.io/cluster-type
    name: ClusterType
    type: string
  - JSONPath: .spec.baseDomain
    name: BaseDomain
    type: string
  - JSONPath: .spec.installed
    name: Installed
    type: boolean
  - JSONPath: .spec.clusterMetadata.infraID
    name: InfraID
    type: string
  - JSONPath: .metadata.creationTimestamp
    name: Age
    type: date
  group: hive.openshift.io
  names:
    kind: ClusterDeployment
    plural: clusterdeployments
    shortNames:
    - cd
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: ClusterDeployment is the Schema for the clusterdeployments API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: ClusterDeploymentSpec defines the desired state of ClusterDeployment
          type: object
          required:
          - baseDomain
          - clusterName
          - platform
          properties:
            baseDomain:
              description: BaseDomain is the base domain to which the cluster should
                belong.
              type: string
            certificateBundles:
              description: CertificateBundles is a list of certificate bundles associated
                with this cluster
              type: array
              items:
                description: CertificateBundleSpec specifies a certificate bundle
                  associated with a cluster deployment
                type: object
                required:
                - certificateSecretRef
                - name
                properties:
                  certificateSecretRef:
                    description: CertificateSecretRef is the reference to the secret
                      that contains the certificate bundle. If the certificate bundle
                      is to be generated, it will be generated with the name in this
                      reference. Otherwise, it is expected that the secret should
                      exist in the same namespace as the ClusterDeployment
                    type: object
                    properties:
                      name:
                        description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                          TODO: Add other useful fields. apiVersion, kind, uid?'
                        type: string
                  generate:
                    description: Generate indicates whether this bundle should have
                      real certificates generated for it.
                    type: boolean
                  name:
                    description: Name is an identifier that must be unique within
                      the bundle and must be referenced by an ingress or by the control
                      plane serving certs
                    type: string
            clusterMetadata:
              description: ClusterMetadata contains metadata information about the
                installed cluster.
              type: object
              required:
              - adminKubeconfigSecretRef
              - adminPasswordSecretRef
              - clusterID
              - infraID
              properties:
                adminKubeconfigSecretRef:
                  description: AdminKubeconfigSecretRef references the secret containing
                    the admin kubeconfig for this cluster.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
                adminPasswordSecretRef:
                  description: AdminPasswordSecretRef references the secret containing
                    the admin username/password which can be used to login to this
                    cluster.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
                clusterID:
                  description: ClusterID is a globally unique identifier for this
                    cluster generated during installation. Used for reporting metrics
                    among other places.
                  type: string
                infraID:
                  description: InfraID is an identifier for this cluster generated
                    during installation and used for tagging/naming resources in cloud
                    providers.
                  type: string
            clusterName:
              description: ClusterName is the friendly name of the cluster. It is
                used for subdomains, some resource tagging, and other instances where
                a friendly name for the cluster is useful.
              type: string
            controlPlaneConfig:
              description: ControlPlaneConfig contains additional configuration for
                the target cluster's control plane
              type: object
              properties:
                apiURLOverride:
                  description: APIURLOverride is the optional URL override to which
                    Hive will transition for communication with the API server of
                    the remote cluster. When a remote cluster is created, Hive will
                    initially communicate using the API URL established during installation.
                    If an API URL Override is specified, Hive will periodically attempt
                    to connect to the remote cluster using the override URL. Once
                    Hive has determined that the override URL is active, Hive will
                    use the override URL for further communications with the API server
                    of the remote cluster.
                  type: string
                servingCertificates:
                  description: ServingCertificates specifies serving certificates
                    for the control plane
                  type: object
                  properties:
                    additional:
                      description: Additional is a list of additional domains and
                        certificates that are also associated with the control plane's
                        api endpoint.
                      type: array
                      items:
                        description: ControlPlaneAdditionalCertificate defines an
                          additional serving certificate for a control plane
                        type: object
                        required:
                        - domain
                        - name
                        properties:
                          domain:
                            description: Domain is the domain of the additional control
                              plane certificate
                            type: string
                          name:
                            description: Name references a CertificateBundle in the
                              ClusterDeployment.Spec that should be used for this
                              additional certificate.
                            type: string
                    default:
                      description: Default references the name of a CertificateBundle
                        in the ClusterDeployment that should be used for the control
                        plane's default endpoint.
                      type: string
            ingress:
              description: Ingress allows defining desired clusteringress/shards to
                be configured on the cluster.
              type: array
              items:
                description: ClusterIngress contains the configurable pieces for any
                  ClusterIngress objects that should exist on the cluster.
                type: object
                required:
                - domain
                - name
                properties:
                  domain:
                    description: Domain (sometimes referred to as shard) is the full
                      DNS suffix that the resulting IngressController object will
                      service (eg abcd.mycluster.mydomain.com).
                    type: string
                  name:
                    description: Name of the ClusterIngress object to create.
                    type: string
                  namespaceSelector:
                    description: NamespaceSelector allows filtering the list of namespaces
                      serviced by the ingress controller.
                    type: object
                    properties:
                      matchExpressions:
                        description: matchExpressions is a list of label selector
                          requirements. The requirements are ANDed.
                        type: array
                        items:
                          description: A label selector requirement is a selector
                            that contains values, a key, and an operator that relates
                            the key and values.
                          type: object
                          required:
                          - key
                          - operator
                          properties:
                            key:
                              description: key is the label key that the selector
                                applies to.
                              type: string
                            operator:
                              description: operator represents a key's relationship
                                to a set of values. Valid operators are In, NotIn,
                                Exists and DoesNotExist.
                              type: string
                            values:
                              description: values is an array of string values. If
                                the operator is In or NotIn, the values array must
                                be non-empty. If the operator is Exists or DoesNotExist,
                                the values array must be empty. This array is replaced
                                during a strategic merge patch.
                              type: array
                              items:
                                type: string
                      matchLabels:
                        description: matchLabels is a map of {key,value} pairs. A
                          single {key,value} in the matchLabels map is equivalent
                          to an element of matchExpressions, whose key field is "key",
                          the operator is "In", and the values array contains only
                          "value". The requirements are ANDed.
                        type: object
                        additionalProperties:
                          type: string
                  routeSelector:
                    description: RouteSelector allows filtering the set of Routes
                      serviced by the ingress controller
                    type: object
                    properties:
                      matchExpressions:
                        description: matchExpressions is a list of label selector
                          requirements. The requirements are ANDed.
                        type: array
                        items:
                          description: A label selector requirement is a selector
                            that contains values, a key, and an operator that relates
                            the key and values.
                          type: object
                          required:
                          - key
                          - operator
                          properties:
                            key:
                              description: key is the label key that the selector
                                applies to.
                              type: string
                            operator:
                              description: operator represents a key's relationship
                                to a set of values. Valid operators are In, NotIn,
                                Exists and DoesNotExist.
                              type: string
                            values:
                              description: values is an array of string values. If
                                the operator is In or NotIn, the values array must
                                be non-empty. If the operator is Exists or DoesNotExist,
                                the values array must be empty. This array is replaced
                                during a strategic merge patch.
                              type: array
                              items:
                                type: string
                      matchLabels:
                        description: matchLabels is a map of {key,value} pairs. A
                          single {key,value} in the matchLabels map is equivalent
                          to an element of matchExpressions, whose key field is "key",
                          the operator is "In", and the values array contains only
                          "value". The requirements are ANDed.
                        type: object
                        additionalProperties:
                          type: string
                  servingCertificate:
                    description: ServingCertificate references a CertificateBundle
                      in the ClusterDeployment.Spec that should be used for this Ingress
                    type: string
            installed:
              description: Installed is true if the cluster has been installed
              type: boolean
            manageDNS:
              description: ManageDNS specifies whether a DNSZone should be created
                and managed automatically for this ClusterDeployment
              type: boolean
            platform:
              description: Platform is the configuration for the specific platform
                upon which to perform the installation.
              type: object
              properties:
                aws:
                  description: AWS is the configuration used when installing on AWS.
                  type: object
                  required:
                  - credentialsSecretRef
                  - region
                  properties:
                    credentialsSecretRef:
                      description: CredentialsSecretRef refers to a secret that contains
                        the AWS account access credentials.
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    region:
                      description: Region specifies the AWS region where the cluster
                        will be created.
                      type: string
                    userTags:
                      description: UserTags specifies additional tags for AWS resources
                        created for the cluster.
                      type: object
                      additionalProperties:
                        type: string
                azure:
                  description: Azure is the configuration used when installing on
                    Azure.
                  type: object
                  required:
                  - credentialsSecretRef
                  - region
                  properties:
                    baseDomainResourceGroupName:
                      description: BaseDomainResourceGroupName specifies the resource
                        group where the azure DNS zone for the base domain is found
                      type: string
                    credentialsSecretRef:
                      description: CredentialsSecretRef refers to a secret that contains
                        the Azure account access credentials.
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    region:
                      description: Region specifies the Azure region where the cluster
                        will be created.
                      type: string
                baremetal:
                  description: BareMetal is the configuration used when installing
                    on bare metal.
                  type: object
                  required:
                  - libvirtSSHPrivateKeySecretRef
                  properties:
                    libvirtSSHPrivateKeySecretRef:
                      description: LibvirtSSHPrivateKeySecretRef is the reference
                        to the secret that contains the private SSH key to use for
                        access to the libvirt provisioning host. The SSH private key
                        is expected to be in the secret data under the "ssh-privatekey"
                        key.
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                gcp:
                  description: GCP is the configuration used when installing on Google
                    Cloud Platform.
                  type: object
                  required:
                  - credentialsSecretRef
                  - region
                  properties:
                    credentialsSecretRef:
                      description: CredentialsSecretRef refers to a secret that contains
                        the GCP account access credentials.
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    region:
                      description: Region specifies the GCP region where the cluster
                        will be created.
                      type: string
                openstack:
                  description: OpenStack is the configuration used when installing
                    on OpenStack
                  type: object
                  required:
                  - cloud
                  - credentialsSecretRef
                  properties:
                    cloud:
                      description: Cloud will be used to indicate the OS_CLOUD value
                        to use the right section from the cloud.yaml in the CredentialsSecretRef.
                      type: string
                    credentialsSecretRef:
                      description: CredentialsSecretRef refers to a secret that contains
                        the OpenStack account access credentials.
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    trunkSupport:
                      description: TrunkSupport indicates whether or not to use trunk
                        ports in your OpenShift cluster.
                      type: boolean
            preserveOnDelete:
              description: PreserveOnDelete allows the user to disconnect a cluster
                from Hive without deprovisioning it
              type: boolean
            provisioning:
              description: Provisioning contains settings used only for initial cluster
                provisioning. May be unset in the case of adopted clusters.
              type: object
              required:
              - installConfigSecretRef
              properties:
                imageSetRef:
                  description: ImageSetRef is a reference to a ClusterImageSet. If
                    a value is specified for ReleaseImage, that will take precedence
                    over the one from the ClusterImageSet.
                  type: object
                  required:
                  - name
                  properties:
                    name:
                      description: Name is the name of the ClusterImageSet that this
                        refers to
                      type: string
                installConfigSecretRef:
                  description: InstallConfigSecretRef is the reference to a secret
                    that contains an openshift-install InstallConfig. This file will
                    be passed through directly to the installer. Any version of InstallConfig
                    can be used, provided it can be parsed by the openshift-install
                    version for the release you are provisioning.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
                installerEnv:
                  description: InstallerEnv are extra environment variables to pass
                    through to the installer. This may be used to enable additional
                    features of the installer.
                  type: array
                  items:
                    description: EnvVar represents an environment variable present
                      in a Container.
                    type: object
                    required:
                    - name
                    properties:
                      name:
                        description: Name of the environment variable. Must be a C_IDENTIFIER.
                        type: string
                      value:
                        description: 'Variable references $(VAR_NAME) are expanded
                          using the previous defined environment variables in the
                          container and any service environment variables. If a variable
                          cannot be resolved, the reference in the input string will
                          be unchanged. The $(VAR_NAME) syntax can be escaped with
                          a double $$, ie: $$(VAR_NAME). Escaped references will never
                          be expanded, regardless of whether the variable exists or
                          not. Defaults to "".'
                        type: string
                      valueFrom:
                        description: Source for the environment variable's value.
                          Cannot be used if value is not empty.
                        type: object
                        properties:
                          configMapKeyRef:
                            description: Selects a key of a ConfigMap.
                            type: object
                            required:
                            - key
                            properties:
                              key:
                                description: The key to select.
                                type: string
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                              optional:
                                description: Specify whether the ConfigMap or its
                                  key must be defined
                                type: boolean
                          fieldRef:
                            description: 'Selects a field of the pod: supports metadata.name,
                              metadata.namespace, metadata.labels, metadata.annotations,
                              spec.nodeName, spec.serviceAccountName, status.hostIP,
                              status.podIP, status.podIPs.'
                            type: object
                            required:
                            - fieldPath
                            properties:
                              apiVersion:
                                description: Version of the schema the FieldPath is
                                  written in terms of, defaults to "v1".
                                type: string
                              fieldPath:
                                description: Path of the field to select in the specified
                                  API version.
                                type: string
                          resourceFieldRef:
                            description: 'Selects a resource of the container: only
                              resources limits and requests (limits.cpu, limits.memory,
                              limits.ephemeral-storage, requests.cpu, requests.memory
                              and requests.ephemeral-storage) are currently supported.'
                            type: object
                            required:
                            - resource
                            properties:
                              containerName:
                                description: 'Container name: required for volumes,
                                  optional for env vars'
                                type: string
                              divisor:
                                description: Specifies the output format of the exposed
                                  resources, defaults to "1"
                                type: string
                              resource:
                                description: 'Required: resource to select'
                                type: string
                          secretKeyRef:
                            description: Selects a key of a secret in the pod's namespace
                            type: object
                            required:
                            - key
                            properties:
                              key:
                                description: The key of the secret to select from.  Must
                                  be a valid secret key.
                                type: string
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                              optional:
                                description: Specify whether the Secret or its key
                                  must be defined
                                type: boolean
                manifestsConfigMapRef:
                  description: ManifestsConfigMapRef is a reference to user-provided
                    manifests to add to or replace manifests that are generated by
                    the installer.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
                releaseImage:
                  description: ReleaseImage is the image containing metadata for all
                    components that run in the cluster, and is the primary and best
                    way to specify what specific version of OpenShift you wish to
                    install.
                  type: string
                sshKnownHosts:
                  description: SSHKnownHosts are known hosts to be configured in the
                    hive install manager pod to avoid ssh prompts. Use of ssh in the
                    install pod is somewhat limited today (failure log gathering from
                    cluster, some bare metal provisioning scenarios), so this setting
                    is often not needed.
                  type: array
                  items:
                    type: string
                sshPrivateKeySecretRef:
                  description: SSHPrivateKeySecretRef is the reference to the secret
                    that contains the private SSH key to use for access to compute
                    instances. This private key should correspond to the public key
                    included in the InstallConfig. The private key is used by Hive
                    to gather logs on the target cluster if there are install failures.
                    The SSH private key is expected to be in the secret data under
                    the "ssh-privatekey" key.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
            pullSecretRef:
              description: PullSecretRef is the reference to the secret to use when
                pulling images.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
        status:
          description: ClusterDeploymentStatus defines the observed state of ClusterDeployment
          type: object
          properties:
            apiURL:
              description: APIURL is the URL where the cluster's API can be accessed.
              type: string
            certificateBundles:
              description: CertificateBundles contains of the status of the certificate
                bundles associated with this cluster deployment.
              type: array
              items:
                description: CertificateBundleStatus specifies whether a certificate
                  bundle was generated for this cluster deployment.
                type: object
                required:
                - generated
                - name
                properties:
                  generated:
                    description: Generated indicates whether the certificate bundle
                      was generated
                    type: boolean
                  name:
                    description: Name of the certificate bundle
                    type: string
            cliImage:
              description: CLIImage is the name of the oc cli image to use when installing
                the target cluster
              type: string
            clusterVersionStatus:
              description: ClusterVersionStatus will hold a copy of the remote cluster's
                ClusterVersion.Status
              type: object
              required:
              - availableUpdates
              - desired
              - observedGeneration
              - versionHash
              properties:
                availableUpdates:
                  description: availableUpdates contains the list of updates that
                    are appropriate for this cluster. This list may be empty if no
                    updates are recommended, if the update service is unavailable,
                    or if an invalid channel has been specified.
                  type: array
                  items:
                    description: Update represents a release of the ClusterVersionOperator,
                      referenced by the Image member.
                    type: object
                    properties:
                      force:
                        description: "force allows an administrator to update to an
                          image that has failed verification, does not appear in the
                          availableUpdates list, or otherwise would be blocked by
                          normal protections on update. This option should only be
                          used when the authenticity of the provided image has been
                          verified out of band because the provided image will run
                          with full administrative access to the cluster. Do not use
                          this flag with images that comes from unknown or potentially
                          malicious sources. \n This flag does not override other
                          forms of consistency checking that are required before a
                          new update is deployed."
                        type: boolean
                      image:
                        description: image is a container image location that contains
                          the update. When this field is part of spec, image is optional
                          if version is specified and the availableUpdates field contains
                          a matching version.
                        type: string
                      version:
                        description: version is a semantic versioning identifying
                          the update version. When this field is part of spec, version
                          is optional if image is specified.
                        type: string
                  nullable: true
                conditions:
                  description: conditions provides information about the cluster version.
                    The condition "Available" is set to true if the desiredUpdate
                    has been reached. The condition "Progressing" is set to true if
                    an update is being applied. The condition "Degraded" is set to
                    true if an update is currently blocked by a temporary or permanent
                    error. Conditions are only valid for the current desiredUpdate
                    when metadata.generation is equal to status.generation.
                  type: array
                  items:
                    description: ClusterOperatorStatusCondition represents the state
                      of the operator's managed and monitored components.
                    type: object
                    required:
                    - lastTransitionTime
                    - status
                    - type
                    properties:
                      lastTransitionTime:
                        description: lastTransitionTime is the time of the last update
                          to the current status property.
                        type: string
                        format: date-time
                      message:
                        description: message provides additional information about
                          the current condition. This is only to be consumed by humans.
                        type: string
                      reason:
                        description: reason is the CamelCase reason for the condition's
                          current status.
                        type: string
                      status:
                        description: status of the condition, one of True, False,
                          Unknown.
                        type: string
                      type:
                        description: type specifies the aspect reported by this condition.
                        type: string
                desired:
                  description: desired is the version that the cluster is reconciling
                    towards. If the cluster is not yet fully initialized desired will
                    be set with the information available, which may be an image or
                    a tag.
                  type: object
                  properties:
                    force:
                      description: "force allows an administrator to update to an
                        image that has failed verification, does not appear in the
                        availableUpdates list, or otherwise would be blocked by normal
                        protections on update. This option should only be used when
                        the authenticity of the provided image has been verified out
                        of band because the provided image will run with full administrative
                        access to the cluster. Do not use this flag with images that
                        comes from unknown or potentially malicious sources. \n This
                        flag does not override other forms of consistency checking
                        that are required before a new update is deployed."
                      type: boolean
                    image:
                      description: image is a container image location that contains
                        the update. When this field is part of spec, image is optional
                        if version is specified and the availableUpdates field contains
                        a matching version.
                      type: string
                    version:
                      description: version is a semantic versioning identifying the
                        update version. When this field is part of spec, version is
                        optional if image is specified.
                      type: string
                history:
                  description: history contains a list of the most recent versions
                    applied to the cluster. This value may be empty during cluster
                    startup, and then will be updated when a new update is being applied.
                    The newest update is first in the list and it is ordered by recency.
                    Updates in the history have state Completed if the rollout completed
                    - if an update was failing or halfway applied the state will be
                    Partial. Only a limited amount of update history is preserved.
                  type: array
                  items:
                    description: UpdateHistory is a single attempted update to the
                      cluster.
                    type: object
                    required:
                    - completionTime
                    - image
                    - startedTime
                    - state
                    - verified
                    properties:
                      completionTime:
                        description: completionTime, if set, is when the update was
                          fully applied. The update that is currently being applied
                          will have a null completion time. Completion time will always
                          be set for entries that are not the current update (usually
                          to the started time of the next update).
                        type: string
                        format: date-time
                        nullable: true
                      image:
                        description: image is a container image location that contains
                          the update. This value is always populated.
                        type: string
                      startedTime:
                        description: startedTime is the time at which the update was
                          started.
                        type: string
                        format: date-time
                      state:
                        description: state reflects whether the update was fully applied.
                          The Partial state indicates the update is not fully applied,
                          while the Completed state indicates the update was successfully
                          rolled out at least once (all parts of the update successfully
                          applied).
                        type: string
                      verified:
                        description: verified indicates whether the provided update
                          was properly verified before it was installed. If this is
                          false the cluster may not be trusted.
                        type: boolean
                      version:
                        description: version is a semantic versioning identifying
                          the update version. If the requested image does not define
                          a version, or if a failure occurs retrieving the image,
                          this value may be empty.
                        type: string
                observedGeneration:
                  description: observedGeneration reports which version of the spec
                    is being synced. If this value is not equal to metadata.generation,
                    then the desired and conditions fields may represent a previous
                    version.
                  type: integer
                  format: int64
                versionHash:
                  description: versionHash is a fingerprint of the content that the
                    cluster will be updated with. It is used by the operator to avoid
                    unnecessary work and is for internal use only.
                  type: string
            conditions:
              description: Conditions includes more detailed status for the cluster
                deployment
              type: array
              items:
                description: ClusterDeploymentCondition contains details for the current
                  condition of a cluster deployment
                type: object
                required:
                - status
                - type
                properties:
                  lastProbeTime:
                    description: LastProbeTime is the last time we probed the condition.
                    type: string
                    format: date-time
                  lastTransitionTime:
                    description: LastTransitionTime is the last time the condition
                      transitioned from one status to another.
                    type: string
                    format: date-time
                  message:
                    description: Message is a human-readable message indicating details
                      about last transition.
                    type: string
                  reason:
                    description: Reason is a unique, one-word, CamelCase reason for
                      the condition's last transition.
                    type: string
                  status:
                    description: Status is the status of the condition.
                    type: string
                  type:
                    description: Type is the type of the condition.
                    type: string
            installRestarts:
              description: InstallRestarts is the total count of container restarts
                on the clusters install job.
              type: integer
            installedTimestamp:
              description: InstalledTimestamp is the time we first detected that the
                cluster has been successfully installed.
              type: string
              format: date-time
            installerImage:
              description: InstallerImage is the name of the installer image to use
                when installing the target cluster
              type: string
            provisionRef:
              description: ProvisionRef is a reference to the last ClusterProvision
                created for the deployment
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            webConsoleURL:
              description: WebConsoleURL is the URL for the cluster's web console
                UI.
              type: string
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_clusterdeploymentYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_clusterdeploymentYaml, nil
}

func configCrdsHive_v1_clusterdeploymentYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_clusterdeploymentYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_clusterdeployment.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_clusterdeprovisionYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusterdeprovisions.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.infraID
    name: InfraID
    type: string
  - JSONPath: .spec.clusterID
    name: ClusterID
    type: string
  - JSONPath: .status.completed
    name: Completed
    type: boolean
  - JSONPath: .metadata.creationTimestamp
    name: Age
    type: date
  group: hive.openshift.io
  names:
    kind: ClusterDeprovision
    plural: clusterdeprovisions
    shortNames:
    - cdr
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: ClusterDeprovision is the Schema for the clusterdeprovisions API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: ClusterDeprovisionSpec defines the desired state of ClusterDeprovision
          type: object
          required:
          - infraID
          properties:
            clusterID:
              description: ClusterID is a globally unique identifier for the cluster
                to deprovision. It will be used if specified.
              type: string
            infraID:
              description: InfraID is the identifier generated during installation
                for a cluster. It is used for tagging/naming resources in cloud providers.
              type: string
            platform:
              description: Platform contains platform-specific configuration for a
                ClusterDeprovision
              type: object
              properties:
                aws:
                  description: AWS contains AWS-specific deprovision settings
                  type: object
                  required:
                  - region
                  properties:
                    credentialsSecretRef:
                      description: CredentialsSecretRef is the AWS account credentials
                        to use for deprovisioning the cluster
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    region:
                      description: Region is the AWS region for this deprovisioning
                      type: string
                azure:
                  description: Azure contains Azure-specific deprovision settings
                  type: object
                  properties:
                    credentialsSecretRef:
                      description: CredentialsSecretRef is the Azure account credentials
                        to use for deprovisioning the cluster
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                gcp:
                  description: GCP contains GCP-specific deprovision settings
                  type: object
                  required:
                  - region
                  properties:
                    credentialsSecretRef:
                      description: CredentialsSecretRef is the GCP account credentials
                        to use for deprovisioning the cluster
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
                    region:
                      description: Region is the GCP region for this deprovision
                      type: string
                openstack:
                  description: OpenStack contains OpenStack-specific deprovision settings
                  type: object
                  required:
                  - cloud
                  properties:
                    cloud:
                      description: Cloud is the secion in the clouds.yaml secret below
                        to use for auth/connectivity.
                      type: string
                    credentialsSecretRef:
                      description: CredentialsSecretRef is the OpenStack account credentials
                        to use for deprovisioning the cluster
                      type: object
                      properties:
                        name:
                          description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                            TODO: Add other useful fields. apiVersion, kind, uid?'
                          type: string
        status:
          description: ClusterDeprovisionStatus defines the observed state of ClusterDeprovision
          type: object
          properties:
            completed:
              description: Completed is true when the uninstall has completed successfully
              type: boolean
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_clusterdeprovisionYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_clusterdeprovisionYaml, nil
}

func configCrdsHive_v1_clusterdeprovisionYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_clusterdeprovisionYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_clusterdeprovision.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_clusterimagesetYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusterimagesets.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.releaseImage
    name: Release
    type: string
  group: hive.openshift.io
  names:
    kind: ClusterImageSet
    plural: clusterimagesets
    shortNames:
    - imgset
  scope: Cluster
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: ClusterImageSet is the Schema for the clusterimagesets API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: ClusterImageSetSpec defines the desired state of ClusterImageSet
          type: object
          required:
          - releaseImage
          properties:
            releaseImage:
              description: ReleaseImage is the image that contains the payload to
                use when installing a cluster.
              type: string
        status:
          description: ClusterImageSetStatus defines the observed state of ClusterImageSet
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_clusterimagesetYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_clusterimagesetYaml, nil
}

func configCrdsHive_v1_clusterimagesetYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_clusterimagesetYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_clusterimageset.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_clusterprovisionYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusterprovisions.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.clusterDeploymentRef.name
    name: ClusterDeployment
    type: string
  - JSONPath: .spec.stage
    name: Stage
    type: string
  - JSONPath: .spec.infraID
    name: InfraID
    type: string
  group: hive.openshift.io
  names:
    kind: ClusterProvision
    plural: clusterprovisions
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: ClusterProvision is the Schema for the clusterprovisions API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: ClusterProvisionSpec defines the results of provisioning a
            cluster.
          type: object
          required:
          - attempt
          - clusterDeploymentRef
          - podSpec
          - stage
          properties:
            adminKubeconfigSecretRef:
              description: AdminKubeconfigSecretRef references the secret containing
                the admin kubeconfig for this cluster.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            adminPasswordSecretRef:
              description: AdminPasswordSecretRef references the secret containing
                the admin username/password which can be used to login to this cluster.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            attempt:
              description: Attempt is which attempt number of the cluster deployment
                that this ClusterProvision is
              type: integer
            clusterDeploymentRef:
              description: ClusterDeploymentRef references the cluster deployment
                provisioned.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            clusterID:
              description: ClusterID is a globally unique identifier for this cluster
                generated during installation. Used for reporting metrics among other
                places.
              type: string
            infraID:
              description: InfraID is an identifier for this cluster generated during
                installation and used for tagging/naming resources in cloud providers.
              type: string
            installLog:
              description: InstallLog is the log from the installer.
              type: string
            metadata:
              description: Metadata is the metadata.json generated by the installer,
                providing metadata information about the cluster created.
              type: object
            podSpec:
              description: PodSpec is the spec to use for the installer pod.
              type: object
              required:
              - containers
              properties:
                activeDeadlineSeconds:
                  description: Optional duration in seconds the pod may be active
                    on the node relative to StartTime before the system will actively
                    try to mark it failed and kill associated containers. Value must
                    be a positive integer.
                  type: integer
                  format: int64
                affinity:
                  description: If specified, the pod's scheduling constraints
                  type: object
                  properties:
                    nodeAffinity:
                      description: Describes node affinity scheduling rules for the
                        pod.
                      type: object
                      properties:
                        preferredDuringSchedulingIgnoredDuringExecution:
                          description: The scheduler will prefer to schedule pods
                            to nodes that satisfy the affinity expressions specified
                            by this field, but it may choose a node that violates
                            one or more of the expressions. The node that is most
                            preferred is the one with the greatest sum of weights,
                            i.e. for each node that meets all of the scheduling requirements
                            (resource request, requiredDuringScheduling affinity expressions,
                            etc.), compute a sum by iterating through the elements
                            of this field and adding "weight" to the sum if the node
                            matches the corresponding matchExpressions; the node(s)
                            with the highest sum are the most preferred.
                          type: array
                          items:
                            description: An empty preferred scheduling term matches
                              all objects with implicit weight 0 (i.e. it's a no-op).
                              A null preferred scheduling term matches no objects
                              (i.e. is also a no-op).
                            type: object
                            required:
                            - preference
                            - weight
                            properties:
                              preference:
                                description: A node selector term, associated with
                                  the corresponding weight.
                                type: object
                                properties:
                                  matchExpressions:
                                    description: A list of node selector requirements
                                      by node's labels.
                                    type: array
                                    items:
                                      description: A node selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: The label key that the selector
                                            applies to.
                                          type: string
                                        operator:
                                          description: Represents a key's relationship
                                            to a set of values. Valid operators are
                                            In, NotIn, Exists, DoesNotExist. Gt, and
                                            Lt.
                                          type: string
                                        values:
                                          description: An array of string values.
                                            If the operator is In or NotIn, the values
                                            array must be non-empty. If the operator
                                            is Exists or DoesNotExist, the values
                                            array must be empty. If the operator is
                                            Gt or Lt, the values array must have a
                                            single element, which will be interpreted
                                            as an integer. This array is replaced
                                            during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                                  matchFields:
                                    description: A list of node selector requirements
                                      by node's fields.
                                    type: array
                                    items:
                                      description: A node selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: The label key that the selector
                                            applies to.
                                          type: string
                                        operator:
                                          description: Represents a key's relationship
                                            to a set of values. Valid operators are
                                            In, NotIn, Exists, DoesNotExist. Gt, and
                                            Lt.
                                          type: string
                                        values:
                                          description: An array of string values.
                                            If the operator is In or NotIn, the values
                                            array must be non-empty. If the operator
                                            is Exists or DoesNotExist, the values
                                            array must be empty. If the operator is
                                            Gt or Lt, the values array must have a
                                            single element, which will be interpreted
                                            as an integer. This array is replaced
                                            during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                              weight:
                                description: Weight associated with matching the corresponding
                                  nodeSelectorTerm, in the range 1-100.
                                type: integer
                                format: int32
                        requiredDuringSchedulingIgnoredDuringExecution:
                          description: If the affinity requirements specified by this
                            field are not met at scheduling time, the pod will not
                            be scheduled onto the node. If the affinity requirements
                            specified by this field cease to be met at some point
                            during pod execution (e.g. due to an update), the system
                            may or may not try to eventually evict the pod from its
                            node.
                          type: object
                          required:
                          - nodeSelectorTerms
                          properties:
                            nodeSelectorTerms:
                              description: Required. A list of node selector terms.
                                The terms are ORed.
                              type: array
                              items:
                                description: A null or empty node selector term matches
                                  no objects. The requirements of them are ANDed.
                                  The TopologySelectorTerm type implements a subset
                                  of the NodeSelectorTerm.
                                type: object
                                properties:
                                  matchExpressions:
                                    description: A list of node selector requirements
                                      by node's labels.
                                    type: array
                                    items:
                                      description: A node selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: The label key that the selector
                                            applies to.
                                          type: string
                                        operator:
                                          description: Represents a key's relationship
                                            to a set of values. Valid operators are
                                            In, NotIn, Exists, DoesNotExist. Gt, and
                                            Lt.
                                          type: string
                                        values:
                                          description: An array of string values.
                                            If the operator is In or NotIn, the values
                                            array must be non-empty. If the operator
                                            is Exists or DoesNotExist, the values
                                            array must be empty. If the operator is
                                            Gt or Lt, the values array must have a
                                            single element, which will be interpreted
                                            as an integer. This array is replaced
                                            during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                                  matchFields:
                                    description: A list of node selector requirements
                                      by node's fields.
                                    type: array
                                    items:
                                      description: A node selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: The label key that the selector
                                            applies to.
                                          type: string
                                        operator:
                                          description: Represents a key's relationship
                                            to a set of values. Valid operators are
                                            In, NotIn, Exists, DoesNotExist. Gt, and
                                            Lt.
                                          type: string
                                        values:
                                          description: An array of string values.
                                            If the operator is In or NotIn, the values
                                            array must be non-empty. If the operator
                                            is Exists or DoesNotExist, the values
                                            array must be empty. If the operator is
                                            Gt or Lt, the values array must have a
                                            single element, which will be interpreted
                                            as an integer. This array is replaced
                                            during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                    podAffinity:
                      description: Describes pod affinity scheduling rules (e.g. co-locate
                        this pod in the same node, zone, etc. as some other pod(s)).
                      type: object
                      properties:
                        preferredDuringSchedulingIgnoredDuringExecution:
                          description: The scheduler will prefer to schedule pods
                            to nodes that satisfy the affinity expressions specified
                            by this field, but it may choose a node that violates
                            one or more of the expressions. The node that is most
                            preferred is the one with the greatest sum of weights,
                            i.e. for each node that meets all of the scheduling requirements
                            (resource request, requiredDuringScheduling affinity expressions,
                            etc.), compute a sum by iterating through the elements
                            of this field and adding "weight" to the sum if the node
                            has pods which matches the corresponding podAffinityTerm;
                            the node(s) with the highest sum are the most preferred.
                          type: array
                          items:
                            description: The weights of all of the matched WeightedPodAffinityTerm
                              fields are added per-node to find the most preferred
                              node(s)
                            type: object
                            required:
                            - podAffinityTerm
                            - weight
                            properties:
                              podAffinityTerm:
                                description: Required. A pod affinity term, associated
                                  with the corresponding weight.
                                type: object
                                required:
                                - topologyKey
                                properties:
                                  labelSelector:
                                    description: A label query over a set of resources,
                                      in this case pods.
                                    type: object
                                    properties:
                                      matchExpressions:
                                        description: matchExpressions is a list of
                                          label selector requirements. The requirements
                                          are ANDed.
                                        type: array
                                        items:
                                          description: A label selector requirement
                                            is a selector that contains values, a
                                            key, and an operator that relates the
                                            key and values.
                                          type: object
                                          required:
                                          - key
                                          - operator
                                          properties:
                                            key:
                                              description: key is the label key that
                                                the selector applies to.
                                              type: string
                                            operator:
                                              description: operator represents a key's
                                                relationship to a set of values. Valid
                                                operators are In, NotIn, Exists and
                                                DoesNotExist.
                                              type: string
                                            values:
                                              description: values is an array of string
                                                values. If the operator is In or NotIn,
                                                the values array must be non-empty.
                                                If the operator is Exists or DoesNotExist,
                                                the values array must be empty. This
                                                array is replaced during a strategic
                                                merge patch.
                                              type: array
                                              items:
                                                type: string
                                      matchLabels:
                                        description: matchLabels is a map of {key,value}
                                          pairs. A single {key,value} in the matchLabels
                                          map is equivalent to an element of matchExpressions,
                                          whose key field is "key", the operator is
                                          "In", and the values array contains only
                                          "value". The requirements are ANDed.
                                        type: object
                                        additionalProperties:
                                          type: string
                                  namespaces:
                                    description: namespaces specifies which namespaces
                                      the labelSelector applies to (matches against);
                                      null or empty list means "this pod's namespace"
                                    type: array
                                    items:
                                      type: string
                                  topologyKey:
                                    description: This pod should be co-located (affinity)
                                      or not co-located (anti-affinity) with the pods
                                      matching the labelSelector in the specified
                                      namespaces, where co-located is defined as running
                                      on a node whose value of the label with key
                                      topologyKey matches that of any node on which
                                      any of the selected pods is running. Empty topologyKey
                                      is not allowed.
                                    type: string
                              weight:
                                description: weight associated with matching the corresponding
                                  podAffinityTerm, in the range 1-100.
                                type: integer
                                format: int32
                        requiredDuringSchedulingIgnoredDuringExecution:
                          description: If the affinity requirements specified by this
                            field are not met at scheduling time, the pod will not
                            be scheduled onto the node. If the affinity requirements
                            specified by this field cease to be met at some point
                            during pod execution (e.g. due to a pod label update),
                            the system may or may not try to eventually evict the
                            pod from its node. When there are multiple elements, the
                            lists of nodes corresponding to each podAffinityTerm are
                            intersected, i.e. all terms must be satisfied.
                          type: array
                          items:
                            description: Defines a set of pods (namely those matching
                              the labelSelector relative to the given namespace(s))
                              that this pod should be co-located (affinity) or not
                              co-located (anti-affinity) with, where co-located is
                              defined as running on a node whose value of the label
                              with key <topologyKey> matches that of any node on which
                              a pod of the set of pods is running
                            type: object
                            required:
                            - topologyKey
                            properties:
                              labelSelector:
                                description: A label query over a set of resources,
                                  in this case pods.
                                type: object
                                properties:
                                  matchExpressions:
                                    description: matchExpressions is a list of label
                                      selector requirements. The requirements are
                                      ANDed.
                                    type: array
                                    items:
                                      description: A label selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: key is the label key that the
                                            selector applies to.
                                          type: string
                                        operator:
                                          description: operator represents a key's
                                            relationship to a set of values. Valid
                                            operators are In, NotIn, Exists and DoesNotExist.
                                          type: string
                                        values:
                                          description: values is an array of string
                                            values. If the operator is In or NotIn,
                                            the values array must be non-empty. If
                                            the operator is Exists or DoesNotExist,
                                            the values array must be empty. This array
                                            is replaced during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                                  matchLabels:
                                    description: matchLabels is a map of {key,value}
                                      pairs. A single {key,value} in the matchLabels
                                      map is equivalent to an element of matchExpressions,
                                      whose key field is "key", the operator is "In",
                                      and the values array contains only "value".
                                      The requirements are ANDed.
                                    type: object
                                    additionalProperties:
                                      type: string
                              namespaces:
                                description: namespaces specifies which namespaces
                                  the labelSelector applies to (matches against);
                                  null or empty list means "this pod's namespace"
                                type: array
                                items:
                                  type: string
                              topologyKey:
                                description: This pod should be co-located (affinity)
                                  or not co-located (anti-affinity) with the pods
                                  matching the labelSelector in the specified namespaces,
                                  where co-located is defined as running on a node
                                  whose value of the label with key topologyKey matches
                                  that of any node on which any of the selected pods
                                  is running. Empty topologyKey is not allowed.
                                type: string
                    podAntiAffinity:
                      description: Describes pod anti-affinity scheduling rules (e.g.
                        avoid putting this pod in the same node, zone, etc. as some
                        other pod(s)).
                      type: object
                      properties:
                        preferredDuringSchedulingIgnoredDuringExecution:
                          description: The scheduler will prefer to schedule pods
                            to nodes that satisfy the anti-affinity expressions specified
                            by this field, but it may choose a node that violates
                            one or more of the expressions. The node that is most
                            preferred is the one with the greatest sum of weights,
                            i.e. for each node that meets all of the scheduling requirements
                            (resource request, requiredDuringScheduling anti-affinity
                            expressions, etc.), compute a sum by iterating through
                            the elements of this field and adding "weight" to the
                            sum if the node has pods which matches the corresponding
                            podAffinityTerm; the node(s) with the highest sum are
                            the most preferred.
                          type: array
                          items:
                            description: The weights of all of the matched WeightedPodAffinityTerm
                              fields are added per-node to find the most preferred
                              node(s)
                            type: object
                            required:
                            - podAffinityTerm
                            - weight
                            properties:
                              podAffinityTerm:
                                description: Required. A pod affinity term, associated
                                  with the corresponding weight.
                                type: object
                                required:
                                - topologyKey
                                properties:
                                  labelSelector:
                                    description: A label query over a set of resources,
                                      in this case pods.
                                    type: object
                                    properties:
                                      matchExpressions:
                                        description: matchExpressions is a list of
                                          label selector requirements. The requirements
                                          are ANDed.
                                        type: array
                                        items:
                                          description: A label selector requirement
                                            is a selector that contains values, a
                                            key, and an operator that relates the
                                            key and values.
                                          type: object
                                          required:
                                          - key
                                          - operator
                                          properties:
                                            key:
                                              description: key is the label key that
                                                the selector applies to.
                                              type: string
                                            operator:
                                              description: operator represents a key's
                                                relationship to a set of values. Valid
                                                operators are In, NotIn, Exists and
                                                DoesNotExist.
                                              type: string
                                            values:
                                              description: values is an array of string
                                                values. If the operator is In or NotIn,
                                                the values array must be non-empty.
                                                If the operator is Exists or DoesNotExist,
                                                the values array must be empty. This
                                                array is replaced during a strategic
                                                merge patch.
                                              type: array
                                              items:
                                                type: string
                                      matchLabels:
                                        description: matchLabels is a map of {key,value}
                                          pairs. A single {key,value} in the matchLabels
                                          map is equivalent to an element of matchExpressions,
                                          whose key field is "key", the operator is
                                          "In", and the values array contains only
                                          "value". The requirements are ANDed.
                                        type: object
                                        additionalProperties:
                                          type: string
                                  namespaces:
                                    description: namespaces specifies which namespaces
                                      the labelSelector applies to (matches against);
                                      null or empty list means "this pod's namespace"
                                    type: array
                                    items:
                                      type: string
                                  topologyKey:
                                    description: This pod should be co-located (affinity)
                                      or not co-located (anti-affinity) with the pods
                                      matching the labelSelector in the specified
                                      namespaces, where co-located is defined as running
                                      on a node whose value of the label with key
                                      topologyKey matches that of any node on which
                                      any of the selected pods is running. Empty topologyKey
                                      is not allowed.
                                    type: string
                              weight:
                                description: weight associated with matching the corresponding
                                  podAffinityTerm, in the range 1-100.
                                type: integer
                                format: int32
                        requiredDuringSchedulingIgnoredDuringExecution:
                          description: If the anti-affinity requirements specified
                            by this field are not met at scheduling time, the pod
                            will not be scheduled onto the node. If the anti-affinity
                            requirements specified by this field cease to be met at
                            some point during pod execution (e.g. due to a pod label
                            update), the system may or may not try to eventually evict
                            the pod from its node. When there are multiple elements,
                            the lists of nodes corresponding to each podAffinityTerm
                            are intersected, i.e. all terms must be satisfied.
                          type: array
                          items:
                            description: Defines a set of pods (namely those matching
                              the labelSelector relative to the given namespace(s))
                              that this pod should be co-located (affinity) or not
                              co-located (anti-affinity) with, where co-located is
                              defined as running on a node whose value of the label
                              with key <topologyKey> matches that of any node on which
                              a pod of the set of pods is running
                            type: object
                            required:
                            - topologyKey
                            properties:
                              labelSelector:
                                description: A label query over a set of resources,
                                  in this case pods.
                                type: object
                                properties:
                                  matchExpressions:
                                    description: matchExpressions is a list of label
                                      selector requirements. The requirements are
                                      ANDed.
                                    type: array
                                    items:
                                      description: A label selector requirement is
                                        a selector that contains values, a key, and
                                        an operator that relates the key and values.
                                      type: object
                                      required:
                                      - key
                                      - operator
                                      properties:
                                        key:
                                          description: key is the label key that the
                                            selector applies to.
                                          type: string
                                        operator:
                                          description: operator represents a key's
                                            relationship to a set of values. Valid
                                            operators are In, NotIn, Exists and DoesNotExist.
                                          type: string
                                        values:
                                          description: values is an array of string
                                            values. If the operator is In or NotIn,
                                            the values array must be non-empty. If
                                            the operator is Exists or DoesNotExist,
                                            the values array must be empty. This array
                                            is replaced during a strategic merge patch.
                                          type: array
                                          items:
                                            type: string
                                  matchLabels:
                                    description: matchLabels is a map of {key,value}
                                      pairs. A single {key,value} in the matchLabels
                                      map is equivalent to an element of matchExpressions,
                                      whose key field is "key", the operator is "In",
                                      and the values array contains only "value".
                                      The requirements are ANDed.
                                    type: object
                                    additionalProperties:
                                      type: string
                              namespaces:
                                description: namespaces specifies which namespaces
                                  the labelSelector applies to (matches against);
                                  null or empty list means "this pod's namespace"
                                type: array
                                items:
                                  type: string
                              topologyKey:
                                description: This pod should be co-located (affinity)
                                  or not co-located (anti-affinity) with the pods
                                  matching the labelSelector in the specified namespaces,
                                  where co-located is defined as running on a node
                                  whose value of the label with key topologyKey matches
                                  that of any node on which any of the selected pods
                                  is running. Empty topologyKey is not allowed.
                                type: string
                automountServiceAccountToken:
                  description: AutomountServiceAccountToken indicates whether a service
                    account token should be automatically mounted.
                  type: boolean
                containers:
                  description: List of containers belonging to the pod. Containers
                    cannot currently be added or removed. There must be at least one
                    container in a Pod. Cannot be updated.
                  type: array
                  items:
                    description: A single application container that you want to run
                      within a pod.
                    type: object
                    required:
                    - name
                    properties:
                      args:
                        description: 'Arguments to the entrypoint. The docker image''s
                          CMD is used if this is not provided. Variable references
                          $(VAR_NAME) are expanded using the container''s environment.
                          If a variable cannot be resolved, the reference in the input
                          string will be unchanged. The $(VAR_NAME) syntax can be
                          escaped with a double $$, ie: $$(VAR_NAME). Escaped references
                          will never be expanded, regardless of whether the variable
                          exists or not. Cannot be updated. More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      command:
                        description: 'Entrypoint array. Not executed within a shell.
                          The docker image''s ENTRYPOINT is used if this is not provided.
                          Variable references $(VAR_NAME) are expanded using the container''s
                          environment. If a variable cannot be resolved, the reference
                          in the input string will be unchanged. The $(VAR_NAME) syntax
                          can be escaped with a double $$, ie: $$(VAR_NAME). Escaped
                          references will never be expanded, regardless of whether
                          the variable exists or not. Cannot be updated. More info:
                          https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      env:
                        description: List of environment variables to set in the container.
                          Cannot be updated.
                        type: array
                        items:
                          description: EnvVar represents an environment variable present
                            in a Container.
                          type: object
                          required:
                          - name
                          properties:
                            name:
                              description: Name of the environment variable. Must
                                be a C_IDENTIFIER.
                              type: string
                            value:
                              description: 'Variable references $(VAR_NAME) are expanded
                                using the previous defined environment variables in
                                the container and any service environment variables.
                                If a variable cannot be resolved, the reference in
                                the input string will be unchanged. The $(VAR_NAME)
                                syntax can be escaped with a double $$, ie: $$(VAR_NAME).
                                Escaped references will never be expanded, regardless
                                of whether the variable exists or not. Defaults to
                                "".'
                              type: string
                            valueFrom:
                              description: Source for the environment variable's value.
                                Cannot be used if value is not empty.
                              type: object
                              properties:
                                configMapKeyRef:
                                  description: Selects a key of a ConfigMap.
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key to select.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the ConfigMap or
                                        its key must be defined
                                      type: boolean
                                fieldRef:
                                  description: 'Selects a field of the pod: supports
                                    metadata.name, metadata.namespace, metadata.labels,
                                    metadata.annotations, spec.nodeName, spec.serviceAccountName,
                                    status.hostIP, status.podIP, status.podIPs.'
                                  type: object
                                  required:
                                  - fieldPath
                                  properties:
                                    apiVersion:
                                      description: Version of the schema the FieldPath
                                        is written in terms of, defaults to "v1".
                                      type: string
                                    fieldPath:
                                      description: Path of the field to select in
                                        the specified API version.
                                      type: string
                                resourceFieldRef:
                                  description: 'Selects a resource of the container:
                                    only resources limits and requests (limits.cpu,
                                    limits.memory, limits.ephemeral-storage, requests.cpu,
                                    requests.memory and requests.ephemeral-storage)
                                    are currently supported.'
                                  type: object
                                  required:
                                  - resource
                                  properties:
                                    containerName:
                                      description: 'Container name: required for volumes,
                                        optional for env vars'
                                      type: string
                                    divisor:
                                      description: Specifies the output format of
                                        the exposed resources, defaults to "1"
                                      type: string
                                    resource:
                                      description: 'Required: resource to select'
                                      type: string
                                secretKeyRef:
                                  description: Selects a key of a secret in the pod's
                                    namespace
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key of the secret to select
                                        from.  Must be a valid secret key.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the Secret or its
                                        key must be defined
                                      type: boolean
                      envFrom:
                        description: List of sources to populate environment variables
                          in the container. The keys defined within a source must
                          be a C_IDENTIFIER. All invalid keys will be reported as
                          an event when the container is starting. When a key exists
                          in multiple sources, the value associated with the last
                          source will take precedence. Values defined by an Env with
                          a duplicate key will take precedence. Cannot be updated.
                        type: array
                        items:
                          description: EnvFromSource represents the source of a set
                            of ConfigMaps
                          type: object
                          properties:
                            configMapRef:
                              description: The ConfigMap to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the ConfigMap must
                                    be defined
                                  type: boolean
                            prefix:
                              description: An optional identifier to prepend to each
                                key in the ConfigMap. Must be a C_IDENTIFIER.
                              type: string
                            secretRef:
                              description: The Secret to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the Secret must be
                                    defined
                                  type: boolean
                      image:
                        description: 'Docker image name. More info: https://kubernetes.io/docs/concepts/containers/images
                          This field is optional to allow higher level config management
                          to default or override container images in workload controllers
                          like Deployments and StatefulSets.'
                        type: string
                      imagePullPolicy:
                        description: 'Image pull policy. One of Always, Never, IfNotPresent.
                          Defaults to Always if :latest tag is specified, or IfNotPresent
                          otherwise. Cannot be updated. More info: https://kubernetes.io/docs/concepts/containers/images#updating-images'
                        type: string
                      lifecycle:
                        description: Actions that the management system should take
                          in response to container lifecycle events. Cannot be updated.
                        type: object
                        properties:
                          postStart:
                            description: 'PostStart is called immediately after a
                              container is created. If the handler fails, the container
                              is terminated and restarted according to its restart
                              policy. Other management of the container blocks until
                              the hook completes. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                          preStop:
                            description: 'PreStop is called immediately before a container
                              is terminated due to an API request or management event
                              such as liveness/startup probe failure, preemption,
                              resource contention, etc. The handler is not called
                              if the container crashes or exits. The reason for termination
                              is passed to the handler. The Pod''s termination grace
                              period countdown begins before the PreStop hooked is
                              executed. Regardless of the outcome of the handler,
                              the container will eventually terminate within the Pod''s
                              termination grace period. Other management of the container
                              blocks until the hook completes or until the termination
                              grace period is reached. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                      livenessProbe:
                        description: 'Periodic probe of container liveness. Container
                          will be restarted if the probe fails. Cannot be updated.
                          More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      name:
                        description: Name of the container specified as a DNS_LABEL.
                          Each container in a pod must have a unique name (DNS_LABEL).
                          Cannot be updated.
                        type: string
                      ports:
                        description: List of ports to expose from the container. Exposing
                          a port here gives the system additional information about
                          the network connections a container uses, but is primarily
                          informational. Not specifying a port here DOES NOT prevent
                          that port from being exposed. Any port which is listening
                          on the default "0.0.0.0" address inside a container will
                          be accessible from the network. Cannot be updated.
                        type: array
                        items:
                          description: ContainerPort represents a network port in
                            a single container.
                          type: object
                          required:
                          - containerPort
                          properties:
                            containerPort:
                              description: Number of port to expose on the pod's IP
                                address. This must be a valid port number, 0 < x <
                                65536.
                              type: integer
                              format: int32
                            hostIP:
                              description: What host IP to bind the external port
                                to.
                              type: string
                            hostPort:
                              description: Number of port to expose on the host. If
                                specified, this must be a valid port number, 0 < x
                                < 65536. If HostNetwork is specified, this must match
                                ContainerPort. Most containers do not need this.
                              type: integer
                              format: int32
                            name:
                              description: If specified, this must be an IANA_SVC_NAME
                                and unique within the pod. Each named port in a pod
                                must have a unique name. Name for the port that can
                                be referred to by services.
                              type: string
                            protocol:
                              description: Protocol for port. Must be UDP, TCP, or
                                SCTP. Defaults to "TCP".
                              type: string
                      readinessProbe:
                        description: 'Periodic probe of container service readiness.
                          Container will be removed from service endpoints if the
                          probe fails. Cannot be updated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      resources:
                        description: 'Compute Resources required by this container.
                          Cannot be updated. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                        type: object
                        properties:
                          limits:
                            description: 'Limits describes the maximum amount of compute
                              resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                          requests:
                            description: 'Requests describes the minimum amount of
                              compute resources required. If Requests is omitted for
                              a container, it defaults to Limits if that is explicitly
                              specified, otherwise to an implementation-defined value.
                              More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                      securityContext:
                        description: 'Security options the pod should run with. More
                          info: https://kubernetes.io/docs/concepts/policy/security-context/
                          More info: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/'
                        type: object
                        properties:
                          allowPrivilegeEscalation:
                            description: 'AllowPrivilegeEscalation controls whether
                              a process can gain more privileges than its parent process.
                              This bool directly controls if the no_new_privs flag
                              will be set on the container process. AllowPrivilegeEscalation
                              is true always when the container is: 1) run as Privileged
                              2) has CAP_SYS_ADMIN'
                            type: boolean
                          capabilities:
                            description: The capabilities to add/drop when running
                              containers. Defaults to the default set of capabilities
                              granted by the container runtime.
                            type: object
                            properties:
                              add:
                                description: Added capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                              drop:
                                description: Removed capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                          privileged:
                            description: Run container in privileged mode. Processes
                              in privileged containers are essentially equivalent
                              to root on the host. Defaults to false.
                            type: boolean
                          procMount:
                            description: procMount denotes the type of proc mount
                              to use for the containers. The default is DefaultProcMount
                              which uses the container runtime defaults for readonly
                              paths and masked paths. This requires the ProcMountType
                              feature flag to be enabled.
                            type: string
                          readOnlyRootFilesystem:
                            description: Whether this container has a read-only root
                              filesystem. Default is false.
                            type: boolean
                          runAsGroup:
                            description: The GID to run the entrypoint of the container
                              process. Uses runtime default if unset. May also be
                              set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: integer
                            format: int64
                          runAsNonRoot:
                            description: Indicates that the container must run as
                              a non-root user. If true, the Kubelet will validate
                              the image at runtime to ensure that it does not run
                              as UID 0 (root) and fail to start the container if it
                              does. If unset or false, no such validation will be
                              performed. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: boolean
                          runAsUser:
                            description: The UID to run the entrypoint of the container
                              process. Defaults to user specified in image metadata
                              if unspecified. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: integer
                            format: int64
                          seLinuxOptions:
                            description: The SELinux context to be applied to the
                              container. If unspecified, the container runtime will
                              allocate a random SELinux context for each container.  May
                              also be set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              level:
                                description: Level is SELinux level label that applies
                                  to the container.
                                type: string
                              role:
                                description: Role is a SELinux role label that applies
                                  to the container.
                                type: string
                              type:
                                description: Type is a SELinux type label that applies
                                  to the container.
                                type: string
                              user:
                                description: User is a SELinux user label that applies
                                  to the container.
                                type: string
                          windowsOptions:
                            description: The Windows specific settings applied to
                              all containers. If unspecified, the options from the
                              PodSecurityContext will be used. If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              gmsaCredentialSpec:
                                description: GMSACredentialSpec is where the GMSA
                                  admission webhook (https://github.com/kubernetes-sigs/windows-gmsa)
                                  inlines the contents of the GMSA credential spec
                                  named by the GMSACredentialSpecName field.
                                type: string
                              gmsaCredentialSpecName:
                                description: GMSACredentialSpecName is the name of
                                  the GMSA credential spec to use.
                                type: string
                              runAsUserName:
                                description: The UserName in Windows to run the entrypoint
                                  of the container process. Defaults to the user specified
                                  in image metadata if unspecified. May also be set
                                  in PodSecurityContext. If set in both SecurityContext
                                  and PodSecurityContext, the value specified in SecurityContext
                                  takes precedence.
                                type: string
                      startupProbe:
                        description: 'StartupProbe indicates that the Pod has successfully
                          initialized. If specified, no other probes are executed
                          until this completes successfully. If this probe fails,
                          the Pod will be restarted, just as if the livenessProbe
                          failed. This can be used to provide different probe parameters
                          at the beginning of a Pod''s lifecycle, when it might take
                          a long time to load data or warm a cache, than during steady-state
                          operation. This cannot be updated. This is a beta feature
                          enabled by the StartupProbe feature flag. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      stdin:
                        description: Whether this container should allocate a buffer
                          for stdin in the container runtime. If this is not set,
                          reads from stdin in the container will always result in
                          EOF. Default is false.
                        type: boolean
                      stdinOnce:
                        description: Whether the container runtime should close the
                          stdin channel after it has been opened by a single attach.
                          When stdin is true the stdin stream will remain open across
                          multiple attach sessions. If stdinOnce is set to true, stdin
                          is opened on container start, is empty until the first client
                          attaches to stdin, and then remains open and accepts data
                          until the client disconnects, at which time stdin is closed
                          and remains closed until the container is restarted. If
                          this flag is false, a container processes that reads from
                          stdin will never receive an EOF. Default is false
                        type: boolean
                      terminationMessagePath:
                        description: 'Optional: Path at which the file to which the
                          container''s termination message will be written is mounted
                          into the container''s filesystem. Message written is intended
                          to be brief final status, such as an assertion failure message.
                          Will be truncated by the node if greater than 4096 bytes.
                          The total message length across all containers will be limited
                          to 12kb. Defaults to /dev/termination-log. Cannot be updated.'
                        type: string
                      terminationMessagePolicy:
                        description: Indicate how the termination message should be
                          populated. File will use the contents of terminationMessagePath
                          to populate the container status message on both success
                          and failure. FallbackToLogsOnError will use the last chunk
                          of container log output if the termination message file
                          is empty and the container exited with an error. The log
                          output is limited to 2048 bytes or 80 lines, whichever is
                          smaller. Defaults to File. Cannot be updated.
                        type: string
                      tty:
                        description: Whether this container should allocate a TTY
                          for itself, also requires 'stdin' to be true. Default is
                          false.
                        type: boolean
                      volumeDevices:
                        description: volumeDevices is the list of block devices to
                          be used by the container.
                        type: array
                        items:
                          description: volumeDevice describes a mapping of a raw block
                            device within a container.
                          type: object
                          required:
                          - devicePath
                          - name
                          properties:
                            devicePath:
                              description: devicePath is the path inside of the container
                                that the device will be mapped to.
                              type: string
                            name:
                              description: name must match the name of a persistentVolumeClaim
                                in the pod
                              type: string
                      volumeMounts:
                        description: Pod volumes to mount into the container's filesystem.
                          Cannot be updated.
                        type: array
                        items:
                          description: VolumeMount describes a mounting of a Volume
                            within a container.
                          type: object
                          required:
                          - mountPath
                          - name
                          properties:
                            mountPath:
                              description: Path within the container at which the
                                volume should be mounted.  Must not contain ':'.
                              type: string
                            mountPropagation:
                              description: mountPropagation determines how mounts
                                are propagated from the host to container and the
                                other way around. When not set, MountPropagationNone
                                is used. This field is beta in 1.10.
                              type: string
                            name:
                              description: This must match the Name of a Volume.
                              type: string
                            readOnly:
                              description: Mounted read-only if true, read-write otherwise
                                (false or unspecified). Defaults to false.
                              type: boolean
                            subPath:
                              description: Path within the volume from which the container's
                                volume should be mounted. Defaults to "" (volume's
                                root).
                              type: string
                            subPathExpr:
                              description: Expanded path within the volume from which
                                the container's volume should be mounted. Behaves
                                similarly to SubPath but environment variable references
                                $(VAR_NAME) are expanded using the container's environment.
                                Defaults to "" (volume's root). SubPathExpr and SubPath
                                are mutually exclusive.
                              type: string
                      workingDir:
                        description: Container's working directory. If not specified,
                          the container runtime's default will be used, which might
                          be configured in the container image. Cannot be updated.
                        type: string
                dnsConfig:
                  description: Specifies the DNS parameters of a pod. Parameters specified
                    here will be merged to the generated DNS configuration based on
                    DNSPolicy.
                  type: object
                  properties:
                    nameservers:
                      description: A list of DNS name server IP addresses. This will
                        be appended to the base nameservers generated from DNSPolicy.
                        Duplicated nameservers will be removed.
                      type: array
                      items:
                        type: string
                    options:
                      description: A list of DNS resolver options. This will be merged
                        with the base options generated from DNSPolicy. Duplicated
                        entries will be removed. Resolution options given in Options
                        will override those that appear in the base DNSPolicy.
                      type: array
                      items:
                        description: PodDNSConfigOption defines DNS resolver options
                          of a pod.
                        type: object
                        properties:
                          name:
                            description: Required.
                            type: string
                          value:
                            type: string
                    searches:
                      description: A list of DNS search domains for host-name lookup.
                        This will be appended to the base search paths generated from
                        DNSPolicy. Duplicated search paths will be removed.
                      type: array
                      items:
                        type: string
                dnsPolicy:
                  description: Set DNS policy for the pod. Defaults to "ClusterFirst".
                    Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default'
                    or 'None'. DNS parameters given in DNSConfig will be merged with
                    the policy selected with DNSPolicy. To have DNS options set along
                    with hostNetwork, you have to specify DNS policy explicitly to
                    'ClusterFirstWithHostNet'.
                  type: string
                enableServiceLinks:
                  description: 'EnableServiceLinks indicates whether information about
                    services should be injected into pod''s environment variables,
                    matching the syntax of Docker links. Optional: Defaults to true.'
                  type: boolean
                ephemeralContainers:
                  description: List of ephemeral containers run in this pod. Ephemeral
                    containers may be run in an existing pod to perform user-initiated
                    actions such as debugging. This list cannot be specified when
                    creating a pod, and it cannot be modified by updating the pod
                    spec. In order to add an ephemeral container to an existing pod,
                    use the pod's ephemeralcontainers subresource. This field is alpha-level
                    and is only honored by servers that enable the EphemeralContainers
                    feature.
                  type: array
                  items:
                    description: An EphemeralContainer is a container that may be
                      added temporarily to an existing pod for user-initiated activities
                      such as debugging. Ephemeral containers have no resource or
                      scheduling guarantees, and they will not be restarted when they
                      exit or when a pod is removed or restarted. If an ephemeral
                      container causes a pod to exceed its resource allocation, the
                      pod may be evicted. Ephemeral containers may not be added by
                      directly updating the pod spec. They must be added via the pod's
                      ephemeralcontainers subresource, and they will appear in the
                      pod spec once added. This is an alpha feature enabled by the
                      EphemeralContainers feature flag.
                    type: object
                    required:
                    - name
                    properties:
                      args:
                        description: 'Arguments to the entrypoint. The docker image''s
                          CMD is used if this is not provided. Variable references
                          $(VAR_NAME) are expanded using the container''s environment.
                          If a variable cannot be resolved, the reference in the input
                          string will be unchanged. The $(VAR_NAME) syntax can be
                          escaped with a double $$, ie: $$(VAR_NAME). Escaped references
                          will never be expanded, regardless of whether the variable
                          exists or not. Cannot be updated. More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      command:
                        description: 'Entrypoint array. Not executed within a shell.
                          The docker image''s ENTRYPOINT is used if this is not provided.
                          Variable references $(VAR_NAME) are expanded using the container''s
                          environment. If a variable cannot be resolved, the reference
                          in the input string will be unchanged. The $(VAR_NAME) syntax
                          can be escaped with a double $$, ie: $$(VAR_NAME). Escaped
                          references will never be expanded, regardless of whether
                          the variable exists or not. Cannot be updated. More info:
                          https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      env:
                        description: List of environment variables to set in the container.
                          Cannot be updated.
                        type: array
                        items:
                          description: EnvVar represents an environment variable present
                            in a Container.
                          type: object
                          required:
                          - name
                          properties:
                            name:
                              description: Name of the environment variable. Must
                                be a C_IDENTIFIER.
                              type: string
                            value:
                              description: 'Variable references $(VAR_NAME) are expanded
                                using the previous defined environment variables in
                                the container and any service environment variables.
                                If a variable cannot be resolved, the reference in
                                the input string will be unchanged. The $(VAR_NAME)
                                syntax can be escaped with a double $$, ie: $$(VAR_NAME).
                                Escaped references will never be expanded, regardless
                                of whether the variable exists or not. Defaults to
                                "".'
                              type: string
                            valueFrom:
                              description: Source for the environment variable's value.
                                Cannot be used if value is not empty.
                              type: object
                              properties:
                                configMapKeyRef:
                                  description: Selects a key of a ConfigMap.
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key to select.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the ConfigMap or
                                        its key must be defined
                                      type: boolean
                                fieldRef:
                                  description: 'Selects a field of the pod: supports
                                    metadata.name, metadata.namespace, metadata.labels,
                                    metadata.annotations, spec.nodeName, spec.serviceAccountName,
                                    status.hostIP, status.podIP, status.podIPs.'
                                  type: object
                                  required:
                                  - fieldPath
                                  properties:
                                    apiVersion:
                                      description: Version of the schema the FieldPath
                                        is written in terms of, defaults to "v1".
                                      type: string
                                    fieldPath:
                                      description: Path of the field to select in
                                        the specified API version.
                                      type: string
                                resourceFieldRef:
                                  description: 'Selects a resource of the container:
                                    only resources limits and requests (limits.cpu,
                                    limits.memory, limits.ephemeral-storage, requests.cpu,
                                    requests.memory and requests.ephemeral-storage)
                                    are currently supported.'
                                  type: object
                                  required:
                                  - resource
                                  properties:
                                    containerName:
                                      description: 'Container name: required for volumes,
                                        optional for env vars'
                                      type: string
                                    divisor:
                                      description: Specifies the output format of
                                        the exposed resources, defaults to "1"
                                      type: string
                                    resource:
                                      description: 'Required: resource to select'
                                      type: string
                                secretKeyRef:
                                  description: Selects a key of a secret in the pod's
                                    namespace
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key of the secret to select
                                        from.  Must be a valid secret key.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the Secret or its
                                        key must be defined
                                      type: boolean
                      envFrom:
                        description: List of sources to populate environment variables
                          in the container. The keys defined within a source must
                          be a C_IDENTIFIER. All invalid keys will be reported as
                          an event when the container is starting. When a key exists
                          in multiple sources, the value associated with the last
                          source will take precedence. Values defined by an Env with
                          a duplicate key will take precedence. Cannot be updated.
                        type: array
                        items:
                          description: EnvFromSource represents the source of a set
                            of ConfigMaps
                          type: object
                          properties:
                            configMapRef:
                              description: The ConfigMap to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the ConfigMap must
                                    be defined
                                  type: boolean
                            prefix:
                              description: An optional identifier to prepend to each
                                key in the ConfigMap. Must be a C_IDENTIFIER.
                              type: string
                            secretRef:
                              description: The Secret to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the Secret must be
                                    defined
                                  type: boolean
                      image:
                        description: 'Docker image name. More info: https://kubernetes.io/docs/concepts/containers/images'
                        type: string
                      imagePullPolicy:
                        description: 'Image pull policy. One of Always, Never, IfNotPresent.
                          Defaults to Always if :latest tag is specified, or IfNotPresent
                          otherwise. Cannot be updated. More info: https://kubernetes.io/docs/concepts/containers/images#updating-images'
                        type: string
                      lifecycle:
                        description: Lifecycle is not allowed for ephemeral containers.
                        type: object
                        properties:
                          postStart:
                            description: 'PostStart is called immediately after a
                              container is created. If the handler fails, the container
                              is terminated and restarted according to its restart
                              policy. Other management of the container blocks until
                              the hook completes. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                          preStop:
                            description: 'PreStop is called immediately before a container
                              is terminated due to an API request or management event
                              such as liveness/startup probe failure, preemption,
                              resource contention, etc. The handler is not called
                              if the container crashes or exits. The reason for termination
                              is passed to the handler. The Pod''s termination grace
                              period countdown begins before the PreStop hooked is
                              executed. Regardless of the outcome of the handler,
                              the container will eventually terminate within the Pod''s
                              termination grace period. Other management of the container
                              blocks until the hook completes or until the termination
                              grace period is reached. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                      livenessProbe:
                        description: Probes are not allowed for ephemeral containers.
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      name:
                        description: Name of the ephemeral container specified as
                          a DNS_LABEL. This name must be unique among all containers,
                          init containers and ephemeral containers.
                        type: string
                      ports:
                        description: Ports are not allowed for ephemeral containers.
                        type: array
                        items:
                          description: ContainerPort represents a network port in
                            a single container.
                          type: object
                          required:
                          - containerPort
                          properties:
                            containerPort:
                              description: Number of port to expose on the pod's IP
                                address. This must be a valid port number, 0 < x <
                                65536.
                              type: integer
                              format: int32
                            hostIP:
                              description: What host IP to bind the external port
                                to.
                              type: string
                            hostPort:
                              description: Number of port to expose on the host. If
                                specified, this must be a valid port number, 0 < x
                                < 65536. If HostNetwork is specified, this must match
                                ContainerPort. Most containers do not need this.
                              type: integer
                              format: int32
                            name:
                              description: If specified, this must be an IANA_SVC_NAME
                                and unique within the pod. Each named port in a pod
                                must have a unique name. Name for the port that can
                                be referred to by services.
                              type: string
                            protocol:
                              description: Protocol for port. Must be UDP, TCP, or
                                SCTP. Defaults to "TCP".
                              type: string
                      readinessProbe:
                        description: Probes are not allowed for ephemeral containers.
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      resources:
                        description: Resources are not allowed for ephemeral containers.
                          Ephemeral containers use spare resources already allocated
                          to the pod.
                        type: object
                        properties:
                          limits:
                            description: 'Limits describes the maximum amount of compute
                              resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                          requests:
                            description: 'Requests describes the minimum amount of
                              compute resources required. If Requests is omitted for
                              a container, it defaults to Limits if that is explicitly
                              specified, otherwise to an implementation-defined value.
                              More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                      securityContext:
                        description: SecurityContext is not allowed for ephemeral
                          containers.
                        type: object
                        properties:
                          allowPrivilegeEscalation:
                            description: 'AllowPrivilegeEscalation controls whether
                              a process can gain more privileges than its parent process.
                              This bool directly controls if the no_new_privs flag
                              will be set on the container process. AllowPrivilegeEscalation
                              is true always when the container is: 1) run as Privileged
                              2) has CAP_SYS_ADMIN'
                            type: boolean
                          capabilities:
                            description: The capabilities to add/drop when running
                              containers. Defaults to the default set of capabilities
                              granted by the container runtime.
                            type: object
                            properties:
                              add:
                                description: Added capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                              drop:
                                description: Removed capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                          privileged:
                            description: Run container in privileged mode. Processes
                              in privileged containers are essentially equivalent
                              to root on the host. Defaults to false.
                            type: boolean
                          procMount:
                            description: procMount denotes the type of proc mount
                              to use for the containers. The default is DefaultProcMount
                              which uses the container runtime defaults for readonly
                              paths and masked paths. This requires the ProcMountType
                              feature flag to be enabled.
                            type: string
                          readOnlyRootFilesystem:
                            description: Whether this container has a read-only root
                              filesystem. Default is false.
                            type: boolean
                          runAsGroup:
                            description: The GID to run the entrypoint of the container
                              process. Uses runtime default if unset. May also be
                              set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: integer
                            format: int64
                          runAsNonRoot:
                            description: Indicates that the container must run as
                              a non-root user. If true, the Kubelet will validate
                              the image at runtime to ensure that it does not run
                              as UID 0 (root) and fail to start the container if it
                              does. If unset or false, no such validation will be
                              performed. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: boolean
                          runAsUser:
                            description: The UID to run the entrypoint of the container
                              process. Defaults to user specified in image metadata
                              if unspecified. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: integer
                            format: int64
                          seLinuxOptions:
                            description: The SELinux context to be applied to the
                              container. If unspecified, the container runtime will
                              allocate a random SELinux context for each container.  May
                              also be set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              level:
                                description: Level is SELinux level label that applies
                                  to the container.
                                type: string
                              role:
                                description: Role is a SELinux role label that applies
                                  to the container.
                                type: string
                              type:
                                description: Type is a SELinux type label that applies
                                  to the container.
                                type: string
                              user:
                                description: User is a SELinux user label that applies
                                  to the container.
                                type: string
                          windowsOptions:
                            description: The Windows specific settings applied to
                              all containers. If unspecified, the options from the
                              PodSecurityContext will be used. If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              gmsaCredentialSpec:
                                description: GMSACredentialSpec is where the GMSA
                                  admission webhook (https://github.com/kubernetes-sigs/windows-gmsa)
                                  inlines the contents of the GMSA credential spec
                                  named by the GMSACredentialSpecName field.
                                type: string
                              gmsaCredentialSpecName:
                                description: GMSACredentialSpecName is the name of
                                  the GMSA credential spec to use.
                                type: string
                              runAsUserName:
                                description: The UserName in Windows to run the entrypoint
                                  of the container process. Defaults to the user specified
                                  in image metadata if unspecified. May also be set
                                  in PodSecurityContext. If set in both SecurityContext
                                  and PodSecurityContext, the value specified in SecurityContext
                                  takes precedence.
                                type: string
                      startupProbe:
                        description: Probes are not allowed for ephemeral containers.
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      stdin:
                        description: Whether this container should allocate a buffer
                          for stdin in the container runtime. If this is not set,
                          reads from stdin in the container will always result in
                          EOF. Default is false.
                        type: boolean
                      stdinOnce:
                        description: Whether the container runtime should close the
                          stdin channel after it has been opened by a single attach.
                          When stdin is true the stdin stream will remain open across
                          multiple attach sessions. If stdinOnce is set to true, stdin
                          is opened on container start, is empty until the first client
                          attaches to stdin, and then remains open and accepts data
                          until the client disconnects, at which time stdin is closed
                          and remains closed until the container is restarted. If
                          this flag is false, a container processes that reads from
                          stdin will never receive an EOF. Default is false
                        type: boolean
                      targetContainerName:
                        description: If set, the name of the container from PodSpec
                          that this ephemeral container targets. The ephemeral container
                          will be run in the namespaces (IPC, PID, etc) of this container.
                          If not set then the ephemeral container is run in whatever
                          namespaces are shared for the pod. Note that the container
                          runtime must support this feature.
                        type: string
                      terminationMessagePath:
                        description: 'Optional: Path at which the file to which the
                          container''s termination message will be written is mounted
                          into the container''s filesystem. Message written is intended
                          to be brief final status, such as an assertion failure message.
                          Will be truncated by the node if greater than 4096 bytes.
                          The total message length across all containers will be limited
                          to 12kb. Defaults to /dev/termination-log. Cannot be updated.'
                        type: string
                      terminationMessagePolicy:
                        description: Indicate how the termination message should be
                          populated. File will use the contents of terminationMessagePath
                          to populate the container status message on both success
                          and failure. FallbackToLogsOnError will use the last chunk
                          of container log output if the termination message file
                          is empty and the container exited with an error. The log
                          output is limited to 2048 bytes or 80 lines, whichever is
                          smaller. Defaults to File. Cannot be updated.
                        type: string
                      tty:
                        description: Whether this container should allocate a TTY
                          for itself, also requires 'stdin' to be true. Default is
                          false.
                        type: boolean
                      volumeDevices:
                        description: volumeDevices is the list of block devices to
                          be used by the container.
                        type: array
                        items:
                          description: volumeDevice describes a mapping of a raw block
                            device within a container.
                          type: object
                          required:
                          - devicePath
                          - name
                          properties:
                            devicePath:
                              description: devicePath is the path inside of the container
                                that the device will be mapped to.
                              type: string
                            name:
                              description: name must match the name of a persistentVolumeClaim
                                in the pod
                              type: string
                      volumeMounts:
                        description: Pod volumes to mount into the container's filesystem.
                          Cannot be updated.
                        type: array
                        items:
                          description: VolumeMount describes a mounting of a Volume
                            within a container.
                          type: object
                          required:
                          - mountPath
                          - name
                          properties:
                            mountPath:
                              description: Path within the container at which the
                                volume should be mounted.  Must not contain ':'.
                              type: string
                            mountPropagation:
                              description: mountPropagation determines how mounts
                                are propagated from the host to container and the
                                other way around. When not set, MountPropagationNone
                                is used. This field is beta in 1.10.
                              type: string
                            name:
                              description: This must match the Name of a Volume.
                              type: string
                            readOnly:
                              description: Mounted read-only if true, read-write otherwise
                                (false or unspecified). Defaults to false.
                              type: boolean
                            subPath:
                              description: Path within the volume from which the container's
                                volume should be mounted. Defaults to "" (volume's
                                root).
                              type: string
                            subPathExpr:
                              description: Expanded path within the volume from which
                                the container's volume should be mounted. Behaves
                                similarly to SubPath but environment variable references
                                $(VAR_NAME) are expanded using the container's environment.
                                Defaults to "" (volume's root). SubPathExpr and SubPath
                                are mutually exclusive.
                              type: string
                      workingDir:
                        description: Container's working directory. If not specified,
                          the container runtime's default will be used, which might
                          be configured in the container image. Cannot be updated.
                        type: string
                hostAliases:
                  description: HostAliases is an optional list of hosts and IPs that
                    will be injected into the pod's hosts file if specified. This
                    is only valid for non-hostNetwork pods.
                  type: array
                  items:
                    description: HostAlias holds the mapping between IP and hostnames
                      that will be injected as an entry in the pod's hosts file.
                    type: object
                    properties:
                      hostnames:
                        description: Hostnames for the above IP address.
                        type: array
                        items:
                          type: string
                      ip:
                        description: IP address of the host file entry.
                        type: string
                hostIPC:
                  description: 'Use the host''s ipc namespace. Optional: Default to
                    false.'
                  type: boolean
                hostNetwork:
                  description: Host networking requested for this pod. Use the host's
                    network namespace. If this option is set, the ports that will
                    be used must be specified. Default to false.
                  type: boolean
                hostPID:
                  description: 'Use the host''s pid namespace. Optional: Default to
                    false.'
                  type: boolean
                hostname:
                  description: Specifies the hostname of the Pod If not specified,
                    the pod's hostname will be set to a system-defined value.
                  type: string
                imagePullSecrets:
                  description: 'ImagePullSecrets is an optional list of references
                    to secrets in the same namespace to use for pulling any of the
                    images used by this PodSpec. If specified, these secrets will
                    be passed to individual puller implementations for them to use.
                    For example, in the case of docker, only DockerConfig type secrets
                    are honored. More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod'
                  type: array
                  items:
                    description: LocalObjectReference contains enough information
                      to let you locate the referenced object inside the same namespace.
                    type: object
                    properties:
                      name:
                        description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                          TODO: Add other useful fields. apiVersion, kind, uid?'
                        type: string
                initContainers:
                  description: 'List of initialization containers belonging to the
                    pod. Init containers are executed in order prior to containers
                    being started. If any init container fails, the pod is considered
                    to have failed and is handled according to its restartPolicy.
                    The name for an init container or normal container must be unique
                    among all containers. Init containers may not have Lifecycle actions,
                    Readiness probes, Liveness probes, or Startup probes. The resourceRequirements
                    of an init container are taken into account during scheduling
                    by finding the highest request/limit for each resource type, and
                    then using the max of of that value or the sum of the normal containers.
                    Limits are applied to init containers in a similar fashion. Init
                    containers cannot currently be added or removed. Cannot be updated.
                    More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/'
                  type: array
                  items:
                    description: A single application container that you want to run
                      within a pod.
                    type: object
                    required:
                    - name
                    properties:
                      args:
                        description: 'Arguments to the entrypoint. The docker image''s
                          CMD is used if this is not provided. Variable references
                          $(VAR_NAME) are expanded using the container''s environment.
                          If a variable cannot be resolved, the reference in the input
                          string will be unchanged. The $(VAR_NAME) syntax can be
                          escaped with a double $$, ie: $$(VAR_NAME). Escaped references
                          will never be expanded, regardless of whether the variable
                          exists or not. Cannot be updated. More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      command:
                        description: 'Entrypoint array. Not executed within a shell.
                          The docker image''s ENTRYPOINT is used if this is not provided.
                          Variable references $(VAR_NAME) are expanded using the container''s
                          environment. If a variable cannot be resolved, the reference
                          in the input string will be unchanged. The $(VAR_NAME) syntax
                          can be escaped with a double $$, ie: $$(VAR_NAME). Escaped
                          references will never be expanded, regardless of whether
                          the variable exists or not. Cannot be updated. More info:
                          https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell'
                        type: array
                        items:
                          type: string
                      env:
                        description: List of environment variables to set in the container.
                          Cannot be updated.
                        type: array
                        items:
                          description: EnvVar represents an environment variable present
                            in a Container.
                          type: object
                          required:
                          - name
                          properties:
                            name:
                              description: Name of the environment variable. Must
                                be a C_IDENTIFIER.
                              type: string
                            value:
                              description: 'Variable references $(VAR_NAME) are expanded
                                using the previous defined environment variables in
                                the container and any service environment variables.
                                If a variable cannot be resolved, the reference in
                                the input string will be unchanged. The $(VAR_NAME)
                                syntax can be escaped with a double $$, ie: $$(VAR_NAME).
                                Escaped references will never be expanded, regardless
                                of whether the variable exists or not. Defaults to
                                "".'
                              type: string
                            valueFrom:
                              description: Source for the environment variable's value.
                                Cannot be used if value is not empty.
                              type: object
                              properties:
                                configMapKeyRef:
                                  description: Selects a key of a ConfigMap.
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key to select.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the ConfigMap or
                                        its key must be defined
                                      type: boolean
                                fieldRef:
                                  description: 'Selects a field of the pod: supports
                                    metadata.name, metadata.namespace, metadata.labels,
                                    metadata.annotations, spec.nodeName, spec.serviceAccountName,
                                    status.hostIP, status.podIP, status.podIPs.'
                                  type: object
                                  required:
                                  - fieldPath
                                  properties:
                                    apiVersion:
                                      description: Version of the schema the FieldPath
                                        is written in terms of, defaults to "v1".
                                      type: string
                                    fieldPath:
                                      description: Path of the field to select in
                                        the specified API version.
                                      type: string
                                resourceFieldRef:
                                  description: 'Selects a resource of the container:
                                    only resources limits and requests (limits.cpu,
                                    limits.memory, limits.ephemeral-storage, requests.cpu,
                                    requests.memory and requests.ephemeral-storage)
                                    are currently supported.'
                                  type: object
                                  required:
                                  - resource
                                  properties:
                                    containerName:
                                      description: 'Container name: required for volumes,
                                        optional for env vars'
                                      type: string
                                    divisor:
                                      description: Specifies the output format of
                                        the exposed resources, defaults to "1"
                                      type: string
                                    resource:
                                      description: 'Required: resource to select'
                                      type: string
                                secretKeyRef:
                                  description: Selects a key of a secret in the pod's
                                    namespace
                                  type: object
                                  required:
                                  - key
                                  properties:
                                    key:
                                      description: The key of the secret to select
                                        from.  Must be a valid secret key.
                                      type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the Secret or its
                                        key must be defined
                                      type: boolean
                      envFrom:
                        description: List of sources to populate environment variables
                          in the container. The keys defined within a source must
                          be a C_IDENTIFIER. All invalid keys will be reported as
                          an event when the container is starting. When a key exists
                          in multiple sources, the value associated with the last
                          source will take precedence. Values defined by an Env with
                          a duplicate key will take precedence. Cannot be updated.
                        type: array
                        items:
                          description: EnvFromSource represents the source of a set
                            of ConfigMaps
                          type: object
                          properties:
                            configMapRef:
                              description: The ConfigMap to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the ConfigMap must
                                    be defined
                                  type: boolean
                            prefix:
                              description: An optional identifier to prepend to each
                                key in the ConfigMap. Must be a C_IDENTIFIER.
                              type: string
                            secretRef:
                              description: The Secret to select from
                              type: object
                              properties:
                                name:
                                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                    TODO: Add other useful fields. apiVersion, kind,
                                    uid?'
                                  type: string
                                optional:
                                  description: Specify whether the Secret must be
                                    defined
                                  type: boolean
                      image:
                        description: 'Docker image name. More info: https://kubernetes.io/docs/concepts/containers/images
                          This field is optional to allow higher level config management
                          to default or override container images in workload controllers
                          like Deployments and StatefulSets.'
                        type: string
                      imagePullPolicy:
                        description: 'Image pull policy. One of Always, Never, IfNotPresent.
                          Defaults to Always if :latest tag is specified, or IfNotPresent
                          otherwise. Cannot be updated. More info: https://kubernetes.io/docs/concepts/containers/images#updating-images'
                        type: string
                      lifecycle:
                        description: Actions that the management system should take
                          in response to container lifecycle events. Cannot be updated.
                        type: object
                        properties:
                          postStart:
                            description: 'PostStart is called immediately after a
                              container is created. If the handler fails, the container
                              is terminated and restarted according to its restart
                              policy. Other management of the container blocks until
                              the hook completes. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                          preStop:
                            description: 'PreStop is called immediately before a container
                              is terminated due to an API request or management event
                              such as liveness/startup probe failure, preemption,
                              resource contention, etc. The handler is not called
                              if the container crashes or exits. The reason for termination
                              is passed to the handler. The Pod''s termination grace
                              period countdown begins before the PreStop hooked is
                              executed. Regardless of the outcome of the handler,
                              the container will eventually terminate within the Pod''s
                              termination grace period. Other management of the container
                              blocks until the hook completes or until the termination
                              grace period is reached. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks'
                            type: object
                            properties:
                              exec:
                                description: One and only one of the following should
                                  be specified. Exec specifies the action to take.
                                type: object
                                properties:
                                  command:
                                    description: Command is the command line to execute
                                      inside the container, the working directory
                                      for the command  is root ('/') in the container's
                                      filesystem. The command is simply exec'd, it
                                      is not run inside a shell, so traditional shell
                                      instructions ('|', etc) won't work. To use a
                                      shell, you need to explicitly call out to that
                                      shell. Exit status of 0 is treated as live/healthy
                                      and non-zero is unhealthy.
                                    type: array
                                    items:
                                      type: string
                              httpGet:
                                description: HTTPGet specifies the http request to
                                  perform.
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: Host name to connect to, defaults
                                      to the pod IP. You probably want to set "Host"
                                      in httpHeaders instead.
                                    type: string
                                  httpHeaders:
                                    description: Custom headers to set in the request.
                                      HTTP allows repeated headers.
                                    type: array
                                    items:
                                      description: HTTPHeader describes a custom header
                                        to be used in HTTP probes
                                      type: object
                                      required:
                                      - name
                                      - value
                                      properties:
                                        name:
                                          description: The header field name
                                          type: string
                                        value:
                                          description: The header field value
                                          type: string
                                  path:
                                    description: Path to access on the HTTP server.
                                    type: string
                                  port:
                                    description: Name or number of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                                  scheme:
                                    description: Scheme to use for connecting to the
                                      host. Defaults to HTTP.
                                    type: string
                              tcpSocket:
                                description: 'TCPSocket specifies an action involving
                                  a TCP port. TCP hooks not yet supported TODO: implement
                                  a realistic TCP lifecycle hook'
                                type: object
                                required:
                                - port
                                properties:
                                  host:
                                    description: 'Optional: Host name to connect to,
                                      defaults to the pod IP.'
                                    type: string
                                  port:
                                    description: Number or name of the port to access
                                      on the container. Number must be in the range
                                      1 to 65535. Name must be an IANA_SVC_NAME.
                                    anyOf:
                                    - type: string
                                    - type: integer
                      livenessProbe:
                        description: 'Periodic probe of container liveness. Container
                          will be restarted if the probe fails. Cannot be updated.
                          More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      name:
                        description: Name of the container specified as a DNS_LABEL.
                          Each container in a pod must have a unique name (DNS_LABEL).
                          Cannot be updated.
                        type: string
                      ports:
                        description: List of ports to expose from the container. Exposing
                          a port here gives the system additional information about
                          the network connections a container uses, but is primarily
                          informational. Not specifying a port here DOES NOT prevent
                          that port from being exposed. Any port which is listening
                          on the default "0.0.0.0" address inside a container will
                          be accessible from the network. Cannot be updated.
                        type: array
                        items:
                          description: ContainerPort represents a network port in
                            a single container.
                          type: object
                          required:
                          - containerPort
                          properties:
                            containerPort:
                              description: Number of port to expose on the pod's IP
                                address. This must be a valid port number, 0 < x <
                                65536.
                              type: integer
                              format: int32
                            hostIP:
                              description: What host IP to bind the external port
                                to.
                              type: string
                            hostPort:
                              description: Number of port to expose on the host. If
                                specified, this must be a valid port number, 0 < x
                                < 65536. If HostNetwork is specified, this must match
                                ContainerPort. Most containers do not need this.
                              type: integer
                              format: int32
                            name:
                              description: If specified, this must be an IANA_SVC_NAME
                                and unique within the pod. Each named port in a pod
                                must have a unique name. Name for the port that can
                                be referred to by services.
                              type: string
                            protocol:
                              description: Protocol for port. Must be UDP, TCP, or
                                SCTP. Defaults to "TCP".
                              type: string
                      readinessProbe:
                        description: 'Periodic probe of container service readiness.
                          Container will be removed from service endpoints if the
                          probe fails. Cannot be updated. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      resources:
                        description: 'Compute Resources required by this container.
                          Cannot be updated. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                        type: object
                        properties:
                          limits:
                            description: 'Limits describes the maximum amount of compute
                              resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                          requests:
                            description: 'Requests describes the minimum amount of
                              compute resources required. If Requests is omitted for
                              a container, it defaults to Limits if that is explicitly
                              specified, otherwise to an implementation-defined value.
                              More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/'
                            type: object
                            additionalProperties:
                              type: string
                      securityContext:
                        description: 'Security options the pod should run with. More
                          info: https://kubernetes.io/docs/concepts/policy/security-context/
                          More info: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/'
                        type: object
                        properties:
                          allowPrivilegeEscalation:
                            description: 'AllowPrivilegeEscalation controls whether
                              a process can gain more privileges than its parent process.
                              This bool directly controls if the no_new_privs flag
                              will be set on the container process. AllowPrivilegeEscalation
                              is true always when the container is: 1) run as Privileged
                              2) has CAP_SYS_ADMIN'
                            type: boolean
                          capabilities:
                            description: The capabilities to add/drop when running
                              containers. Defaults to the default set of capabilities
                              granted by the container runtime.
                            type: object
                            properties:
                              add:
                                description: Added capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                              drop:
                                description: Removed capabilities
                                type: array
                                items:
                                  description: Capability represent POSIX capabilities
                                    type
                                  type: string
                          privileged:
                            description: Run container in privileged mode. Processes
                              in privileged containers are essentially equivalent
                              to root on the host. Defaults to false.
                            type: boolean
                          procMount:
                            description: procMount denotes the type of proc mount
                              to use for the containers. The default is DefaultProcMount
                              which uses the container runtime defaults for readonly
                              paths and masked paths. This requires the ProcMountType
                              feature flag to be enabled.
                            type: string
                          readOnlyRootFilesystem:
                            description: Whether this container has a read-only root
                              filesystem. Default is false.
                            type: boolean
                          runAsGroup:
                            description: The GID to run the entrypoint of the container
                              process. Uses runtime default if unset. May also be
                              set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: integer
                            format: int64
                          runAsNonRoot:
                            description: Indicates that the container must run as
                              a non-root user. If true, the Kubelet will validate
                              the image at runtime to ensure that it does not run
                              as UID 0 (root) and fail to start the container if it
                              does. If unset or false, no such validation will be
                              performed. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: boolean
                          runAsUser:
                            description: The UID to run the entrypoint of the container
                              process. Defaults to user specified in image metadata
                              if unspecified. May also be set in PodSecurityContext.  If
                              set in both SecurityContext and PodSecurityContext,
                              the value specified in SecurityContext takes precedence.
                            type: integer
                            format: int64
                          seLinuxOptions:
                            description: The SELinux context to be applied to the
                              container. If unspecified, the container runtime will
                              allocate a random SELinux context for each container.  May
                              also be set in PodSecurityContext.  If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              level:
                                description: Level is SELinux level label that applies
                                  to the container.
                                type: string
                              role:
                                description: Role is a SELinux role label that applies
                                  to the container.
                                type: string
                              type:
                                description: Type is a SELinux type label that applies
                                  to the container.
                                type: string
                              user:
                                description: User is a SELinux user label that applies
                                  to the container.
                                type: string
                          windowsOptions:
                            description: The Windows specific settings applied to
                              all containers. If unspecified, the options from the
                              PodSecurityContext will be used. If set in both SecurityContext
                              and PodSecurityContext, the value specified in SecurityContext
                              takes precedence.
                            type: object
                            properties:
                              gmsaCredentialSpec:
                                description: GMSACredentialSpec is where the GMSA
                                  admission webhook (https://github.com/kubernetes-sigs/windows-gmsa)
                                  inlines the contents of the GMSA credential spec
                                  named by the GMSACredentialSpecName field.
                                type: string
                              gmsaCredentialSpecName:
                                description: GMSACredentialSpecName is the name of
                                  the GMSA credential spec to use.
                                type: string
                              runAsUserName:
                                description: The UserName in Windows to run the entrypoint
                                  of the container process. Defaults to the user specified
                                  in image metadata if unspecified. May also be set
                                  in PodSecurityContext. If set in both SecurityContext
                                  and PodSecurityContext, the value specified in SecurityContext
                                  takes precedence.
                                type: string
                      startupProbe:
                        description: 'StartupProbe indicates that the Pod has successfully
                          initialized. If specified, no other probes are executed
                          until this completes successfully. If this probe fails,
                          the Pod will be restarted, just as if the livenessProbe
                          failed. This can be used to provide different probe parameters
                          at the beginning of a Pod''s lifecycle, when it might take
                          a long time to load data or warm a cache, than during steady-state
                          operation. This cannot be updated. This is a beta feature
                          enabled by the StartupProbe feature flag. More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                        type: object
                        properties:
                          exec:
                            description: One and only one of the following should
                              be specified. Exec specifies the action to take.
                            type: object
                            properties:
                              command:
                                description: Command is the command line to execute
                                  inside the container, the working directory for
                                  the command  is root ('/') in the container's filesystem.
                                  The command is simply exec'd, it is not run inside
                                  a shell, so traditional shell instructions ('|',
                                  etc) won't work. To use a shell, you need to explicitly
                                  call out to that shell. Exit status of 0 is treated
                                  as live/healthy and non-zero is unhealthy.
                                type: array
                                items:
                                  type: string
                          failureThreshold:
                            description: Minimum consecutive failures for the probe
                              to be considered failed after having succeeded. Defaults
                              to 3. Minimum value is 1.
                            type: integer
                            format: int32
                          httpGet:
                            description: HTTPGet specifies the http request to perform.
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: Host name to connect to, defaults to
                                  the pod IP. You probably want to set "Host" in httpHeaders
                                  instead.
                                type: string
                              httpHeaders:
                                description: Custom headers to set in the request.
                                  HTTP allows repeated headers.
                                type: array
                                items:
                                  description: HTTPHeader describes a custom header
                                    to be used in HTTP probes
                                  type: object
                                  required:
                                  - name
                                  - value
                                  properties:
                                    name:
                                      description: The header field name
                                      type: string
                                    value:
                                      description: The header field value
                                      type: string
                              path:
                                description: Path to access on the HTTP server.
                                type: string
                              port:
                                description: Name or number of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                              scheme:
                                description: Scheme to use for connecting to the host.
                                  Defaults to HTTP.
                                type: string
                          initialDelaySeconds:
                            description: 'Number of seconds after the container has
                              started before liveness probes are initiated. More info:
                              https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                          periodSeconds:
                            description: How often (in seconds) to perform the probe.
                              Default to 10 seconds. Minimum value is 1.
                            type: integer
                            format: int32
                          successThreshold:
                            description: Minimum consecutive successes for the probe
                              to be considered successful after having failed. Defaults
                              to 1. Must be 1 for liveness and startup. Minimum value
                              is 1.
                            type: integer
                            format: int32
                          tcpSocket:
                            description: 'TCPSocket specifies an action involving
                              a TCP port. TCP hooks not yet supported TODO: implement
                              a realistic TCP lifecycle hook'
                            type: object
                            required:
                            - port
                            properties:
                              host:
                                description: 'Optional: Host name to connect to, defaults
                                  to the pod IP.'
                                type: string
                              port:
                                description: Number or name of the port to access
                                  on the container. Number must be in the range 1
                                  to 65535. Name must be an IANA_SVC_NAME.
                                anyOf:
                                - type: string
                                - type: integer
                          timeoutSeconds:
                            description: 'Number of seconds after which the probe
                              times out. Defaults to 1 second. Minimum value is 1.
                              More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes'
                            type: integer
                            format: int32
                      stdin:
                        description: Whether this container should allocate a buffer
                          for stdin in the container runtime. If this is not set,
                          reads from stdin in the container will always result in
                          EOF. Default is false.
                        type: boolean
                      stdinOnce:
                        description: Whether the container runtime should close the
                          stdin channel after it has been opened by a single attach.
                          When stdin is true the stdin stream will remain open across
                          multiple attach sessions. If stdinOnce is set to true, stdin
                          is opened on container start, is empty until the first client
                          attaches to stdin, and then remains open and accepts data
                          until the client disconnects, at which time stdin is closed
                          and remains closed until the container is restarted. If
                          this flag is false, a container processes that reads from
                          stdin will never receive an EOF. Default is false
                        type: boolean
                      terminationMessagePath:
                        description: 'Optional: Path at which the file to which the
                          container''s termination message will be written is mounted
                          into the container''s filesystem. Message written is intended
                          to be brief final status, such as an assertion failure message.
                          Will be truncated by the node if greater than 4096 bytes.
                          The total message length across all containers will be limited
                          to 12kb. Defaults to /dev/termination-log. Cannot be updated.'
                        type: string
                      terminationMessagePolicy:
                        description: Indicate how the termination message should be
                          populated. File will use the contents of terminationMessagePath
                          to populate the container status message on both success
                          and failure. FallbackToLogsOnError will use the last chunk
                          of container log output if the termination message file
                          is empty and the container exited with an error. The log
                          output is limited to 2048 bytes or 80 lines, whichever is
                          smaller. Defaults to File. Cannot be updated.
                        type: string
                      tty:
                        description: Whether this container should allocate a TTY
                          for itself, also requires 'stdin' to be true. Default is
                          false.
                        type: boolean
                      volumeDevices:
                        description: volumeDevices is the list of block devices to
                          be used by the container.
                        type: array
                        items:
                          description: volumeDevice describes a mapping of a raw block
                            device within a container.
                          type: object
                          required:
                          - devicePath
                          - name
                          properties:
                            devicePath:
                              description: devicePath is the path inside of the container
                                that the device will be mapped to.
                              type: string
                            name:
                              description: name must match the name of a persistentVolumeClaim
                                in the pod
                              type: string
                      volumeMounts:
                        description: Pod volumes to mount into the container's filesystem.
                          Cannot be updated.
                        type: array
                        items:
                          description: VolumeMount describes a mounting of a Volume
                            within a container.
                          type: object
                          required:
                          - mountPath
                          - name
                          properties:
                            mountPath:
                              description: Path within the container at which the
                                volume should be mounted.  Must not contain ':'.
                              type: string
                            mountPropagation:
                              description: mountPropagation determines how mounts
                                are propagated from the host to container and the
                                other way around. When not set, MountPropagationNone
                                is used. This field is beta in 1.10.
                              type: string
                            name:
                              description: This must match the Name of a Volume.
                              type: string
                            readOnly:
                              description: Mounted read-only if true, read-write otherwise
                                (false or unspecified). Defaults to false.
                              type: boolean
                            subPath:
                              description: Path within the volume from which the container's
                                volume should be mounted. Defaults to "" (volume's
                                root).
                              type: string
                            subPathExpr:
                              description: Expanded path within the volume from which
                                the container's volume should be mounted. Behaves
                                similarly to SubPath but environment variable references
                                $(VAR_NAME) are expanded using the container's environment.
                                Defaults to "" (volume's root). SubPathExpr and SubPath
                                are mutually exclusive.
                              type: string
                      workingDir:
                        description: Container's working directory. If not specified,
                          the container runtime's default will be used, which might
                          be configured in the container image. Cannot be updated.
                        type: string
                nodeName:
                  description: NodeName is a request to schedule this pod onto a specific
                    node. If it is non-empty, the scheduler simply schedules this
                    pod onto that node, assuming that it fits resource requirements.
                  type: string
                nodeSelector:
                  description: 'NodeSelector is a selector which must be true for
                    the pod to fit on a node. Selector which must match a node''s
                    labels for the pod to be scheduled on that node. More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/'
                  type: object
                  additionalProperties:
                    type: string
                overhead:
                  description: 'Overhead represents the resource overhead associated
                    with running a pod for a given RuntimeClass. This field will be
                    autopopulated at admission time by the RuntimeClass admission
                    controller. If the RuntimeClass admission controller is enabled,
                    overhead must not be set in Pod create requests. The RuntimeClass
                    admission controller will reject Pod create requests which have
                    the overhead already set. If RuntimeClass is configured and selected
                    in the PodSpec, Overhead will be set to the value defined in the
                    corresponding RuntimeClass, otherwise it will remain unset and
                    treated as zero. More info: https://git.k8s.io/enhancements/keps/sig-node/20190226-pod-overhead.md
                    This field is alpha-level as of Kubernetes v1.16, and is only
                    honored by servers that enable the PodOverhead feature.'
                  type: object
                  additionalProperties:
                    type: string
                preemptionPolicy:
                  description: PreemptionPolicy is the Policy for preempting pods
                    with lower priority. One of Never, PreemptLowerPriority. Defaults
                    to PreemptLowerPriority if unset. This field is alpha-level and
                    is only honored by servers that enable the NonPreemptingPriority
                    feature.
                  type: string
                priority:
                  description: The priority value. Various system components use this
                    field to find the priority of the pod. When Priority Admission
                    Controller is enabled, it prevents users from setting this field.
                    The admission controller populates this field from PriorityClassName.
                    The higher the value, the higher the priority.
                  type: integer
                  format: int32
                priorityClassName:
                  description: If specified, indicates the pod's priority. "system-node-critical"
                    and "system-cluster-critical" are two special keywords which indicate
                    the highest priorities with the former being the highest priority.
                    Any other name must be defined by creating a PriorityClass object
                    with that name. If not specified, the pod priority will be default
                    or zero if there is no default.
                  type: string
                readinessGates:
                  description: 'If specified, all readiness gates will be evaluated
                    for pod readiness. A pod is ready when all its containers are
                    ready AND all conditions specified in the readiness gates have
                    status equal to "True" More info: https://git.k8s.io/enhancements/keps/sig-network/0007-pod-ready%2B%2B.md'
                  type: array
                  items:
                    description: PodReadinessGate contains the reference to a pod
                      condition
                    type: object
                    required:
                    - conditionType
                    properties:
                      conditionType:
                        description: ConditionType refers to a condition in the pod's
                          condition list with matching type.
                        type: string
                restartPolicy:
                  description: 'Restart policy for all containers within the pod.
                    One of Always, OnFailure, Never. Default to Always. More info:
                    https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy'
                  type: string
                runtimeClassName:
                  description: 'RuntimeClassName refers to a RuntimeClass object in
                    the node.k8s.io group, which should be used to run this pod.  If
                    no RuntimeClass resource matches the named class, the pod will
                    not be run. If unset or empty, the "legacy" RuntimeClass will
                    be used, which is an implicit class with an empty definition that
                    uses the default runtime handler. More info: https://git.k8s.io/enhancements/keps/sig-node/runtime-class.md
                    This is a beta feature as of Kubernetes v1.14.'
                  type: string
                schedulerName:
                  description: If specified, the pod will be dispatched by specified
                    scheduler. If not specified, the pod will be dispatched by default
                    scheduler.
                  type: string
                securityContext:
                  description: 'SecurityContext holds pod-level security attributes
                    and common container settings. Optional: Defaults to empty.  See
                    type description for default values of each field.'
                  type: object
                  properties:
                    fsGroup:
                      description: "A special supplemental group that applies to all
                        containers in a pod. Some volume types allow the Kubelet to
                        change the ownership of that volume to be owned by the pod:
                        \n 1. The owning GID will be the FSGroup 2. The setgid bit
                        is set (new files created in the volume will be owned by FSGroup)
                        3. The permission bits are OR'd with rw-rw---- \n If unset,
                        the Kubelet will not modify the ownership and permissions
                        of any volume."
                      type: integer
                      format: int64
                    fsGroupChangePolicy:
                      description: 'fsGroupChangePolicy defines behavior of changing
                        ownership and permission of the volume before being exposed
                        inside Pod. This field will only apply to volume types which
                        support fsGroup based ownership(and permissions). It will
                        have no effect on ephemeral volume types such as: secret,
                        configmaps and emptydir. Valid values are "OnRootMismatch"
                        and "Always". If not specified defaults to "Always".'
                      type: string
                    runAsGroup:
                      description: The GID to run the entrypoint of the container
                        process. Uses runtime default if unset. May also be set in
                        SecurityContext.  If set in both SecurityContext and PodSecurityContext,
                        the value specified in SecurityContext takes precedence for
                        that container.
                      type: integer
                      format: int64
                    runAsNonRoot:
                      description: Indicates that the container must run as a non-root
                        user. If true, the Kubelet will validate the image at runtime
                        to ensure that it does not run as UID 0 (root) and fail to
                        start the container if it does. If unset or false, no such
                        validation will be performed. May also be set in SecurityContext.  If
                        set in both SecurityContext and PodSecurityContext, the value
                        specified in SecurityContext takes precedence.
                      type: boolean
                    runAsUser:
                      description: The UID to run the entrypoint of the container
                        process. Defaults to user specified in image metadata if unspecified.
                        May also be set in SecurityContext.  If set in both SecurityContext
                        and PodSecurityContext, the value specified in SecurityContext
                        takes precedence for that container.
                      type: integer
                      format: int64
                    seLinuxOptions:
                      description: The SELinux context to be applied to all containers.
                        If unspecified, the container runtime will allocate a random
                        SELinux context for each container.  May also be set in SecurityContext.  If
                        set in both SecurityContext and PodSecurityContext, the value
                        specified in SecurityContext takes precedence for that container.
                      type: object
                      properties:
                        level:
                          description: Level is SELinux level label that applies to
                            the container.
                          type: string
                        role:
                          description: Role is a SELinux role label that applies to
                            the container.
                          type: string
                        type:
                          description: Type is a SELinux type label that applies to
                            the container.
                          type: string
                        user:
                          description: User is a SELinux user label that applies to
                            the container.
                          type: string
                    supplementalGroups:
                      description: A list of groups applied to the first process run
                        in each container, in addition to the container's primary
                        GID.  If unspecified, no groups will be added to any container.
                      type: array
                      items:
                        type: integer
                        format: int64
                    sysctls:
                      description: Sysctls hold a list of namespaced sysctls used
                        for the pod. Pods with unsupported sysctls (by the container
                        runtime) might fail to launch.
                      type: array
                      items:
                        description: Sysctl defines a kernel parameter to be set
                        type: object
                        required:
                        - name
                        - value
                        properties:
                          name:
                            description: Name of a property to set
                            type: string
                          value:
                            description: Value of a property to set
                            type: string
                    windowsOptions:
                      description: The Windows specific settings applied to all containers.
                        If unspecified, the options within a container's SecurityContext
                        will be used. If set in both SecurityContext and PodSecurityContext,
                        the value specified in SecurityContext takes precedence.
                      type: object
                      properties:
                        gmsaCredentialSpec:
                          description: GMSACredentialSpec is where the GMSA admission
                            webhook (https://github.com/kubernetes-sigs/windows-gmsa)
                            inlines the contents of the GMSA credential spec named
                            by the GMSACredentialSpecName field.
                          type: string
                        gmsaCredentialSpecName:
                          description: GMSACredentialSpecName is the name of the GMSA
                            credential spec to use.
                          type: string
                        runAsUserName:
                          description: The UserName in Windows to run the entrypoint
                            of the container process. Defaults to the user specified
                            in image metadata if unspecified. May also be set in PodSecurityContext.
                            If set in both SecurityContext and PodSecurityContext,
                            the value specified in SecurityContext takes precedence.
                          type: string
                serviceAccount:
                  description: 'DeprecatedServiceAccount is a depreciated alias for
                    ServiceAccountName. Deprecated: Use serviceAccountName instead.'
                  type: string
                serviceAccountName:
                  description: 'ServiceAccountName is the name of the ServiceAccount
                    to use to run this pod. More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/'
                  type: string
                shareProcessNamespace:
                  description: 'Share a single process namespace between all of the
                    containers in a pod. When this is set containers will be able
                    to view and signal processes from other containers in the same
                    pod, and the first process in each container will not be assigned
                    PID 1. HostPID and ShareProcessNamespace cannot both be set. Optional:
                    Default to false.'
                  type: boolean
                subdomain:
                  description: If specified, the fully qualified Pod hostname will
                    be "<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>".
                    If not specified, the pod will not have a domainname at all.
                  type: string
                terminationGracePeriodSeconds:
                  description: Optional duration in seconds the pod needs to terminate
                    gracefully. May be decreased in delete request. Value must be
                    non-negative integer. The value zero indicates delete immediately.
                    If this value is nil, the default grace period will be used instead.
                    The grace period is the duration in seconds after the processes
                    running in the pod are sent a termination signal and the time
                    when the processes are forcibly halted with a kill signal. Set
                    this value longer than the expected cleanup time for your process.
                    Defaults to 30 seconds.
                  type: integer
                  format: int64
                tolerations:
                  description: If specified, the pod's tolerations.
                  type: array
                  items:
                    description: The pod this Toleration is attached to tolerates
                      any taint that matches the triple <key,value,effect> using the
                      matching operator <operator>.
                    type: object
                    properties:
                      effect:
                        description: Effect indicates the taint effect to match. Empty
                          means match all taint effects. When specified, allowed values
                          are NoSchedule, PreferNoSchedule and NoExecute.
                        type: string
                      key:
                        description: Key is the taint key that the toleration applies
                          to. Empty means match all taint keys. If the key is empty,
                          operator must be Exists; this combination means to match
                          all values and all keys.
                        type: string
                      operator:
                        description: Operator represents a key's relationship to the
                          value. Valid operators are Exists and Equal. Defaults to
                          Equal. Exists is equivalent to wildcard for value, so that
                          a pod can tolerate all taints of a particular category.
                        type: string
                      tolerationSeconds:
                        description: TolerationSeconds represents the period of time
                          the toleration (which must be of effect NoExecute, otherwise
                          this field is ignored) tolerates the taint. By default,
                          it is not set, which means tolerate the taint forever (do
                          not evict). Zero and negative values will be treated as
                          0 (evict immediately) by the system.
                        type: integer
                        format: int64
                      value:
                        description: Value is the taint value the toleration matches
                          to. If the operator is Exists, the value should be empty,
                          otherwise just a regular string.
                        type: string
                topologySpreadConstraints:
                  description: TopologySpreadConstraints describes how a group of
                    pods ought to spread across topology domains. Scheduler will schedule
                    pods in a way which abides by the constraints. This field is only
                    honored by clusters that enable the EvenPodsSpread feature. All
                    topologySpreadConstraints are ANDed.
                  type: array
                  items:
                    description: TopologySpreadConstraint specifies how to spread
                      matching pods among the given topology.
                    type: object
                    required:
                    - maxSkew
                    - topologyKey
                    - whenUnsatisfiable
                    properties:
                      labelSelector:
                        description: LabelSelector is used to find matching pods.
                          Pods that match this label selector are counted to determine
                          the number of pods in their corresponding topology domain.
                        type: object
                        properties:
                          matchExpressions:
                            description: matchExpressions is a list of label selector
                              requirements. The requirements are ANDed.
                            type: array
                            items:
                              description: A label selector requirement is a selector
                                that contains values, a key, and an operator that
                                relates the key and values.
                              type: object
                              required:
                              - key
                              - operator
                              properties:
                                key:
                                  description: key is the label key that the selector
                                    applies to.
                                  type: string
                                operator:
                                  description: operator represents a key's relationship
                                    to a set of values. Valid operators are In, NotIn,
                                    Exists and DoesNotExist.
                                  type: string
                                values:
                                  description: values is an array of string values.
                                    If the operator is In or NotIn, the values array
                                    must be non-empty. If the operator is Exists or
                                    DoesNotExist, the values array must be empty.
                                    This array is replaced during a strategic merge
                                    patch.
                                  type: array
                                  items:
                                    type: string
                          matchLabels:
                            description: matchLabels is a map of {key,value} pairs.
                              A single {key,value} in the matchLabels map is equivalent
                              to an element of matchExpressions, whose key field is
                              "key", the operator is "In", and the values array contains
                              only "value". The requirements are ANDed.
                            type: object
                            additionalProperties:
                              type: string
                      maxSkew:
                        description: 'MaxSkew describes the degree to which pods may
                          be unevenly distributed. It''s the maximum permitted difference
                          between the number of matching pods in any two topology
                          domains of a given topology type. For example, in a 3-zone
                          cluster, MaxSkew is set to 1, and pods with the same labelSelector
                          spread as 1/1/0: | zone1 | zone2 | zone3 | |   P   |   P   |       |
                          - if MaxSkew is 1, incoming pod can only be scheduled to
                          zone3 to become 1/1/1; scheduling it onto zone1(zone2) would
                          make the ActualSkew(2-0) on zone1(zone2) violate MaxSkew(1).
                          - if MaxSkew is 2, incoming pod can be scheduled onto any
                          zone. It''s a required field. Default value is 1 and 0 is
                          not allowed.'
                        type: integer
                        format: int32
                      topologyKey:
                        description: TopologyKey is the key of node labels. Nodes
                          that have a label with this key and identical values are
                          considered to be in the same topology. We consider each
                          <key, value> as a "bucket", and try to put balanced number
                          of pods into each bucket. It's a required field.
                        type: string
                      whenUnsatisfiable:
                        description: 'WhenUnsatisfiable indicates how to deal with
                          a pod if it doesn''t satisfy the spread constraint. - DoNotSchedule
                          (default) tells the scheduler not to schedule it - ScheduleAnyway
                          tells the scheduler to still schedule it It''s considered
                          as "Unsatisfiable" if and only if placing incoming pod on
                          any topology violates "MaxSkew". For example, in a 3-zone
                          cluster, MaxSkew is set to 1, and pods with the same labelSelector
                          spread as 3/1/1: | zone1 | zone2 | zone3 | | P P P |   P   |   P   |
                          If WhenUnsatisfiable is set to DoNotSchedule, incoming pod
                          can only be scheduled to zone2(zone3) to become 3/2/1(3/1/2)
                          as ActualSkew(2-1) on zone2(zone3) satisfies MaxSkew(1).
                          In other words, the cluster can still be imbalanced, but
                          scheduler won''t make it *more* imbalanced. It''s a required
                          field.'
                        type: string
                volumes:
                  description: 'List of volumes that can be mounted by containers
                    belonging to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes'
                  type: array
                  items:
                    description: Volume represents a named volume in a pod that may
                      be accessed by any container in the pod.
                    type: object
                    required:
                    - name
                    properties:
                      awsElasticBlockStore:
                        description: 'AWSElasticBlockStore represents an AWS Disk
                          resource that is attached to a kubelet''s host machine and
                          then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore'
                        type: object
                        required:
                        - volumeID
                        properties:
                          fsType:
                            description: 'Filesystem type of the volume that you want
                              to mount. Tip: Ensure that the filesystem type is supported
                              by the host operating system. Examples: "ext4", "xfs",
                              "ntfs". Implicitly inferred to be "ext4" if unspecified.
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
                              TODO: how do we prevent errors in the filesystem from
                              compromising the machine'
                            type: string
                          partition:
                            description: 'The partition in the volume that you want
                              to mount. If omitted, the default is to mount by volume
                              name. Examples: For volume /dev/sda1, you specify the
                              partition as "1". Similarly, the volume partition for
                              /dev/sda is "0" (or you can leave the property empty).'
                            type: integer
                            format: int32
                          readOnly:
                            description: 'Specify "true" to force and set the ReadOnly
                              property in VolumeMounts to "true". If omitted, the
                              default is "false". More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore'
                            type: boolean
                          volumeID:
                            description: 'Unique ID of the persistent disk resource
                              in AWS (Amazon EBS volume). More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore'
                            type: string
                      azureDisk:
                        description: AzureDisk represents an Azure Data Disk mount
                          on the host and bind mount to the pod.
                        type: object
                        required:
                        - diskName
                        - diskURI
                        properties:
                          cachingMode:
                            description: 'Host Caching mode: None, Read Only, Read
                              Write.'
                            type: string
                          diskName:
                            description: The Name of the data disk in the blob storage
                            type: string
                          diskURI:
                            description: The URI the data disk in the blob storage
                            type: string
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
                            type: string
                          kind:
                            description: 'Expected values Shared: multiple blob disks
                              per storage account  Dedicated: single blob disk per
                              storage account  Managed: azure managed data disk (only
                              in managed availability set). defaults to shared'
                            type: string
                          readOnly:
                            description: Defaults to false (read/write). ReadOnly
                              here will force the ReadOnly setting in VolumeMounts.
                            type: boolean
                      azureFile:
                        description: AzureFile represents an Azure File Service mount
                          on the host and bind mount to the pod.
                        type: object
                        required:
                        - secretName
                        - shareName
                        properties:
                          readOnly:
                            description: Defaults to false (read/write). ReadOnly
                              here will force the ReadOnly setting in VolumeMounts.
                            type: boolean
                          secretName:
                            description: the name of secret that contains Azure Storage
                              Account Name and Key
                            type: string
                          shareName:
                            description: Share Name
                            type: string
                      cephfs:
                        description: CephFS represents a Ceph FS mount on the host
                          that shares a pod's lifetime
                        type: object
                        required:
                        - monitors
                        properties:
                          monitors:
                            description: 'Required: Monitors is a collection of Ceph
                              monitors More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it'
                            type: array
                            items:
                              type: string
                          path:
                            description: 'Optional: Used as the mounted root, rather
                              than the full Ceph tree, default is /'
                            type: string
                          readOnly:
                            description: 'Optional: Defaults to false (read/write).
                              ReadOnly here will force the ReadOnly setting in VolumeMounts.
                              More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it'
                            type: boolean
                          secretFile:
                            description: 'Optional: SecretFile is the path to key
                              ring for User, default is /etc/ceph/user.secret More
                              info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it'
                            type: string
                          secretRef:
                            description: 'Optional: SecretRef is reference to the
                              authentication secret for User, default is empty. More
                              info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it'
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          user:
                            description: 'Optional: User is the rados user name, default
                              is admin More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it'
                            type: string
                      cinder:
                        description: 'Cinder represents a cinder volume attached and
                          mounted on kubelets host machine. More info: https://examples.k8s.io/mysql-cinder-pd/README.md'
                        type: object
                        required:
                        - volumeID
                        properties:
                          fsType:
                            description: 'Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Examples:
                              "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4"
                              if unspecified. More info: https://examples.k8s.io/mysql-cinder-pd/README.md'
                            type: string
                          readOnly:
                            description: 'Optional: Defaults to false (read/write).
                              ReadOnly here will force the ReadOnly setting in VolumeMounts.
                              More info: https://examples.k8s.io/mysql-cinder-pd/README.md'
                            type: boolean
                          secretRef:
                            description: 'Optional: points to a secret object containing
                              parameters used to connect to OpenStack.'
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          volumeID:
                            description: 'volume id used to identify the volume in
                              cinder. More info: https://examples.k8s.io/mysql-cinder-pd/README.md'
                            type: string
                      configMap:
                        description: ConfigMap represents a configMap that should
                          populate this volume
                        type: object
                        properties:
                          defaultMode:
                            description: 'Optional: mode bits to use on created files
                              by default. Must be a value between 0 and 0777. Defaults
                              to 0644. Directories within the path are not affected
                              by this setting. This might be in conflict with other
                              options that affect the file mode, like fsGroup, and
                              the result can be other mode bits set.'
                            type: integer
                            format: int32
                          items:
                            description: If unspecified, each key-value pair in the
                              Data field of the referenced ConfigMap will be projected
                              into the volume as a file whose name is the key and
                              content is the value. If specified, the listed keys
                              will be projected into the specified paths, and unlisted
                              keys will not be present. If a key is specified which
                              is not present in the ConfigMap, the volume setup will
                              error unless it is marked optional. Paths must be relative
                              and may not contain the '..' path or start with '..'.
                            type: array
                            items:
                              description: Maps a string key to a path within a volume.
                              type: object
                              required:
                              - key
                              - path
                              properties:
                                key:
                                  description: The key to project.
                                  type: string
                                mode:
                                  description: 'Optional: mode bits to use on this
                                    file, must be a value between 0 and 0777. If not
                                    specified, the volume defaultMode will be used.
                                    This might be in conflict with other options that
                                    affect the file mode, like fsGroup, and the result
                                    can be other mode bits set.'
                                  type: integer
                                  format: int32
                                path:
                                  description: The relative path of the file to map
                                    the key to. May not be an absolute path. May not
                                    contain the path element '..'. May not start with
                                    the string '..'.
                                  type: string
                          name:
                            description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                              TODO: Add other useful fields. apiVersion, kind, uid?'
                            type: string
                          optional:
                            description: Specify whether the ConfigMap or its keys
                              must be defined
                            type: boolean
                      csi:
                        description: CSI (Container Storage Interface) represents
                          storage that is handled by an external CSI driver (Alpha
                          feature).
                        type: object
                        required:
                        - driver
                        properties:
                          driver:
                            description: Driver is the name of the CSI driver that
                              handles this volume. Consult with your admin for the
                              correct name as registered in the cluster.
                            type: string
                          fsType:
                            description: Filesystem type to mount. Ex. "ext4", "xfs",
                              "ntfs". If not provided, the empty value is passed to
                              the associated CSI driver which will determine the default
                              filesystem to apply.
                            type: string
                          nodePublishSecretRef:
                            description: NodePublishSecretRef is a reference to the
                              secret object containing sensitive information to pass
                              to the CSI driver to complete the CSI NodePublishVolume
                              and NodeUnpublishVolume calls. This field is optional,
                              and  may be empty if no secret is required. If the secret
                              object contains more than one secret, all secret references
                              are passed.
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          readOnly:
                            description: Specifies a read-only configuration for the
                              volume. Defaults to false (read/write).
                            type: boolean
                          volumeAttributes:
                            description: VolumeAttributes stores driver-specific properties
                              that are passed to the CSI driver. Consult your driver's
                              documentation for supported values.
                            type: object
                            additionalProperties:
                              type: string
                      downwardAPI:
                        description: DownwardAPI represents downward API about the
                          pod that should populate this volume
                        type: object
                        properties:
                          defaultMode:
                            description: 'Optional: mode bits to use on created files
                              by default. Must be a value between 0 and 0777. Defaults
                              to 0644. Directories within the path are not affected
                              by this setting. This might be in conflict with other
                              options that affect the file mode, like fsGroup, and
                              the result can be other mode bits set.'
                            type: integer
                            format: int32
                          items:
                            description: Items is a list of downward API volume file
                            type: array
                            items:
                              description: DownwardAPIVolumeFile represents information
                                to create the file containing the pod field
                              type: object
                              required:
                              - path
                              properties:
                                fieldRef:
                                  description: 'Required: Selects a field of the pod:
                                    only annotations, labels, name and namespace are
                                    supported.'
                                  type: object
                                  required:
                                  - fieldPath
                                  properties:
                                    apiVersion:
                                      description: Version of the schema the FieldPath
                                        is written in terms of, defaults to "v1".
                                      type: string
                                    fieldPath:
                                      description: Path of the field to select in
                                        the specified API version.
                                      type: string
                                mode:
                                  description: 'Optional: mode bits to use on this
                                    file, must be a value between 0 and 0777. If not
                                    specified, the volume defaultMode will be used.
                                    This might be in conflict with other options that
                                    affect the file mode, like fsGroup, and the result
                                    can be other mode bits set.'
                                  type: integer
                                  format: int32
                                path:
                                  description: 'Required: Path is  the relative path
                                    name of the file to be created. Must not be absolute
                                    or contain the ''..'' path. Must be utf-8 encoded.
                                    The first item of the relative path must not start
                                    with ''..'''
                                  type: string
                                resourceFieldRef:
                                  description: 'Selects a resource of the container:
                                    only resources limits and requests (limits.cpu,
                                    limits.memory, requests.cpu and requests.memory)
                                    are currently supported.'
                                  type: object
                                  required:
                                  - resource
                                  properties:
                                    containerName:
                                      description: 'Container name: required for volumes,
                                        optional for env vars'
                                      type: string
                                    divisor:
                                      description: Specifies the output format of
                                        the exposed resources, defaults to "1"
                                      type: string
                                    resource:
                                      description: 'Required: resource to select'
                                      type: string
                      emptyDir:
                        description: 'EmptyDir represents a temporary directory that
                          shares a pod''s lifetime. More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir'
                        type: object
                        properties:
                          medium:
                            description: 'What type of storage medium should back
                              this directory. The default is "" which means to use
                              the node''s default medium. Must be an empty string
                              (default) or Memory. More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir'
                            type: string
                          sizeLimit:
                            description: 'Total amount of local storage required for
                              this EmptyDir volume. The size limit is also applicable
                              for memory medium. The maximum usage on memory medium
                              EmptyDir would be the minimum value between the SizeLimit
                              specified here and the sum of memory limits of all containers
                              in a pod. The default is nil which means that the limit
                              is undefined. More info: http://kubernetes.io/docs/user-guide/volumes#emptydir'
                            type: string
                      fc:
                        description: FC represents a Fibre Channel resource that is
                          attached to a kubelet's host machine and then exposed to
                          the pod.
                        type: object
                        properties:
                          fsType:
                            description: 'Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
                              TODO: how do we prevent errors in the filesystem from
                              compromising the machine'
                            type: string
                          lun:
                            description: 'Optional: FC target lun number'
                            type: integer
                            format: int32
                          readOnly:
                            description: 'Optional: Defaults to false (read/write).
                              ReadOnly here will force the ReadOnly setting in VolumeMounts.'
                            type: boolean
                          targetWWNs:
                            description: 'Optional: FC target worldwide names (WWNs)'
                            type: array
                            items:
                              type: string
                          wwids:
                            description: 'Optional: FC volume world wide identifiers
                              (wwids) Either wwids or combination of targetWWNs and
                              lun must be set, but not both simultaneously.'
                            type: array
                            items:
                              type: string
                      flexVolume:
                        description: FlexVolume represents a generic volume resource
                          that is provisioned/attached using an exec based plugin.
                        type: object
                        required:
                        - driver
                        properties:
                          driver:
                            description: Driver is the name of the driver to use for
                              this volume.
                            type: string
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". The default filesystem depends on FlexVolume
                              script.
                            type: string
                          options:
                            description: 'Optional: Extra command options if any.'
                            type: object
                            additionalProperties:
                              type: string
                          readOnly:
                            description: 'Optional: Defaults to false (read/write).
                              ReadOnly here will force the ReadOnly setting in VolumeMounts.'
                            type: boolean
                          secretRef:
                            description: 'Optional: SecretRef is reference to the
                              secret object containing sensitive information to pass
                              to the plugin scripts. This may be empty if no secret
                              object is specified. If the secret object contains more
                              than one secret, all secrets are passed to the plugin
                              scripts.'
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                      flocker:
                        description: Flocker represents a Flocker volume attached
                          to a kubelet's host machine. This depends on the Flocker
                          control service being running
                        type: object
                        properties:
                          datasetName:
                            description: Name of the dataset stored as metadata ->
                              name on the dataset for Flocker should be considered
                              as deprecated
                            type: string
                          datasetUUID:
                            description: UUID of the dataset. This is unique identifier
                              of a Flocker dataset
                            type: string
                      gcePersistentDisk:
                        description: 'GCEPersistentDisk represents a GCE Disk resource
                          that is attached to a kubelet''s host machine and then exposed
                          to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk'
                        type: object
                        required:
                        - pdName
                        properties:
                          fsType:
                            description: 'Filesystem type of the volume that you want
                              to mount. Tip: Ensure that the filesystem type is supported
                              by the host operating system. Examples: "ext4", "xfs",
                              "ntfs". Implicitly inferred to be "ext4" if unspecified.
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
                              TODO: how do we prevent errors in the filesystem from
                              compromising the machine'
                            type: string
                          partition:
                            description: 'The partition in the volume that you want
                              to mount. If omitted, the default is to mount by volume
                              name. Examples: For volume /dev/sda1, you specify the
                              partition as "1". Similarly, the volume partition for
                              /dev/sda is "0" (or you can leave the property empty).
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk'
                            type: integer
                            format: int32
                          pdName:
                            description: 'Unique name of the PD resource in GCE. Used
                              to identify the disk in GCE. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk'
                            type: string
                          readOnly:
                            description: 'ReadOnly here will force the ReadOnly setting
                              in VolumeMounts. Defaults to false. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk'
                            type: boolean
                      gitRepo:
                        description: 'GitRepo represents a git repository at a particular
                          revision. DEPRECATED: GitRepo is deprecated. To provision
                          a container with a git repo, mount an EmptyDir into an InitContainer
                          that clones the repo using git, then mount the EmptyDir
                          into the Pod''s container.'
                        type: object
                        required:
                        - repository
                        properties:
                          directory:
                            description: Target directory name. Must not contain or
                              start with '..'.  If '.' is supplied, the volume directory
                              will be the git repository.  Otherwise, if specified,
                              the volume will contain the git repository in the subdirectory
                              with the given name.
                            type: string
                          repository:
                            description: Repository URL
                            type: string
                          revision:
                            description: Commit hash for the specified revision.
                            type: string
                      glusterfs:
                        description: 'Glusterfs represents a Glusterfs mount on the
                          host that shares a pod''s lifetime. More info: https://examples.k8s.io/volumes/glusterfs/README.md'
                        type: object
                        required:
                        - endpoints
                        - path
                        properties:
                          endpoints:
                            description: 'EndpointsName is the endpoint name that
                              details Glusterfs topology. More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod'
                            type: string
                          path:
                            description: 'Path is the Glusterfs volume path. More
                              info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod'
                            type: string
                          readOnly:
                            description: 'ReadOnly here will force the Glusterfs volume
                              to be mounted with read-only permissions. Defaults to
                              false. More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod'
                            type: boolean
                      hostPath:
                        description: 'HostPath represents a pre-existing file or directory
                          on the host machine that is directly exposed to the container.
                          This is generally used for system agents or other privileged
                          things that are allowed to see the host machine. Most containers
                          will NOT need this. More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
                          --- TODO(jonesdl) We need to restrict who can use host directory
                          mounts and who can/can not mount host directories as read/write.'
                        type: object
                        required:
                        - path
                        properties:
                          path:
                            description: 'Path of the directory on the host. If the
                              path is a symlink, it will follow the link to the real
                              path. More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath'
                            type: string
                          type:
                            description: 'Type for HostPath Volume Defaults to ""
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath'
                            type: string
                      iscsi:
                        description: 'ISCSI represents an ISCSI Disk resource that
                          is attached to a kubelet''s host machine and then exposed
                          to the pod. More info: https://examples.k8s.io/volumes/iscsi/README.md'
                        type: object
                        required:
                        - iqn
                        - lun
                        - targetPortal
                        properties:
                          chapAuthDiscovery:
                            description: whether support iSCSI Discovery CHAP authentication
                            type: boolean
                          chapAuthSession:
                            description: whether support iSCSI Session CHAP authentication
                            type: boolean
                          fsType:
                            description: 'Filesystem type of the volume that you want
                              to mount. Tip: Ensure that the filesystem type is supported
                              by the host operating system. Examples: "ext4", "xfs",
                              "ntfs". Implicitly inferred to be "ext4" if unspecified.
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#iscsi
                              TODO: how do we prevent errors in the filesystem from
                              compromising the machine'
                            type: string
                          initiatorName:
                            description: Custom iSCSI Initiator Name. If initiatorName
                              is specified with iscsiInterface simultaneously, new
                              iSCSI interface <target portal>:<volume name> will be
                              created for the connection.
                            type: string
                          iqn:
                            description: Target iSCSI Qualified Name.
                            type: string
                          iscsiInterface:
                            description: iSCSI Interface Name that uses an iSCSI transport.
                              Defaults to 'default' (tcp).
                            type: string
                          lun:
                            description: iSCSI Target Lun number.
                            type: integer
                            format: int32
                          portals:
                            description: iSCSI Target Portal List. The portal is either
                              an IP or ip_addr:port if the port is other than default
                              (typically TCP ports 860 and 3260).
                            type: array
                            items:
                              type: string
                          readOnly:
                            description: ReadOnly here will force the ReadOnly setting
                              in VolumeMounts. Defaults to false.
                            type: boolean
                          secretRef:
                            description: CHAP Secret for iSCSI target and initiator
                              authentication
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          targetPortal:
                            description: iSCSI Target Portal. The Portal is either
                              an IP or ip_addr:port if the port is other than default
                              (typically TCP ports 860 and 3260).
                            type: string
                      name:
                        description: 'Volume''s name. Must be a DNS_LABEL and unique
                          within the pod. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names'
                        type: string
                      nfs:
                        description: 'NFS represents an NFS mount on the host that
                          shares a pod''s lifetime More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs'
                        type: object
                        required:
                        - path
                        - server
                        properties:
                          path:
                            description: 'Path that is exported by the NFS server.
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs'
                            type: string
                          readOnly:
                            description: 'ReadOnly here will force the NFS export
                              to be mounted with read-only permissions. Defaults to
                              false. More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs'
                            type: boolean
                          server:
                            description: 'Server is the hostname or IP address of
                              the NFS server. More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs'
                            type: string
                      persistentVolumeClaim:
                        description: 'PersistentVolumeClaimVolumeSource represents
                          a reference to a PersistentVolumeClaim in the same namespace.
                          More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims'
                        type: object
                        required:
                        - claimName
                        properties:
                          claimName:
                            description: 'ClaimName is the name of a PersistentVolumeClaim
                              in the same namespace as the pod using this volume.
                              More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims'
                            type: string
                          readOnly:
                            description: Will force the ReadOnly setting in VolumeMounts.
                              Default false.
                            type: boolean
                      photonPersistentDisk:
                        description: PhotonPersistentDisk represents a PhotonController
                          persistent disk attached and mounted on kubelets host machine
                        type: object
                        required:
                        - pdID
                        properties:
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
                            type: string
                          pdID:
                            description: ID that identifies Photon Controller persistent
                              disk
                            type: string
                      portworxVolume:
                        description: PortworxVolume represents a portworx volume attached
                          and mounted on kubelets host machine
                        type: object
                        required:
                        - volumeID
                        properties:
                          fsType:
                            description: FSType represents the filesystem type to
                              mount Must be a filesystem type supported by the host
                              operating system. Ex. "ext4", "xfs". Implicitly inferred
                              to be "ext4" if unspecified.
                            type: string
                          readOnly:
                            description: Defaults to false (read/write). ReadOnly
                              here will force the ReadOnly setting in VolumeMounts.
                            type: boolean
                          volumeID:
                            description: VolumeID uniquely identifies a Portworx volume
                            type: string
                      projected:
                        description: Items for all in one resources secrets, configmaps,
                          and downward API
                        type: object
                        required:
                        - sources
                        properties:
                          defaultMode:
                            description: Mode bits to use on created files by default.
                              Must be a value between 0 and 0777. Directories within
                              the path are not affected by this setting. This might
                              be in conflict with other options that affect the file
                              mode, like fsGroup, and the result can be other mode
                              bits set.
                            type: integer
                            format: int32
                          sources:
                            description: list of volume projections
                            type: array
                            items:
                              description: Projection that may be projected along
                                with other supported volume types
                              type: object
                              properties:
                                configMap:
                                  description: information about the configMap data
                                    to project
                                  type: object
                                  properties:
                                    items:
                                      description: If unspecified, each key-value
                                        pair in the Data field of the referenced ConfigMap
                                        will be projected into the volume as a file
                                        whose name is the key and content is the value.
                                        If specified, the listed keys will be projected
                                        into the specified paths, and unlisted keys
                                        will not be present. If a key is specified
                                        which is not present in the ConfigMap, the
                                        volume setup will error unless it is marked
                                        optional. Paths must be relative and may not
                                        contain the '..' path or start with '..'.
                                      type: array
                                      items:
                                        description: Maps a string key to a path within
                                          a volume.
                                        type: object
                                        required:
                                        - key
                                        - path
                                        properties:
                                          key:
                                            description: The key to project.
                                            type: string
                                          mode:
                                            description: 'Optional: mode bits to use
                                              on this file, must be a value between
                                              0 and 0777. If not specified, the volume
                                              defaultMode will be used. This might
                                              be in conflict with other options that
                                              affect the file mode, like fsGroup,
                                              and the result can be other mode bits
                                              set.'
                                            type: integer
                                            format: int32
                                          path:
                                            description: The relative path of the
                                              file to map the key to. May not be an
                                              absolute path. May not contain the path
                                              element '..'. May not start with the
                                              string '..'.
                                            type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the ConfigMap or
                                        its keys must be defined
                                      type: boolean
                                downwardAPI:
                                  description: information about the downwardAPI data
                                    to project
                                  type: object
                                  properties:
                                    items:
                                      description: Items is a list of DownwardAPIVolume
                                        file
                                      type: array
                                      items:
                                        description: DownwardAPIVolumeFile represents
                                          information to create the file containing
                                          the pod field
                                        type: object
                                        required:
                                        - path
                                        properties:
                                          fieldRef:
                                            description: 'Required: Selects a field
                                              of the pod: only annotations, labels,
                                              name and namespace are supported.'
                                            type: object
                                            required:
                                            - fieldPath
                                            properties:
                                              apiVersion:
                                                description: Version of the schema
                                                  the FieldPath is written in terms
                                                  of, defaults to "v1".
                                                type: string
                                              fieldPath:
                                                description: Path of the field to
                                                  select in the specified API version.
                                                type: string
                                          mode:
                                            description: 'Optional: mode bits to use
                                              on this file, must be a value between
                                              0 and 0777. If not specified, the volume
                                              defaultMode will be used. This might
                                              be in conflict with other options that
                                              affect the file mode, like fsGroup,
                                              and the result can be other mode bits
                                              set.'
                                            type: integer
                                            format: int32
                                          path:
                                            description: 'Required: Path is  the relative
                                              path name of the file to be created.
                                              Must not be absolute or contain the
                                              ''..'' path. Must be utf-8 encoded.
                                              The first item of the relative path
                                              must not start with ''..'''
                                            type: string
                                          resourceFieldRef:
                                            description: 'Selects a resource of the
                                              container: only resources limits and
                                              requests (limits.cpu, limits.memory,
                                              requests.cpu and requests.memory) are
                                              currently supported.'
                                            type: object
                                            required:
                                            - resource
                                            properties:
                                              containerName:
                                                description: 'Container name: required
                                                  for volumes, optional for env vars'
                                                type: string
                                              divisor:
                                                description: Specifies the output
                                                  format of the exposed resources,
                                                  defaults to "1"
                                                type: string
                                              resource:
                                                description: 'Required: resource to
                                                  select'
                                                type: string
                                secret:
                                  description: information about the secret data to
                                    project
                                  type: object
                                  properties:
                                    items:
                                      description: If unspecified, each key-value
                                        pair in the Data field of the referenced Secret
                                        will be projected into the volume as a file
                                        whose name is the key and content is the value.
                                        If specified, the listed keys will be projected
                                        into the specified paths, and unlisted keys
                                        will not be present. If a key is specified
                                        which is not present in the Secret, the volume
                                        setup will error unless it is marked optional.
                                        Paths must be relative and may not contain
                                        the '..' path or start with '..'.
                                      type: array
                                      items:
                                        description: Maps a string key to a path within
                                          a volume.
                                        type: object
                                        required:
                                        - key
                                        - path
                                        properties:
                                          key:
                                            description: The key to project.
                                            type: string
                                          mode:
                                            description: 'Optional: mode bits to use
                                              on this file, must be a value between
                                              0 and 0777. If not specified, the volume
                                              defaultMode will be used. This might
                                              be in conflict with other options that
                                              affect the file mode, like fsGroup,
                                              and the result can be other mode bits
                                              set.'
                                            type: integer
                                            format: int32
                                          path:
                                            description: The relative path of the
                                              file to map the key to. May not be an
                                              absolute path. May not contain the path
                                              element '..'. May not start with the
                                              string '..'.
                                            type: string
                                    name:
                                      description: 'Name of the referent. More info:
                                        https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                        TODO: Add other useful fields. apiVersion,
                                        kind, uid?'
                                      type: string
                                    optional:
                                      description: Specify whether the Secret or its
                                        key must be defined
                                      type: boolean
                                serviceAccountToken:
                                  description: information about the serviceAccountToken
                                    data to project
                                  type: object
                                  required:
                                  - path
                                  properties:
                                    audience:
                                      description: Audience is the intended audience
                                        of the token. A recipient of a token must
                                        identify itself with an identifier specified
                                        in the audience of the token, and otherwise
                                        should reject the token. The audience defaults
                                        to the identifier of the apiserver.
                                      type: string
                                    expirationSeconds:
                                      description: ExpirationSeconds is the requested
                                        duration of validity of the service account
                                        token. As the token approaches expiration,
                                        the kubelet volume plugin will proactively
                                        rotate the service account token. The kubelet
                                        will start trying to rotate the token if the
                                        token is older than 80 percent of its time
                                        to live or if the token is older than 24 hours.Defaults
                                        to 1 hour and must be at least 10 minutes.
                                      type: integer
                                      format: int64
                                    path:
                                      description: Path is the path relative to the
                                        mount point of the file to project the token
                                        into.
                                      type: string
                      quobyte:
                        description: Quobyte represents a Quobyte mount on the host
                          that shares a pod's lifetime
                        type: object
                        required:
                        - registry
                        - volume
                        properties:
                          group:
                            description: Group to map volume access to Default is
                              no group
                            type: string
                          readOnly:
                            description: ReadOnly here will force the Quobyte volume
                              to be mounted with read-only permissions. Defaults to
                              false.
                            type: boolean
                          registry:
                            description: Registry represents a single or multiple
                              Quobyte Registry services specified as a string as host:port
                              pair (multiple entries are separated with commas) which
                              acts as the central registry for volumes
                            type: string
                          tenant:
                            description: Tenant owning the given Quobyte volume in
                              the Backend Used with dynamically provisioned Quobyte
                              volumes, value is set by the plugin
                            type: string
                          user:
                            description: User to map volume access to Defaults to
                              serivceaccount user
                            type: string
                          volume:
                            description: Volume is a string that references an already
                              created Quobyte volume by name.
                            type: string
                      rbd:
                        description: 'RBD represents a Rados Block Device mount on
                          the host that shares a pod''s lifetime. More info: https://examples.k8s.io/volumes/rbd/README.md'
                        type: object
                        required:
                        - image
                        - monitors
                        properties:
                          fsType:
                            description: 'Filesystem type of the volume that you want
                              to mount. Tip: Ensure that the filesystem type is supported
                              by the host operating system. Examples: "ext4", "xfs",
                              "ntfs". Implicitly inferred to be "ext4" if unspecified.
                              More info: https://kubernetes.io/docs/concepts/storage/volumes#rbd
                              TODO: how do we prevent errors in the filesystem from
                              compromising the machine'
                            type: string
                          image:
                            description: 'The rados image name. More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: string
                          keyring:
                            description: 'Keyring is the path to key ring for RBDUser.
                              Default is /etc/ceph/keyring. More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: string
                          monitors:
                            description: 'A collection of Ceph monitors. More info:
                              https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: array
                            items:
                              type: string
                          pool:
                            description: 'The rados pool name. Default is rbd. More
                              info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: string
                          readOnly:
                            description: 'ReadOnly here will force the ReadOnly setting
                              in VolumeMounts. Defaults to false. More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: boolean
                          secretRef:
                            description: 'SecretRef is name of the authentication
                              secret for RBDUser. If provided overrides keyring. Default
                              is nil. More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          user:
                            description: 'The rados user name. Default is admin. More
                              info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it'
                            type: string
                      scaleIO:
                        description: ScaleIO represents a ScaleIO persistent volume
                          attached and mounted on Kubernetes nodes.
                        type: object
                        required:
                        - gateway
                        - secretRef
                        - system
                        properties:
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Default is "xfs".
                            type: string
                          gateway:
                            description: The host address of the ScaleIO API Gateway.
                            type: string
                          protectionDomain:
                            description: The name of the ScaleIO Protection Domain
                              for the configured storage.
                            type: string
                          readOnly:
                            description: Defaults to false (read/write). ReadOnly
                              here will force the ReadOnly setting in VolumeMounts.
                            type: boolean
                          secretRef:
                            description: SecretRef references to the secret for ScaleIO
                              user and other sensitive information. If this is not
                              provided, Login operation will fail.
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          sslEnabled:
                            description: Flag to enable/disable SSL communication
                              with Gateway, default false
                            type: boolean
                          storageMode:
                            description: Indicates whether the storage for a volume
                              should be ThickProvisioned or ThinProvisioned. Default
                              is ThinProvisioned.
                            type: string
                          storagePool:
                            description: The ScaleIO Storage Pool associated with
                              the protection domain.
                            type: string
                          system:
                            description: The name of the storage system as configured
                              in ScaleIO.
                            type: string
                          volumeName:
                            description: The name of a volume already created in the
                              ScaleIO system that is associated with this volume source.
                            type: string
                      secret:
                        description: 'Secret represents a secret that should populate
                          this volume. More info: https://kubernetes.io/docs/concepts/storage/volumes#secret'
                        type: object
                        properties:
                          defaultMode:
                            description: 'Optional: mode bits to use on created files
                              by default. Must be a value between 0 and 0777. Defaults
                              to 0644. Directories within the path are not affected
                              by this setting. This might be in conflict with other
                              options that affect the file mode, like fsGroup, and
                              the result can be other mode bits set.'
                            type: integer
                            format: int32
                          items:
                            description: If unspecified, each key-value pair in the
                              Data field of the referenced Secret will be projected
                              into the volume as a file whose name is the key and
                              content is the value. If specified, the listed keys
                              will be projected into the specified paths, and unlisted
                              keys will not be present. If a key is specified which
                              is not present in the Secret, the volume setup will
                              error unless it is marked optional. Paths must be relative
                              and may not contain the '..' path or start with '..'.
                            type: array
                            items:
                              description: Maps a string key to a path within a volume.
                              type: object
                              required:
                              - key
                              - path
                              properties:
                                key:
                                  description: The key to project.
                                  type: string
                                mode:
                                  description: 'Optional: mode bits to use on this
                                    file, must be a value between 0 and 0777. If not
                                    specified, the volume defaultMode will be used.
                                    This might be in conflict with other options that
                                    affect the file mode, like fsGroup, and the result
                                    can be other mode bits set.'
                                  type: integer
                                  format: int32
                                path:
                                  description: The relative path of the file to map
                                    the key to. May not be an absolute path. May not
                                    contain the path element '..'. May not start with
                                    the string '..'.
                                  type: string
                          optional:
                            description: Specify whether the Secret or its keys must
                              be defined
                            type: boolean
                          secretName:
                            description: 'Name of the secret in the pod''s namespace
                              to use. More info: https://kubernetes.io/docs/concepts/storage/volumes#secret'
                            type: string
                      storageos:
                        description: StorageOS represents a StorageOS volume attached
                          and mounted on Kubernetes nodes.
                        type: object
                        properties:
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
                            type: string
                          readOnly:
                            description: Defaults to false (read/write). ReadOnly
                              here will force the ReadOnly setting in VolumeMounts.
                            type: boolean
                          secretRef:
                            description: SecretRef specifies the secret to use for
                              obtaining the StorageOS API credentials.  If not specified,
                              default values will be attempted.
                            type: object
                            properties:
                              name:
                                description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                                  TODO: Add other useful fields. apiVersion, kind,
                                  uid?'
                                type: string
                          volumeName:
                            description: VolumeName is the human-readable name of
                              the StorageOS volume.  Volume names are only unique
                              within a namespace.
                            type: string
                          volumeNamespace:
                            description: VolumeNamespace specifies the scope of the
                              volume within StorageOS.  If no namespace is specified
                              then the Pod's namespace will be used.  This allows
                              the Kubernetes name scoping to be mirrored within StorageOS
                              for tighter integration. Set VolumeName to any name
                              to override the default behaviour. Set to "default"
                              if you are not using namespaces within StorageOS. Namespaces
                              that do not pre-exist within StorageOS will be created.
                            type: string
                      vsphereVolume:
                        description: VsphereVolume represents a vSphere volume attached
                          and mounted on kubelets host machine
                        type: object
                        required:
                        - volumePath
                        properties:
                          fsType:
                            description: Filesystem type to mount. Must be a filesystem
                              type supported by the host operating system. Ex. "ext4",
                              "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
                            type: string
                          storagePolicyID:
                            description: Storage Policy Based Management (SPBM) profile
                              ID associated with the StoragePolicyName.
                            type: string
                          storagePolicyName:
                            description: Storage Policy Based Management (SPBM) profile
                              name.
                            type: string
                          volumePath:
                            description: Path that identifies vSphere volume vmdk
                            type: string
            prevClusterID:
              description: PrevClusterID is the cluster ID of the previous failed
                provision attempt.
              type: string
            prevInfraID:
              description: PrevInfraID is the infra ID of the previous failed provision
                attempt.
              type: string
            stage:
              description: Stage is the stage of provisioning that the cluster deployment
                has reached.
              type: string
        status:
          description: ClusterProvisionStatus defines the observed state of ClusterProvision.
          type: object
          properties:
            conditions:
              description: Conditions includes more detailed status for the cluster
                provision
              type: array
              items:
                description: ClusterProvisionCondition contains details for the current
                  condition of a cluster provision
                type: object
                required:
                - status
                - type
                properties:
                  lastProbeTime:
                    description: LastProbeTime is the last time we probed the condition.
                    type: string
                    format: date-time
                  lastTransitionTime:
                    description: LastTransitionTime is the last time the condition
                      transitioned from one status to another.
                    type: string
                    format: date-time
                  message:
                    description: Message is a human-readable message indicating details
                      about last transition.
                    type: string
                  reason:
                    description: Reason is a unique, one-word, CamelCase reason for
                      the condition's last transition.
                    type: string
                  status:
                    description: Status is the status of the condition.
                    type: string
                  type:
                    description: Type is the type of the condition.
                    type: string
            jobRef:
              description: JobRef is the reference to the job performing the provision.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_clusterprovisionYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_clusterprovisionYaml, nil
}

func configCrdsHive_v1_clusterprovisionYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_clusterprovisionYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_clusterprovision.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_clusterstateYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusterstates.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: ClusterState
    plural: clusterstates
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: ClusterState is the Schema for the clusterstates API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: ClusterStateSpec defines the desired state of ClusterState
          type: object
        status:
          description: ClusterStateStatus defines the observed state of ClusterState
          type: object
          properties:
            clusterOperators:
              description: ClusterOperators contains the state for every cluster operator
                in the target cluster
              type: array
              items:
                description: ClusterOperatorState summarizes the status of a single
                  cluster operator
                type: object
                required:
                - name
                properties:
                  conditions:
                    description: Conditions is the set of conditions in the status
                      of the cluster operator on the target cluster
                    type: array
                    items:
                      description: ClusterOperatorStatusCondition represents the state
                        of the operator's managed and monitored components.
                      type: object
                      required:
                      - lastTransitionTime
                      - status
                      - type
                      properties:
                        lastTransitionTime:
                          description: lastTransitionTime is the time of the last
                            update to the current status property.
                          type: string
                          format: date-time
                        message:
                          description: message provides additional information about
                            the current condition. This is only to be consumed by
                            humans.
                          type: string
                        reason:
                          description: reason is the CamelCase reason for the condition's
                            current status.
                          type: string
                        status:
                          description: status of the condition, one of True, False,
                            Unknown.
                          type: string
                        type:
                          description: type specifies the aspect reported by this
                            condition.
                          type: string
                  name:
                    description: Name is the name of the cluster operator
                    type: string
            lastUpdated:
              description: LastUpdated is the last time that operator state was updated
              type: string
              format: date-time
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_clusterstateYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_clusterstateYaml, nil
}

func configCrdsHive_v1_clusterstateYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_clusterstateYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_clusterstate.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_dnszoneYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: dnszones.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: DNSZone
    plural: dnszones
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: DNSZone is the Schema for the dnszones API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: DNSZoneSpec defines the desired state of DNSZone
          type: object
          required:
          - zone
          properties:
            aws:
              description: AWS specifies AWS-specific cloud configuration
              type: object
              required:
              - credentialsSecretRef
              properties:
                additionalTags:
                  description: AdditionalTags is a set of additional tags to set on
                    the DNS hosted zone. In addition to these tags,the DNS Zone controller
                    will set a hive.openhsift.io/hostedzone tag identifying the HostedZone
                    record that it belongs to.
                  type: array
                  items:
                    description: AWSResourceTag represents a tag that is applied to
                      an AWS cloud resource
                    type: object
                    required:
                    - key
                    - value
                    properties:
                      key:
                        description: Key is the key for the tag
                        type: string
                      value:
                        description: Value is the value for the tag
                        type: string
                credentialsSecretRef:
                  description: CredentialsSecretRef contains a reference to a secret
                    that contains AWS credentials for CRUD operations
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
                region:
                  description: Region is the AWS region to use for route53 operations.
                    This defaults to us-east-1. For AWS China, use cn-northwest-1.
                  type: string
            gcp:
              description: GCP specifies GCP-specific cloud configuration
              type: object
              required:
              - credentialsSecretRef
              properties:
                credentialsSecretRef:
                  description: CredentialsSecretRef references a secret that will
                    be used to authenticate with GCP CloudDNS. It will need permission
                    to create and manage CloudDNS Hosted Zones. Secret should have
                    a key named 'osServiceAccount.json'. The credentials must specify
                    the project to use.
                  type: object
                  properties:
                    name:
                      description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                        TODO: Add other useful fields. apiVersion, kind, uid?'
                      type: string
            linkToParentDomain:
              description: LinkToParentDomain specifies whether DNS records should
                be automatically created to link this DNSZone with a parent domain.
              type: boolean
            zone:
              description: Zone is the DNS zone to host
              type: string
        status:
          description: DNSZoneStatus defines the observed state of DNSZone
          type: object
          properties:
            aws:
              description: AWSDNSZoneStatus contains status information specific to
                AWS
              type: object
              properties:
                zoneID:
                  description: ZoneID is the ID of the zone in AWS
                  type: string
            conditions:
              description: Conditions includes more detailed status for the DNSZone
              type: array
              items:
                description: DNSZoneCondition contains details for the current condition
                  of a DNSZone
                type: object
                required:
                - status
                - type
                properties:
                  lastProbeTime:
                    description: LastProbeTime is the last time we probed the condition.
                    type: string
                    format: date-time
                  lastTransitionTime:
                    description: LastTransitionTime is the last time the condition
                      transitioned from one status to another.
                    type: string
                    format: date-time
                  message:
                    description: Message is a human-readable message indicating details
                      about last transition.
                    type: string
                  reason:
                    description: Reason is a unique, one-word, CamelCase reason for
                      the condition's last transition.
                    type: string
                  status:
                    description: Status is the status of the condition.
                    type: string
                  type:
                    description: Type is the type of the condition.
                    type: string
            gcp:
              description: GCPDNSZoneStatus contains status information specific to
                GCP
              type: object
              properties:
                zoneName:
                  description: ZoneName is the name of the zone in GCP Cloud DNS
                  type: string
            lastSyncGeneration:
              description: LastSyncGeneration is the generation of the zone resource
                that was last sync'd. This is used to know if the Object has changed
                and we should sync immediately.
              type: integer
              format: int64
            lastSyncTimestamp:
              description: LastSyncTimestamp is the time that the zone was last sync'd.
              type: string
              format: date-time
            nameServers:
              description: NameServers is a list of nameservers for this DNS zone
              type: array
              items:
                type: string
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_dnszoneYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_dnszoneYaml, nil
}

func configCrdsHive_v1_dnszoneYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_dnszoneYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_dnszone.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_hiveconfigYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: hiveconfigs.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: HiveConfig
    plural: hiveconfigs
  scope: Cluster
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: HiveConfig is the Schema for the hives API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: HiveConfigSpec defines the desired state of Hive
          type: object
          properties:
            additionalCertificateAuthoritiesSecretRef:
              description: AdditionalCertificateAuthoritiesSecretRef is a list of
                references to secrets in the TargetNamespace that contain an additional
                Certificate Authority to use when communicating with target clusters.
                These certificate authorities will be used in addition to any self-signed
                CA generated by each cluster on installation.
              type: array
              items:
                description: LocalObjectReference contains enough information to let
                  you locate the referenced object inside the same namespace.
                type: object
                properties:
                  name:
                    description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                      TODO: Add other useful fields. apiVersion, kind, uid?'
                    type: string
            backup:
              description: Backup specifies configuration for backup integration.
                If absent, backup integration will be disabled.
              type: object
              properties:
                minBackupPeriodSeconds:
                  description: MinBackupPeriodSeconds specifies that a minimum of
                    MinBackupPeriodSeconds will occur in between each backup. This
                    is used to rate limit backups. This potentially batches together
                    multiple changes into 1 backup. No backups will be lost as changes
                    that happen during this interval are queued up and will result
                    in a backup happening once the interval has been completed.
                  type: integer
                velero:
                  description: Velero specifies configuration for the Velero backup
                    integration.
                  type: object
                  properties:
                    enabled:
                      description: Enabled dictates if Velero backup integration is
                        enabled. If not specified, the default is disabled.
                      type: boolean
            deprovisionsDisabled:
              description: DeprovisionsDisabled can be set to true to block deprovision
                jobs from running.
              type: boolean
            failedProvisionConfig:
              description: FailedProvisionConfig is used to configure settings related
                to handling provision failures.
              type: object
              properties:
                skipGatherLogs:
                  description: SkipGatherLogs disables functionality that attempts
                    to gather full logs from the cluster if an installation fails
                    for any reason. The logs will be stored in a persistent volume
                    for up to 7 days.
                  type: boolean
            globalPullSecretRef:
              description: GlobalPullSecretRef is used to specify a pull secret that
                will be used globally by all of the cluster deployments. For each
                cluster deployment, the contents of GlobalPullSecret will be merged
                with the specific pull secret for a cluster deployment(if specified),
                with precedence given to the contents of the pull secret for the cluster
                deployment. The global pull secret is assumed to be in the TargetNamespace.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            hiveAPIEnabled:
              description: HiveAPIEnabled is a boolean controlling whether or not
                the Hive operator will start up the v1alpha1 aggregated API server.
              type: boolean
            logLevel:
              description: LogLevel is the level of logging to use for the Hive controllers.
                Acceptable levels, from coarsest to finest, are panic, fatal, error,
                warn, info, debug, and trace. The default level is info.
              type: string
            maintenanceMode:
              description: MaintenanceMode can be set to true to disable the hive
                controllers in situations where we need to ensure nothing is running
                that will add or act upon finalizers on Hive types. This should rarely
                be needed. Sets replicas to 0 for the hive-controllers deployment
                to accomplish this.
              type: boolean
            managedDomains:
              description: 'ManagedDomains is the list of DNS domains that are managed
                by the Hive cluster When specifying ''managedDNS: true'' in a ClusterDeployment,
                the ClusterDeployment''s baseDomain should be a direct child of one
                of these domains, otherwise the ClusterDeployment creation will result
                in a validation error.'
              type: array
              items:
                description: ManageDNSConfig contains the domain being managed, and
                  the cloud-specific details for accessing/managing the domain.
                type: object
                required:
                - domains
                properties:
                  aws:
                    description: AWS contains AWS-specific settings for external DNS
                    type: object
                    properties:
                      credentialsSecretRef:
                        description: CredentialsSecretRef references a secret in the
                          TargetNamespace that will be used to authenticate with AWS
                          Route53. It will need permission to manage entries for the
                          domain listed in the parent ManageDNSConfig object. Secret
                          should have AWS keys named 'aws_access_key_id' and 'aws_secret_access_key'.
                        type: object
                        properties:
                          name:
                            description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                              TODO: Add other useful fields. apiVersion, kind, uid?'
                            type: string
                      region:
                        description: Region is the AWS region to use for route53 operations.
                          This defaults to us-east-1. For AWS China, use cn-northwest-1.
                        type: string
                  domains:
                    description: Domains is the list of domains that hive will be
                      managing entries for with the provided credentials.
                    type: array
                    items:
                      type: string
                  gcp:
                    description: GCP contains GCP-specific settings for external DNS
                    type: object
                    properties:
                      credentialsSecretRef:
                        description: CredentialsSecretRef references a secret in the
                          TargetNamespace that will be used to authenticate with GCP
                          DNS. It will need permission to manage entries in each of
                          the managed domains for this cluster. listed in the parent
                          ManageDNSConfig object. Secret should have a key named 'osServiceAccount.json'.
                          The credentials must specify the project to use.
                        type: object
                        properties:
                          name:
                            description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                              TODO: Add other useful fields. apiVersion, kind, uid?'
                            type: string
            syncSetReapplyInterval:
              description: SyncSetReapplyInterval is a string duration indicating
                how much time must pass before SyncSet resources will be reapplied.
                The default reapply interval is two hours.
              type: string
            targetNamespace:
              description: TargetNamespace is the namespace where the core Hive components
                should be run. Defaults to "hive". Will be created if it does not
                already exist. All resource references in HiveConfig can be assumed
                to be in the TargetNamespace.
              type: string
        status:
          description: HiveConfigStatus defines the observed state of Hive
          type: object
          properties:
            aggregatorClientCAHash:
              description: AggregatorClientCAHash keeps an md5 hash of the aggregator
                client CA configmap data from the openshift-config-managed namespace.
                When the configmap changes, admission is redeployed.
              type: string
            configApplied:
              description: ConfigApplied will be set by the hive operator to indicate
                whether or not the LastGenerationObserved was successfully reconciled.
              type: boolean
            observedGeneration:
              description: ObservedGeneration will record the most recently processed
                HiveConfig object's generation.
              type: integer
              format: int64
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_hiveconfigYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_hiveconfigYaml, nil
}

func configCrdsHive_v1_hiveconfigYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_hiveconfigYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_hiveconfig.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_machinepoolYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machinepools.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.name
    name: PoolName
    type: string
  - JSONPath: .spec.clusterDeploymentRef.name
    name: ClusterDeployment
    type: string
  - JSONPath: .spec.replicas
    name: Replicas
    type: integer
  group: hive.openshift.io
  names:
    kind: MachinePool
    plural: machinepools
  scope: Namespaced
  subresources:
    scale:
      specReplicasPath: .spec.replicas
      statusReplicasPath: .status.replicas
    status: {}
  validation:
    openAPIV3Schema:
      description: MachinePool is the Schema for the machinepools API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: MachinePoolSpec defines the desired state of MachinePool
          type: object
          required:
          - clusterDeploymentRef
          - name
          - platform
          properties:
            autoscaling:
              description: Autoscaling is the details for auto-scaling the machine
                pool. Replicas and autoscaling cannot be used together.
              type: object
              required:
              - maxReplicas
              - minReplicas
              properties:
                maxReplicas:
                  description: MaxReplicas is the maximum number of replicas for the
                    machine pool.
                  type: integer
                  format: int32
                minReplicas:
                  description: MinReplicas is the minimum number of replicas for the
                    machine pool.
                  type: integer
                  format: int32
            clusterDeploymentRef:
              description: ClusterDeploymentRef references the cluster deployment
                to which this machine pool belongs.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            labels:
              description: Map of label string keys and values that will be applied
                to the created MachineSet's MachineSpec. This list will overwrite
                any modifications made to Node labels on an ongoing basis.
              type: object
              additionalProperties:
                type: string
            name:
              description: Name is the name of the machine pool.
              type: string
            platform:
              description: Platform is configuration for machine pool specific to
                the platform.
              type: object
              properties:
                aws:
                  description: AWS is the configuration used when installing on AWS.
                  type: object
                  required:
                  - rootVolume
                  - type
                  properties:
                    rootVolume:
                      description: EC2RootVolume defines the storage for ec2 instance.
                      type: object
                      required:
                      - iops
                      - size
                      - type
                      properties:
                        iops:
                          description: IOPS defines the iops for the storage.
                          type: integer
                        size:
                          description: Size defines the size of the storage.
                          type: integer
                        type:
                          description: Type defines the type of the storage.
                          type: string
                    type:
                      description: InstanceType defines the ec2 instance type. eg.
                        m4-large
                      type: string
                    zones:
                      description: Zones is list of availability zones that can be
                        used.
                      type: array
                      items:
                        type: string
                azure:
                  description: Azure is the configuration used when installing on
                    Azure.
                  type: object
                  required:
                  - osDisk
                  - type
                  properties:
                    osDisk:
                      description: OSDisk defines the storage for instance.
                      type: object
                      required:
                      - diskSizeGB
                      properties:
                        diskSizeGB:
                          description: DiskSizeGB defines the size of disk in GB.
                          type: integer
                          format: int32
                    type:
                      description: InstanceType defines the azure instance type. eg.
                        Standard_DS_V2
                      type: string
                    zones:
                      description: Zones is list of availability zones that can be
                        used. eg. ["1", "2", "3"]
                      type: array
                      items:
                        type: string
                gcp:
                  description: GCP is the configuration used when installing on GCP.
                  type: object
                  required:
                  - type
                  properties:
                    type:
                      description: InstanceType defines the GCP instance type. eg.
                        n1-standard-4
                      type: string
                    zones:
                      description: Zones is list of availability zones that can be
                        used.
                      type: array
                      items:
                        type: string
                openstack:
                  description: OpenStack is the configuration used when installing
                    on OpenStack.
                  type: object
                  required:
                  - flavor
                  properties:
                    flavor:
                      description: Flavor defines the OpenStack Nova flavor. eg. m1.large
                        The json key here differs from the installer which uses both
                        "computeFlavor" and type "type" depending on which type you're
                        looking at, and the resulting field on the MachineSet is "flavor".
                        We are opting to stay consistent with the end result.
                      type: string
                    rootVolume:
                      description: RootVolume defines the root volume for instances
                        in the machine pool. The instances use ephemeral disks if
                        not set.
                      type: object
                      required:
                      - size
                      - type
                      properties:
                        size:
                          description: Size defines the size of the volume in gibibytes
                            (GiB). Required
                          type: integer
                        type:
                          description: Type defines the type of the volume. Required
                          type: string
            replicas:
              description: Replicas is the count of machines for this machine pool.
                Replicas and autoscaling cannot be used together. Default is 1, if
                autoscaling is not used.
              type: integer
              format: int64
            taints:
              description: List of taints that will be applied to the created MachineSet's
                MachineSpec. This list will overwrite any modifications made to Node
                taints on an ongoing basis.
              type: array
              items:
                description: The node this Taint is attached to has the "effect" on
                  any pod that does not tolerate the Taint.
                type: object
                required:
                - effect
                - key
                properties:
                  effect:
                    description: Required. The effect of the taint on pods that do
                      not tolerate the taint. Valid effects are NoSchedule, PreferNoSchedule
                      and NoExecute.
                    type: string
                  key:
                    description: Required. The taint key to be applied to a node.
                    type: string
                  timeAdded:
                    description: TimeAdded represents the time at which the taint
                      was added. It is only written for NoExecute taints.
                    type: string
                    format: date-time
                  value:
                    description: The taint value corresponding to the taint key.
                    type: string
        status:
          description: MachinePoolStatus defines the observed state of MachinePool
          type: object
          properties:
            conditions:
              description: Conditions includes more detailed status for the cluster
                deployment
              type: array
              items:
                description: MachinePoolCondition contains details for the current
                  condition of a machine pool
                type: object
                required:
                - status
                - type
                properties:
                  lastProbeTime:
                    description: LastProbeTime is the last time we probed the condition.
                    type: string
                    format: date-time
                  lastTransitionTime:
                    description: LastTransitionTime is the last time the condition
                      transitioned from one status to another.
                    type: string
                    format: date-time
                  message:
                    description: Message is a human-readable message indicating details
                      about last transition.
                    type: string
                  reason:
                    description: Reason is a unique, one-word, CamelCase reason for
                      the condition's last transition.
                    type: string
                  status:
                    description: Status is the status of the condition.
                    type: string
                  type:
                    description: Type is the type of the condition.
                    type: string
            machineSets:
              description: MachineSets is the status of the machine sets for the machine
                pool on the remote cluster.
              type: array
              items:
                description: MachineSetStatus is the status of a machineset in the
                  remote cluster.
                type: object
                required:
                - maxReplicas
                - minReplicas
                - name
                - replicas
                properties:
                  maxReplicas:
                    description: MaxReplicas is the maximum number of replicas for
                      the machine set.
                    type: integer
                    format: int32
                  minReplicas:
                    description: MinReplicas is the minimum number of replicas for
                      the machine set.
                    type: integer
                    format: int32
                  name:
                    description: Name is the name of the machine set.
                    type: string
                  replicas:
                    description: Replicas is the current number of replicas for the
                      machine set.
                    type: integer
                    format: int32
            replicas:
              description: Replicas is the current number of replicas for the machine
                pool.
              type: integer
              format: int32
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_machinepoolYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_machinepoolYaml, nil
}

func configCrdsHive_v1_machinepoolYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_machinepoolYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_machinepool.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_machinepoolnameleaseYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machinepoolnameleases.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .metadata.labels.hive\.openshift\.io/machine-pool-name
    name: MachinePool
    type: string
  - JSONPath: .metadata.labels.hive\.openshift\.io/cluster-deployment-name
    name: Cluster
    type: string
  - JSONPath: .metadata.creationTimestamp
    name: Age
    type: date
  group: hive.openshift.io
  names:
    kind: MachinePoolNameLease
    plural: machinepoolnameleases
  scope: Namespaced
  validation:
    openAPIV3Schema:
      description: MachinePoolNameLease is the Schema for the MachinePoolNameLeases
        API. This resource is mostly empty as we're primarily relying on the name
        to determine if a lease is available. Note that not all cloud providers require
        the use of a lease for naming, at present this is only required for GCP where
        we're extremely restricted on name lengths.
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: MachinePoolNameLeaseSpec is a minimal resource for obtaining
            unique machine pool names of a limited length.
          type: object
        status:
          description: MachinePoolNameLeaseStatus defines the observed state of MachinePoolNameLease.
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_machinepoolnameleaseYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_machinepoolnameleaseYaml, nil
}

func configCrdsHive_v1_machinepoolnameleaseYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_machinepoolnameleaseYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_machinepoolnamelease.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_selectorsyncidentityproviderYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: selectorsyncidentityproviders.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: SelectorSyncIdentityProvider
    plural: selectorsyncidentityproviders
  scope: Cluster
  validation:
    openAPIV3Schema:
      description: SelectorSyncIdentityProvider is the Schema for the SelectorSyncSet
        API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: SelectorSyncIdentityProviderSpec defines the SyncIdentityProviderCommonSpec
            to sync to ClusterDeploymentSelector indicating which clusters the SelectorSyncIdentityProvider
            applies to in any namespace.
          type: object
          required:
          - identityProviders
          properties:
            clusterDeploymentSelector:
              description: ClusterDeploymentSelector is a LabelSelector indicating
                which clusters the SelectorIdentityProvider applies to in any namespace.
              type: object
              properties:
                matchExpressions:
                  description: matchExpressions is a list of label selector requirements.
                    The requirements are ANDed.
                  type: array
                  items:
                    description: A label selector requirement is a selector that contains
                      values, a key, and an operator that relates the key and values.
                    type: object
                    required:
                    - key
                    - operator
                    properties:
                      key:
                        description: key is the label key that the selector applies
                          to.
                        type: string
                      operator:
                        description: operator represents a key's relationship to a
                          set of values. Valid operators are In, NotIn, Exists and
                          DoesNotExist.
                        type: string
                      values:
                        description: values is an array of string values. If the operator
                          is In or NotIn, the values array must be non-empty. If the
                          operator is Exists or DoesNotExist, the values array must
                          be empty. This array is replaced during a strategic merge
                          patch.
                        type: array
                        items:
                          type: string
                matchLabels:
                  description: matchLabels is a map of {key,value} pairs. A single
                    {key,value} in the matchLabels map is equivalent to an element
                    of matchExpressions, whose key field is "key", the operator is
                    "In", and the values array contains only "value". The requirements
                    are ANDed.
                  type: object
                  additionalProperties:
                    type: string
            identityProviders:
              description: IdentityProviders is an ordered list of ways for a user
                to identify themselves
              type: array
              items:
                description: IdentityProvider provides identities for users authenticating
                  using credentials
                type: object
                properties:
                  basicAuth:
                    description: basicAuth contains configuration options for the
                      BasicAuth IdP
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      tlsClientCert:
                        description: tlsClientCert is an optional reference to a secret
                          by name that contains the PEM-encoded TLS client certificate
                          to present when connecting to the server. The key "tls.crt"
                          is used to locate the data. If specified and the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified certificate data is not valid,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      tlsClientKey:
                        description: tlsClientKey is an optional reference to a secret
                          by name that contains the PEM-encoded TLS private key for
                          the client certificate referenced in tlsClientCert. The
                          key "tls.key" is used to locate the data. If specified and
                          the secret or expected key is not found, the identity provider
                          is not honored. If the specified certificate data is not
                          valid, the identity provider is not honored. The namespace
                          for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the remote URL to connect to
                        type: string
                  github:
                    description: github enables user authentication using GitHub credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          This can only be configured when hostname is set to a non-empty
                          value. The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      hostname:
                        description: hostname is the optional domain (e.g. "mycompany.com")
                          for use with a hosted instance of GitHub Enterprise. It
                          must match the GitHub Enterprise settings value configured
                          at /setup/settings#hostname.
                        type: string
                      organizations:
                        description: organizations optionally restricts which organizations
                          are allowed to log in
                        type: array
                        items:
                          type: string
                      teams:
                        description: teams optionally restricts which teams are allowed
                          to log in. Format is <org>/<team>.
                        type: array
                        items:
                          type: string
                  gitlab:
                    description: gitlab enables user authentication using GitLab credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the oauth server base URL
                        type: string
                  google:
                    description: google enables user authentication using Google credentials
                    type: object
                    properties:
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      hostedDomain:
                        description: hostedDomain is the optional Google App domain
                          (e.g. "mycompany.com") to restrict logins to
                        type: string
                  htpasswd:
                    description: htpasswd enables user authentication using an HTPasswd
                      file to validate credentials
                    type: object
                    properties:
                      fileData:
                        description: fileData is a required reference to a secret
                          by name containing the data to use as the htpasswd file.
                          The key "htpasswd" is used to locate the data. If the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified htpasswd data is not valid, the
                          identity provider is not honored. The namespace for this
                          secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                  keystone:
                    description: keystone enables user authentication using keystone
                      password credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      domainName:
                        description: domainName is required for keystone v3
                        type: string
                      tlsClientCert:
                        description: tlsClientCert is an optional reference to a secret
                          by name that contains the PEM-encoded TLS client certificate
                          to present when connecting to the server. The key "tls.crt"
                          is used to locate the data. If specified and the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified certificate data is not valid,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      tlsClientKey:
                        description: tlsClientKey is an optional reference to a secret
                          by name that contains the PEM-encoded TLS private key for
                          the client certificate referenced in tlsClientCert. The
                          key "tls.key" is used to locate the data. If specified and
                          the secret or expected key is not found, the identity provider
                          is not honored. If the specified certificate data is not
                          valid, the identity provider is not honored. The namespace
                          for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the remote URL to connect to
                        type: string
                  ldap:
                    description: ldap enables user authentication using LDAP credentials
                    type: object
                    properties:
                      attributes:
                        description: attributes maps LDAP attributes to identities
                        type: object
                        properties:
                          email:
                            description: email is the list of attributes whose values
                              should be used as the email address. Optional. If unspecified,
                              no email is set for the identity
                            type: array
                            items:
                              type: string
                          id:
                            description: id is the list of attributes whose values
                              should be used as the user ID. Required. First non-empty
                              attribute is used. At least one attribute is required.
                              If none of the listed attribute have a value, authentication
                              fails. LDAP standard identity attribute is "dn"
                            type: array
                            items:
                              type: string
                          name:
                            description: name is the list of attributes whose values
                              should be used as the display name. Optional. If unspecified,
                              no display name is set for the identity LDAP standard
                              display name attribute is "cn"
                            type: array
                            items:
                              type: string
                          preferredUsername:
                            description: preferredUsername is the list of attributes
                              whose values should be used as the preferred username.
                              LDAP standard login attribute is "uid"
                            type: array
                            items:
                              type: string
                      bindDN:
                        description: bindDN is an optional DN to bind with during
                          the search phase.
                        type: string
                      bindPassword:
                        description: bindPassword is an optional reference to a secret
                          by name containing a password to bind with during the search
                          phase. The key "bindPassword" is used to locate the data.
                          If specified and the secret or expected key is not found,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      insecure:
                        description: 'insecure, if true, indicates the connection
                          should not use TLS WARNING: Should not be set to ` + "`" + `true` + "`" + `
                          with the URL scheme "ldaps://" as "ldaps://" URLs always          attempt
                          to connect using TLS, even when ` + "`" + `insecure` + "`" + ` is set to ` + "`" + `true` + "`" + `
                          When ` + "`" + `true` + "`" + `, "ldap://" URLS connect insecurely. When ` + "`" + `false` + "`" + `,
                          "ldap://" URLs are upgraded to a TLS connection using StartTLS
                          as specified in https://tools.ietf.org/html/rfc2830.'
                        type: boolean
                      url:
                        description: 'url is an RFC 2255 URL which specifies the LDAP
                          search parameters to use. The syntax of the URL is: ldap://host:port/basedn?attribute?scope?filter'
                        type: string
                  mappingMethod:
                    description: mappingMethod determines how identities from this
                      provider are mapped to users Defaults to "claim"
                    type: string
                  name:
                    description: 'name is used to qualify the identities returned
                      by this provider. - It MUST be unique and not shared by any
                      other identity provider used - It MUST be a valid path segment:
                      name cannot equal "." or ".." or contain "/" or "%" or ":"   Ref:
                      https://godoc.org/github.com/openshift/origin/pkg/user/apis/user/validation#ValidateIdentityProviderName'
                    type: string
                  openID:
                    description: openID enables user authentication using OpenID credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      claims:
                        description: claims mappings
                        type: object
                        properties:
                          email:
                            description: email is the list of claims whose values
                              should be used as the email address. Optional. If unspecified,
                              no email is set for the identity
                            type: array
                            items:
                              type: string
                          name:
                            description: name is the list of claims whose values should
                              be used as the display name. Optional. If unspecified,
                              no display name is set for the identity
                            type: array
                            items:
                              type: string
                          preferredUsername:
                            description: preferredUsername is the list of claims whose
                              values should be used as the preferred username. If
                              unspecified, the preferred username is determined from
                              the value of the sub claim
                            type: array
                            items:
                              type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      extraAuthorizeParameters:
                        description: extraAuthorizeParameters are any custom parameters
                          to add to the authorize request.
                        type: object
                        additionalProperties:
                          type: string
                      extraScopes:
                        description: extraScopes are any scopes to request in addition
                          to the standard "openid" scope.
                        type: array
                        items:
                          type: string
                      issuer:
                        description: issuer is the URL that the OpenID Provider asserts
                          as its Issuer Identifier. It must use the https scheme with
                          no query or fragment component.
                        type: string
                  requestHeader:
                    description: requestHeader enables user authentication using request
                      header credentials
                    type: object
                    properties:
                      ca:
                        description: ca is a required reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. Specifically, it allows verification
                          of incoming requests to prevent header spoofing. The key
                          "ca.crt" is used to locate the data. If the config map or
                          expected key is not found, the identity provider is not
                          honored. If the specified ca data is not valid, the identity
                          provider is not honored. The namespace for this config map
                          is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      challengeURL:
                        description: challengeURL is a URL to redirect unauthenticated
                          /authorize requests to Unauthenticated requests from OAuth
                          clients which expect WWW-Authenticate challenges will be
                          redirected here. ${url} is replaced with the current URL,
                          escaped to be safe in a query parameter   https://www.example.com/sso-login?then=${url}
                          ${query} is replaced with the current query string   https://www.example.com/auth-proxy/oauth/authorize?${query}
                          Required when challenge is set to true.
                        type: string
                      clientCommonNames:
                        description: clientCommonNames is an optional list of common
                          names to require a match from. If empty, any client certificate
                          validated against the clientCA bundle is considered authoritative.
                        type: array
                        items:
                          type: string
                      emailHeaders:
                        description: emailHeaders is the set of headers to check for
                          the email address
                        type: array
                        items:
                          type: string
                      headers:
                        description: headers is the set of headers to check for identity
                          information
                        type: array
                        items:
                          type: string
                      loginURL:
                        description: loginURL is a URL to redirect unauthenticated
                          /authorize requests to Unauthenticated requests from OAuth
                          clients which expect interactive logins will be redirected
                          here ${url} is replaced with the current URL, escaped to
                          be safe in a query parameter   https://www.example.com/sso-login?then=${url}
                          ${query} is replaced with the current query string   https://www.example.com/auth-proxy/oauth/authorize?${query}
                          Required when login is set to true.
                        type: string
                      nameHeaders:
                        description: nameHeaders is the set of headers to check for
                          the display name
                        type: array
                        items:
                          type: string
                      preferredUsernameHeaders:
                        description: preferredUsernameHeaders is the set of headers
                          to check for the preferred username
                        type: array
                        items:
                          type: string
                  type:
                    description: type identifies the identity provider type for this
                      entry.
                    type: string
        status:
          description: IdentityProviderStatus defines the observed state of SyncSet
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_selectorsyncidentityproviderYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_selectorsyncidentityproviderYaml, nil
}

func configCrdsHive_v1_selectorsyncidentityproviderYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_selectorsyncidentityproviderYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_selectorsyncidentityprovider.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_selectorsyncsetYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: selectorsyncsets.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: SelectorSyncSet
    plural: selectorsyncsets
    shortNames:
    - sss
  scope: Cluster
  validation:
    openAPIV3Schema:
      description: SelectorSyncSet is the Schema for the SelectorSyncSet API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: SelectorSyncSetSpec defines the SyncSetCommonSpec resources
            and patches to sync along with a ClusterDeploymentSelector indicating
            which clusters the SelectorSyncSet applies to in any namespace.
          type: object
          properties:
            clusterDeploymentSelector:
              description: ClusterDeploymentSelector is a LabelSelector indicating
                which clusters the SelectorSyncSet applies to in any namespace.
              type: object
              properties:
                matchExpressions:
                  description: matchExpressions is a list of label selector requirements.
                    The requirements are ANDed.
                  type: array
                  items:
                    description: A label selector requirement is a selector that contains
                      values, a key, and an operator that relates the key and values.
                    type: object
                    required:
                    - key
                    - operator
                    properties:
                      key:
                        description: key is the label key that the selector applies
                          to.
                        type: string
                      operator:
                        description: operator represents a key's relationship to a
                          set of values. Valid operators are In, NotIn, Exists and
                          DoesNotExist.
                        type: string
                      values:
                        description: values is an array of string values. If the operator
                          is In or NotIn, the values array must be non-empty. If the
                          operator is Exists or DoesNotExist, the values array must
                          be empty. This array is replaced during a strategic merge
                          patch.
                        type: array
                        items:
                          type: string
                matchLabels:
                  description: matchLabels is a map of {key,value} pairs. A single
                    {key,value} in the matchLabels map is equivalent to an element
                    of matchExpressions, whose key field is "key", the operator is
                    "In", and the values array contains only "value". The requirements
                    are ANDed.
                  type: object
                  additionalProperties:
                    type: string
            patches:
              description: Patches is the list of patches to apply.
              type: array
              items:
                description: SyncObjectPatch represents a patch to be applied to a
                  specific object
                type: object
                required:
                - apiVersion
                - kind
                - name
                - patch
                properties:
                  apiVersion:
                    description: APIVersion is the Group and Version of the object
                      to be patched.
                    type: string
                  kind:
                    description: Kind is the Kind of the object to be patched.
                    type: string
                  name:
                    description: Name is the name of the object to be patched.
                    type: string
                  namespace:
                    description: Namespace is the Namespace in which the object to
                      patch exists. Defaults to the SyncSet's Namespace.
                    type: string
                  patch:
                    description: Patch is the patch to apply.
                    type: string
                  patchType:
                    description: PatchType indicates the PatchType as "strategic"
                      (default), "json", or "merge".
                    type: string
            resourceApplyMode:
              description: ResourceApplyMode indicates if the Resource apply mode
                is "Upsert" (default) or "Sync". ApplyMode "Upsert" indicates create
                and update. ApplyMode "Sync" indicates create, update and delete.
              type: string
            resources:
              description: Resources is the list of objects to sync from RawExtension
                definitions.
              type: array
              items:
                type: object
            secretMappings:
              description: Secrets is the list of secrets to sync along with their
                respective destinations.
              type: array
              items:
                description: SecretMapping defines a source and destination for a
                  secret to be synced by a SyncSet
                type: object
                required:
                - sourceRef
                - targetRef
                properties:
                  sourceRef:
                    description: SourceRef specifies the name and namespace of a secret
                      on the management cluster
                    type: object
                    required:
                    - name
                    properties:
                      name:
                        description: Name is the name of the secret
                        type: string
                      namespace:
                        description: Namespace is the namespace where the secret lives.
                          If not present for the source secret reference, it is assumed
                          to be the same namespace as the syncset with the reference.
                        type: string
                  targetRef:
                    description: TargetRef specifies the target name and namespace
                      of the secret on the target cluster
                    type: object
                    required:
                    - name
                    properties:
                      name:
                        description: Name is the name of the secret
                        type: string
                      namespace:
                        description: Namespace is the namespace where the secret lives.
                          If not present for the source secret reference, it is assumed
                          to be the same namespace as the syncset with the reference.
                        type: string
        status:
          description: SelectorSyncSetStatus defines the observed state of a SelectorSyncSet
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_selectorsyncsetYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_selectorsyncsetYaml, nil
}

func configCrdsHive_v1_selectorsyncsetYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_selectorsyncsetYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_selectorsyncset.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_syncidentityproviderYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: syncidentityproviders.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: SyncIdentityProvider
    plural: syncidentityproviders
  scope: Namespaced
  validation:
    openAPIV3Schema:
      description: SyncIdentityProvider is the Schema for the SyncIdentityProvider
        API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: SyncIdentityProviderSpec defines the SyncIdentityProviderCommonSpec
            identity providers to sync along with ClusterDeploymentRefs indicating
            which clusters the SyncIdentityProvider applies to in the SyncIdentityProvider's
            namespace.
          type: object
          required:
          - clusterDeploymentRefs
          - identityProviders
          properties:
            clusterDeploymentRefs:
              description: ClusterDeploymentRefs is the list of LocalObjectReference
                indicating which clusters the SyncSet applies to in the SyncSet's
                namespace.
              type: array
              items:
                description: LocalObjectReference contains enough information to let
                  you locate the referenced object inside the same namespace.
                type: object
                properties:
                  name:
                    description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                      TODO: Add other useful fields. apiVersion, kind, uid?'
                    type: string
            identityProviders:
              description: IdentityProviders is an ordered list of ways for a user
                to identify themselves
              type: array
              items:
                description: IdentityProvider provides identities for users authenticating
                  using credentials
                type: object
                properties:
                  basicAuth:
                    description: basicAuth contains configuration options for the
                      BasicAuth IdP
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      tlsClientCert:
                        description: tlsClientCert is an optional reference to a secret
                          by name that contains the PEM-encoded TLS client certificate
                          to present when connecting to the server. The key "tls.crt"
                          is used to locate the data. If specified and the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified certificate data is not valid,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      tlsClientKey:
                        description: tlsClientKey is an optional reference to a secret
                          by name that contains the PEM-encoded TLS private key for
                          the client certificate referenced in tlsClientCert. The
                          key "tls.key" is used to locate the data. If specified and
                          the secret or expected key is not found, the identity provider
                          is not honored. If the specified certificate data is not
                          valid, the identity provider is not honored. The namespace
                          for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the remote URL to connect to
                        type: string
                  github:
                    description: github enables user authentication using GitHub credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          This can only be configured when hostname is set to a non-empty
                          value. The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      hostname:
                        description: hostname is the optional domain (e.g. "mycompany.com")
                          for use with a hosted instance of GitHub Enterprise. It
                          must match the GitHub Enterprise settings value configured
                          at /setup/settings#hostname.
                        type: string
                      organizations:
                        description: organizations optionally restricts which organizations
                          are allowed to log in
                        type: array
                        items:
                          type: string
                      teams:
                        description: teams optionally restricts which teams are allowed
                          to log in. Format is <org>/<team>.
                        type: array
                        items:
                          type: string
                  gitlab:
                    description: gitlab enables user authentication using GitLab credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the oauth server base URL
                        type: string
                  google:
                    description: google enables user authentication using Google credentials
                    type: object
                    properties:
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      hostedDomain:
                        description: hostedDomain is the optional Google App domain
                          (e.g. "mycompany.com") to restrict logins to
                        type: string
                  htpasswd:
                    description: htpasswd enables user authentication using an HTPasswd
                      file to validate credentials
                    type: object
                    properties:
                      fileData:
                        description: fileData is a required reference to a secret
                          by name containing the data to use as the htpasswd file.
                          The key "htpasswd" is used to locate the data. If the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified htpasswd data is not valid, the
                          identity provider is not honored. The namespace for this
                          secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                  keystone:
                    description: keystone enables user authentication using keystone
                      password credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      domainName:
                        description: domainName is required for keystone v3
                        type: string
                      tlsClientCert:
                        description: tlsClientCert is an optional reference to a secret
                          by name that contains the PEM-encoded TLS client certificate
                          to present when connecting to the server. The key "tls.crt"
                          is used to locate the data. If specified and the secret
                          or expected key is not found, the identity provider is not
                          honored. If the specified certificate data is not valid,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      tlsClientKey:
                        description: tlsClientKey is an optional reference to a secret
                          by name that contains the PEM-encoded TLS private key for
                          the client certificate referenced in tlsClientCert. The
                          key "tls.key" is used to locate the data. If specified and
                          the secret or expected key is not found, the identity provider
                          is not honored. If the specified certificate data is not
                          valid, the identity provider is not honored. The namespace
                          for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      url:
                        description: url is the remote URL to connect to
                        type: string
                  ldap:
                    description: ldap enables user authentication using LDAP credentials
                    type: object
                    properties:
                      attributes:
                        description: attributes maps LDAP attributes to identities
                        type: object
                        properties:
                          email:
                            description: email is the list of attributes whose values
                              should be used as the email address. Optional. If unspecified,
                              no email is set for the identity
                            type: array
                            items:
                              type: string
                          id:
                            description: id is the list of attributes whose values
                              should be used as the user ID. Required. First non-empty
                              attribute is used. At least one attribute is required.
                              If none of the listed attribute have a value, authentication
                              fails. LDAP standard identity attribute is "dn"
                            type: array
                            items:
                              type: string
                          name:
                            description: name is the list of attributes whose values
                              should be used as the display name. Optional. If unspecified,
                              no display name is set for the identity LDAP standard
                              display name attribute is "cn"
                            type: array
                            items:
                              type: string
                          preferredUsername:
                            description: preferredUsername is the list of attributes
                              whose values should be used as the preferred username.
                              LDAP standard login attribute is "uid"
                            type: array
                            items:
                              type: string
                      bindDN:
                        description: bindDN is an optional DN to bind with during
                          the search phase.
                        type: string
                      bindPassword:
                        description: bindPassword is an optional reference to a secret
                          by name containing a password to bind with during the search
                          phase. The key "bindPassword" is used to locate the data.
                          If specified and the secret or expected key is not found,
                          the identity provider is not honored. The namespace for
                          this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      insecure:
                        description: 'insecure, if true, indicates the connection
                          should not use TLS WARNING: Should not be set to ` + "`" + `true` + "`" + `
                          with the URL scheme "ldaps://" as "ldaps://" URLs always          attempt
                          to connect using TLS, even when ` + "`" + `insecure` + "`" + ` is set to ` + "`" + `true` + "`" + `
                          When ` + "`" + `true` + "`" + `, "ldap://" URLS connect insecurely. When ` + "`" + `false` + "`" + `,
                          "ldap://" URLs are upgraded to a TLS connection using StartTLS
                          as specified in https://tools.ietf.org/html/rfc2830.'
                        type: boolean
                      url:
                        description: 'url is an RFC 2255 URL which specifies the LDAP
                          search parameters to use. The syntax of the URL is: ldap://host:port/basedn?attribute?scope?filter'
                        type: string
                  mappingMethod:
                    description: mappingMethod determines how identities from this
                      provider are mapped to users Defaults to "claim"
                    type: string
                  name:
                    description: 'name is used to qualify the identities returned
                      by this provider. - It MUST be unique and not shared by any
                      other identity provider used - It MUST be a valid path segment:
                      name cannot equal "." or ".." or contain "/" or "%" or ":"   Ref:
                      https://godoc.org/github.com/openshift/origin/pkg/user/apis/user/validation#ValidateIdentityProviderName'
                    type: string
                  openID:
                    description: openID enables user authentication using OpenID credentials
                    type: object
                    properties:
                      ca:
                        description: ca is an optional reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. The key "ca.crt" is used to locate
                          the data. If specified and the config map or expected key
                          is not found, the identity provider is not honored. If the
                          specified ca data is not valid, the identity provider is
                          not honored. If empty, the default system roots are used.
                          The namespace for this config map is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      claims:
                        description: claims mappings
                        type: object
                        properties:
                          email:
                            description: email is the list of claims whose values
                              should be used as the email address. Optional. If unspecified,
                              no email is set for the identity
                            type: array
                            items:
                              type: string
                          name:
                            description: name is the list of claims whose values should
                              be used as the display name. Optional. If unspecified,
                              no display name is set for the identity
                            type: array
                            items:
                              type: string
                          preferredUsername:
                            description: preferredUsername is the list of claims whose
                              values should be used as the preferred username. If
                              unspecified, the preferred username is determined from
                              the value of the sub claim
                            type: array
                            items:
                              type: string
                      clientID:
                        description: clientID is the oauth client ID
                        type: string
                      clientSecret:
                        description: clientSecret is a required reference to the secret
                          by name containing the oauth client secret. The key "clientSecret"
                          is used to locate the data. If the secret or expected key
                          is not found, the identity provider is not honored. The
                          namespace for this secret is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              secret
                            type: string
                      extraAuthorizeParameters:
                        description: extraAuthorizeParameters are any custom parameters
                          to add to the authorize request.
                        type: object
                        additionalProperties:
                          type: string
                      extraScopes:
                        description: extraScopes are any scopes to request in addition
                          to the standard "openid" scope.
                        type: array
                        items:
                          type: string
                      issuer:
                        description: issuer is the URL that the OpenID Provider asserts
                          as its Issuer Identifier. It must use the https scheme with
                          no query or fragment component.
                        type: string
                  requestHeader:
                    description: requestHeader enables user authentication using request
                      header credentials
                    type: object
                    properties:
                      ca:
                        description: ca is a required reference to a config map by
                          name containing the PEM-encoded CA bundle. It is used as
                          a trust anchor to validate the TLS certificate presented
                          by the remote server. Specifically, it allows verification
                          of incoming requests to prevent header spoofing. The key
                          "ca.crt" is used to locate the data. If the config map or
                          expected key is not found, the identity provider is not
                          honored. If the specified ca data is not valid, the identity
                          provider is not honored. The namespace for this config map
                          is openshift-config.
                        type: object
                        required:
                        - name
                        properties:
                          name:
                            description: name is the metadata.name of the referenced
                              config map
                            type: string
                      challengeURL:
                        description: challengeURL is a URL to redirect unauthenticated
                          /authorize requests to Unauthenticated requests from OAuth
                          clients which expect WWW-Authenticate challenges will be
                          redirected here. ${url} is replaced with the current URL,
                          escaped to be safe in a query parameter   https://www.example.com/sso-login?then=${url}
                          ${query} is replaced with the current query string   https://www.example.com/auth-proxy/oauth/authorize?${query}
                          Required when challenge is set to true.
                        type: string
                      clientCommonNames:
                        description: clientCommonNames is an optional list of common
                          names to require a match from. If empty, any client certificate
                          validated against the clientCA bundle is considered authoritative.
                        type: array
                        items:
                          type: string
                      emailHeaders:
                        description: emailHeaders is the set of headers to check for
                          the email address
                        type: array
                        items:
                          type: string
                      headers:
                        description: headers is the set of headers to check for identity
                          information
                        type: array
                        items:
                          type: string
                      loginURL:
                        description: loginURL is a URL to redirect unauthenticated
                          /authorize requests to Unauthenticated requests from OAuth
                          clients which expect interactive logins will be redirected
                          here ${url} is replaced with the current URL, escaped to
                          be safe in a query parameter   https://www.example.com/sso-login?then=${url}
                          ${query} is replaced with the current query string   https://www.example.com/auth-proxy/oauth/authorize?${query}
                          Required when login is set to true.
                        type: string
                      nameHeaders:
                        description: nameHeaders is the set of headers to check for
                          the display name
                        type: array
                        items:
                          type: string
                      preferredUsernameHeaders:
                        description: preferredUsernameHeaders is the set of headers
                          to check for the preferred username
                        type: array
                        items:
                          type: string
                  type:
                    description: type identifies the identity provider type for this
                      entry.
                    type: string
        status:
          description: IdentityProviderStatus defines the observed state of SyncSet
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_syncidentityproviderYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_syncidentityproviderYaml, nil
}

func configCrdsHive_v1_syncidentityproviderYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_syncidentityproviderYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_syncidentityprovider.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_syncsetYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: syncsets.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: SyncSet
    plural: syncsets
    shortNames:
    - ss
  scope: Namespaced
  validation:
    openAPIV3Schema:
      description: SyncSet is the Schema for the SyncSet API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: SyncSetSpec defines the SyncSetCommonSpec resources and patches
            to sync along with ClusterDeploymentRefs indicating which clusters the
            SyncSet applies to in the SyncSet's namespace.
          type: object
          required:
          - clusterDeploymentRefs
          properties:
            clusterDeploymentRefs:
              description: ClusterDeploymentRefs is the list of LocalObjectReference
                indicating which clusters the SyncSet applies to in the SyncSet's
                namespace.
              type: array
              items:
                description: LocalObjectReference contains enough information to let
                  you locate the referenced object inside the same namespace.
                type: object
                properties:
                  name:
                    description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                      TODO: Add other useful fields. apiVersion, kind, uid?'
                    type: string
            patches:
              description: Patches is the list of patches to apply.
              type: array
              items:
                description: SyncObjectPatch represents a patch to be applied to a
                  specific object
                type: object
                required:
                - apiVersion
                - kind
                - name
                - patch
                properties:
                  apiVersion:
                    description: APIVersion is the Group and Version of the object
                      to be patched.
                    type: string
                  kind:
                    description: Kind is the Kind of the object to be patched.
                    type: string
                  name:
                    description: Name is the name of the object to be patched.
                    type: string
                  namespace:
                    description: Namespace is the Namespace in which the object to
                      patch exists. Defaults to the SyncSet's Namespace.
                    type: string
                  patch:
                    description: Patch is the patch to apply.
                    type: string
                  patchType:
                    description: PatchType indicates the PatchType as "strategic"
                      (default), "json", or "merge".
                    type: string
            resourceApplyMode:
              description: ResourceApplyMode indicates if the Resource apply mode
                is "Upsert" (default) or "Sync". ApplyMode "Upsert" indicates create
                and update. ApplyMode "Sync" indicates create, update and delete.
              type: string
            resources:
              description: Resources is the list of objects to sync from RawExtension
                definitions.
              type: array
              items:
                type: object
            secretMappings:
              description: Secrets is the list of secrets to sync along with their
                respective destinations.
              type: array
              items:
                description: SecretMapping defines a source and destination for a
                  secret to be synced by a SyncSet
                type: object
                required:
                - sourceRef
                - targetRef
                properties:
                  sourceRef:
                    description: SourceRef specifies the name and namespace of a secret
                      on the management cluster
                    type: object
                    required:
                    - name
                    properties:
                      name:
                        description: Name is the name of the secret
                        type: string
                      namespace:
                        description: Namespace is the namespace where the secret lives.
                          If not present for the source secret reference, it is assumed
                          to be the same namespace as the syncset with the reference.
                        type: string
                  targetRef:
                    description: TargetRef specifies the target name and namespace
                      of the secret on the target cluster
                    type: object
                    required:
                    - name
                    properties:
                      name:
                        description: Name is the name of the secret
                        type: string
                      namespace:
                        description: Namespace is the namespace where the secret lives.
                          If not present for the source secret reference, it is assumed
                          to be the same namespace as the syncset with the reference.
                        type: string
        status:
          description: SyncSetStatus defines the observed state of a SyncSet
          type: object
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_syncsetYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_syncsetYaml, nil
}

func configCrdsHive_v1_syncsetYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_syncsetYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_syncset.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _configCrdsHive_v1_syncsetinstanceYaml = []byte(`apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: syncsetinstances.hive.openshift.io
spec:
  additionalPrinterColumns:
  - JSONPath: .status.applied
    name: Applied
    type: boolean
  group: hive.openshift.io
  names:
    kind: SyncSetInstance
    plural: syncsetinstances
    shortNames:
    - ssi
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      description: SyncSetInstance is the Schema for the syncsetinstances API
      type: object
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          description: SyncSetInstanceSpec defines the desired state of SyncSetInstance
          type: object
          required:
          - clusterDeploymentRef
          properties:
            clusterDeploymentRef:
              description: ClusterDeployment is a reference to to the clusterdeployment
                for this syncsetinstance.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
            resourceApplyMode:
              description: ResourceApplyMode indicates if the resource apply mode
                is "Upsert" (default) or "Sync". ApplyMode "Upsert" indicates create
                and update. ApplyMode "Sync" indicates create, update and delete.
              type: string
            selectorSyncSetRef:
              description: SelectorSyncSetRef is a reference to the selectorsyncset
                for this syncsetinstance.
              type: object
              required:
              - name
              properties:
                name:
                  description: Name is the name of the SelectorSyncSet
                  type: string
            syncSetHash:
              description: SyncSetHash is a hash of the contents of the syncset or
                selectorsyncset spec. Its purpose is to cause a syncset instance update
                whenever there's a change in its source.
              type: string
            syncSetRef:
              description: SyncSet is a reference to the syncset for this syncsetinstance.
              type: object
              properties:
                name:
                  description: 'Name of the referent. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
                    TODO: Add other useful fields. apiVersion, kind, uid?'
                  type: string
        status:
          description: SyncSetInstanceStatus defines the observed state of SyncSetInstance
          type: object
          properties:
            applied:
              description: Applied will be true if all resources, patches, or secrets
                have successfully been applied on last attempt.
              type: boolean
            conditions:
              description: Conditions is the list of SyncConditions used to indicate
                UnknownObject when a resource type cannot be determined from a SyncSet
                resource.
              type: array
              items:
                description: SyncCondition is a condition in a SyncStatus
                type: object
                required:
                - status
                - type
                properties:
                  lastProbeTime:
                    description: LastProbeTime is the last time we probed the condition.
                    type: string
                    format: date-time
                  lastTransitionTime:
                    description: LastTransitionTime is the last time the condition
                      transitioned from one status to another.
                    type: string
                    format: date-time
                  message:
                    description: Message is a human-readable message indicating details
                      about last transition.
                    type: string
                  reason:
                    description: Reason is a unique, one-word, CamelCase reason for
                      the condition's last transition.
                    type: string
                  status:
                    description: Status is the status of the condition.
                    type: string
                  type:
                    description: Type is the type of the condition.
                    type: string
            patches:
              description: Patches is the list of SyncStatus for patches that have
                been applied.
              type: array
              items:
                description: SyncStatus describes objects that have been created or
                  patches that have been applied using the unique md5 sum of the object
                  or patch.
                type: object
                required:
                - apiVersion
                - conditions
                - hash
                - kind
                - name
                - namespace
                properties:
                  apiVersion:
                    description: APIVersion is the Group and Version of the object
                      that was synced or patched.
                    type: string
                  conditions:
                    description: Conditions is the list of conditions indicating success
                      or failure of object create, update and delete as well as patch
                      application.
                    type: array
                    items:
                      description: SyncCondition is a condition in a SyncStatus
                      type: object
                      required:
                      - status
                      - type
                      properties:
                        lastProbeTime:
                          description: LastProbeTime is the last time we probed the
                            condition.
                          type: string
                          format: date-time
                        lastTransitionTime:
                          description: LastTransitionTime is the last time the condition
                            transitioned from one status to another.
                          type: string
                          format: date-time
                        message:
                          description: Message is a human-readable message indicating
                            details about last transition.
                          type: string
                        reason:
                          description: Reason is a unique, one-word, CamelCase reason
                            for the condition's last transition.
                          type: string
                        status:
                          description: Status is the status of the condition.
                          type: string
                        type:
                          description: Type is the type of the condition.
                          type: string
                  hash:
                    description: Hash is the unique md5 hash of the resource or patch.
                    type: string
                  kind:
                    description: Kind is the Kind of the object that was synced or
                      patched.
                    type: string
                  name:
                    description: Name is the name of the object that was synced or
                      patched.
                    type: string
                  namespace:
                    description: Namespace is the Namespace of the object that was
                      synced or patched.
                    type: string
                  resource:
                    description: Resource is the resource name for the object that
                      was synced. This will be populated for resources, but not patches
                    type: string
            resources:
              description: Resources is the list of SyncStatus for objects that have
                been synced.
              type: array
              items:
                description: SyncStatus describes objects that have been created or
                  patches that have been applied using the unique md5 sum of the object
                  or patch.
                type: object
                required:
                - apiVersion
                - conditions
                - hash
                - kind
                - name
                - namespace
                properties:
                  apiVersion:
                    description: APIVersion is the Group and Version of the object
                      that was synced or patched.
                    type: string
                  conditions:
                    description: Conditions is the list of conditions indicating success
                      or failure of object create, update and delete as well as patch
                      application.
                    type: array
                    items:
                      description: SyncCondition is a condition in a SyncStatus
                      type: object
                      required:
                      - status
                      - type
                      properties:
                        lastProbeTime:
                          description: LastProbeTime is the last time we probed the
                            condition.
                          type: string
                          format: date-time
                        lastTransitionTime:
                          description: LastTransitionTime is the last time the condition
                            transitioned from one status to another.
                          type: string
                          format: date-time
                        message:
                          description: Message is a human-readable message indicating
                            details about last transition.
                          type: string
                        reason:
                          description: Reason is a unique, one-word, CamelCase reason
                            for the condition's last transition.
                          type: string
                        status:
                          description: Status is the status of the condition.
                          type: string
                        type:
                          description: Type is the type of the condition.
                          type: string
                  hash:
                    description: Hash is the unique md5 hash of the resource or patch.
                    type: string
                  kind:
                    description: Kind is the Kind of the object that was synced or
                      patched.
                    type: string
                  name:
                    description: Name is the name of the object that was synced or
                      patched.
                    type: string
                  namespace:
                    description: Namespace is the Namespace of the object that was
                      synced or patched.
                    type: string
                  resource:
                    description: Resource is the resource name for the object that
                      was synced. This will be populated for resources, but not patches
                    type: string
            secretReferences:
              description: Secrets is the list of SyncStatus for secrets that have
                been synced.
              type: array
              items:
                description: SyncStatus describes objects that have been created or
                  patches that have been applied using the unique md5 sum of the object
                  or patch.
                type: object
                required:
                - apiVersion
                - conditions
                - hash
                - kind
                - name
                - namespace
                properties:
                  apiVersion:
                    description: APIVersion is the Group and Version of the object
                      that was synced or patched.
                    type: string
                  conditions:
                    description: Conditions is the list of conditions indicating success
                      or failure of object create, update and delete as well as patch
                      application.
                    type: array
                    items:
                      description: SyncCondition is a condition in a SyncStatus
                      type: object
                      required:
                      - status
                      - type
                      properties:
                        lastProbeTime:
                          description: LastProbeTime is the last time we probed the
                            condition.
                          type: string
                          format: date-time
                        lastTransitionTime:
                          description: LastTransitionTime is the last time the condition
                            transitioned from one status to another.
                          type: string
                          format: date-time
                        message:
                          description: Message is a human-readable message indicating
                            details about last transition.
                          type: string
                        reason:
                          description: Reason is a unique, one-word, CamelCase reason
                            for the condition's last transition.
                          type: string
                        status:
                          description: Status is the status of the condition.
                          type: string
                        type:
                          description: Type is the type of the condition.
                          type: string
                  hash:
                    description: Hash is the unique md5 hash of the resource or patch.
                    type: string
                  kind:
                    description: Kind is the Kind of the object that was synced or
                      patched.
                    type: string
                  name:
                    description: Name is the name of the object that was synced or
                      patched.
                    type: string
                  namespace:
                    description: Namespace is the Namespace of the object that was
                      synced or patched.
                    type: string
                  resource:
                    description: Resource is the resource name for the object that
                      was synced. This will be populated for resources, but not patches
                    type: string
  version: v1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
`)

func configCrdsHive_v1_syncsetinstanceYamlBytes() ([]byte, error) {
	return _configCrdsHive_v1_syncsetinstanceYaml, nil
}

func configCrdsHive_v1_syncsetinstanceYaml() (*asset, error) {
	bytes, err := configCrdsHive_v1_syncsetinstanceYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "config/crds/hive_v1_syncsetinstance.yaml", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
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
    - name: AWSNATGatewayLimitExceeded
      searchRegexStrings:
      - "NatGatewayLimitExceeded"
      installFailingReason: AWSNATGatewayLimitExceeded
      installFailingMessage: AWS NAT gateway limit exceeded
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
    # Bare Metal
    - name: LibvirtSSHKeyPermissionDenied
      searchRegexStrings:
      - "platform.baremetal.libvirtURI: Internal error: could not connect to libvirt: virError.Code=38, Domain=7, Message=.Cannot recv data: Permission denied"
      installFailingReason: LibvirtSSHKeyPermissionDenied
      installFailingMessage: "Permission denied connecting to libvirt host, check SSH key configuration and pass phrase"
    # Processing stops at the first match, so this more generic
    # message about the connection failure must always come after the
    # more specific message for LibvirtSSHKeyPermissionDenied.
    - name: LibvirtConnectionFailed
      searchRegexStrings:
      - "could not connect to libvirt"
      installFailingReason: LibvirtConnectionFailed
      installFailingMessage: "Could not connect to libvirt host"
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
	"config/apiserver/apiservice.yaml":                          configApiserverApiserviceYaml,
	"config/apiserver/deployment.yaml":                          configApiserverDeploymentYaml,
	"config/apiserver/hiveapi_rbac_role.yaml":                   configApiserverHiveapi_rbac_roleYaml,
	"config/apiserver/hiveapi_rbac_role_binding.yaml":           configApiserverHiveapi_rbac_role_bindingYaml,
	"config/apiserver/service-account.yaml":                     configApiserverServiceAccountYaml,
	"config/apiserver/service.yaml":                             configApiserverServiceYaml,
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
	"config/rbac/hive_frontend_role.yaml":                       configRbacHive_frontend_roleYaml,
	"config/rbac/hive_frontend_role_binding.yaml":               configRbacHive_frontend_role_bindingYaml,
	"config/rbac/hive_frontend_serviceaccount.yaml":             configRbacHive_frontend_serviceaccountYaml,
	"config/rbac/hive_reader_role.yaml":                         configRbacHive_reader_roleYaml,
	"config/rbac/hive_reader_role_binding.yaml":                 configRbacHive_reader_role_bindingYaml,
	"config/crds/hive_v1_checkpoint.yaml":                       configCrdsHive_v1_checkpointYaml,
	"config/crds/hive_v1_clusterdeployment.yaml":                configCrdsHive_v1_clusterdeploymentYaml,
	"config/crds/hive_v1_clusterdeprovision.yaml":               configCrdsHive_v1_clusterdeprovisionYaml,
	"config/crds/hive_v1_clusterimageset.yaml":                  configCrdsHive_v1_clusterimagesetYaml,
	"config/crds/hive_v1_clusterprovision.yaml":                 configCrdsHive_v1_clusterprovisionYaml,
	"config/crds/hive_v1_clusterstate.yaml":                     configCrdsHive_v1_clusterstateYaml,
	"config/crds/hive_v1_dnszone.yaml":                          configCrdsHive_v1_dnszoneYaml,
	"config/crds/hive_v1_hiveconfig.yaml":                       configCrdsHive_v1_hiveconfigYaml,
	"config/crds/hive_v1_machinepool.yaml":                      configCrdsHive_v1_machinepoolYaml,
	"config/crds/hive_v1_machinepoolnamelease.yaml":             configCrdsHive_v1_machinepoolnameleaseYaml,
	"config/crds/hive_v1_selectorsyncidentityprovider.yaml":     configCrdsHive_v1_selectorsyncidentityproviderYaml,
	"config/crds/hive_v1_selectorsyncset.yaml":                  configCrdsHive_v1_selectorsyncsetYaml,
	"config/crds/hive_v1_syncidentityprovider.yaml":             configCrdsHive_v1_syncidentityproviderYaml,
	"config/crds/hive_v1_syncset.yaml":                          configCrdsHive_v1_syncsetYaml,
	"config/crds/hive_v1_syncsetinstance.yaml":                  configCrdsHive_v1_syncsetinstanceYaml,
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
		"apiserver": {nil, map[string]*bintree{
			"apiservice.yaml":                {configApiserverApiserviceYaml, map[string]*bintree{}},
			"deployment.yaml":                {configApiserverDeploymentYaml, map[string]*bintree{}},
			"hiveapi_rbac_role.yaml":         {configApiserverHiveapi_rbac_roleYaml, map[string]*bintree{}},
			"hiveapi_rbac_role_binding.yaml": {configApiserverHiveapi_rbac_role_bindingYaml, map[string]*bintree{}},
			"service-account.yaml":           {configApiserverServiceAccountYaml, map[string]*bintree{}},
			"service.yaml":                   {configApiserverServiceYaml, map[string]*bintree{}},
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
		"crds": {nil, map[string]*bintree{
			"hive_v1_checkpoint.yaml":                   {configCrdsHive_v1_checkpointYaml, map[string]*bintree{}},
			"hive_v1_clusterdeployment.yaml":            {configCrdsHive_v1_clusterdeploymentYaml, map[string]*bintree{}},
			"hive_v1_clusterdeprovision.yaml":           {configCrdsHive_v1_clusterdeprovisionYaml, map[string]*bintree{}},
			"hive_v1_clusterimageset.yaml":              {configCrdsHive_v1_clusterimagesetYaml, map[string]*bintree{}},
			"hive_v1_clusterprovision.yaml":             {configCrdsHive_v1_clusterprovisionYaml, map[string]*bintree{}},
			"hive_v1_clusterstate.yaml":                 {configCrdsHive_v1_clusterstateYaml, map[string]*bintree{}},
			"hive_v1_dnszone.yaml":                      {configCrdsHive_v1_dnszoneYaml, map[string]*bintree{}},
			"hive_v1_hiveconfig.yaml":                   {configCrdsHive_v1_hiveconfigYaml, map[string]*bintree{}},
			"hive_v1_machinepool.yaml":                  {configCrdsHive_v1_machinepoolYaml, map[string]*bintree{}},
			"hive_v1_machinepoolnamelease.yaml":         {configCrdsHive_v1_machinepoolnameleaseYaml, map[string]*bintree{}},
			"hive_v1_selectorsyncidentityprovider.yaml": {configCrdsHive_v1_selectorsyncidentityproviderYaml, map[string]*bintree{}},
			"hive_v1_selectorsyncset.yaml":              {configCrdsHive_v1_selectorsyncsetYaml, map[string]*bintree{}},
			"hive_v1_syncidentityprovider.yaml":         {configCrdsHive_v1_syncidentityproviderYaml, map[string]*bintree{}},
			"hive_v1_syncset.yaml":                      {configCrdsHive_v1_syncsetYaml, map[string]*bintree{}},
			"hive_v1_syncsetinstance.yaml":              {configCrdsHive_v1_syncsetinstanceYaml, map[string]*bintree{}},
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
