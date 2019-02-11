package assets

import (
	"fmt"
	"strings"
)

var _config_hiveadmission_apiservice_yaml = []byte(`---
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
  name: v1alpha1.admission.hive.openshift.io
  annotations:
    service.alpha.openshift.io/inject-cabundle: "true"
spec:
  group: admission.hive.openshift.io
  groupPriorityMinimum: 1000
  versionPriority: 15
  service:
    name: hiveadmission
    namespace: openshift-hive
  version: v1alpha1
`)

func config_hiveadmission_apiservice_yaml() ([]byte, error) {
	return _config_hiveadmission_apiservice_yaml, nil
}

var _config_hiveadmission_daemonset_yaml = []byte(`---
# to create the namespace-reservation-server
apiVersion: apps/v1
kind: DaemonSet
metadata:
  namespace: openshift-hive
  name: hiveadmission
  labels:
    app: hiveadmission
    hiveadmission: "true"
spec:
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
        - "--v=8"
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
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
        operator: Exists
      volumes:
      - name: serving-cert
        secret:
          defaultMode: 420
          secretName: hiveadmission-serving-cert
`)

func config_hiveadmission_daemonset_yaml() ([]byte, error) {
	return _config_hiveadmission_daemonset_yaml, nil
}

var _config_hiveadmission_service_account_yaml = []byte(`---
# to be able to assign powers to the hiveadmission process
apiVersion: v1
kind: ServiceAccount
metadata:
  namespace: openshift-hive
  name: hiveadmission
`)

func config_hiveadmission_service_account_yaml() ([]byte, error) {
	return _config_hiveadmission_service_account_yaml, nil
}

var _config_hiveadmission_service_yaml = []byte(`---
apiVersion: v1
kind: Service
metadata:
  namespace: openshift-hive
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

func config_hiveadmission_service_yaml() ([]byte, error) {
	return _config_hiveadmission_service_yaml, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
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
var _bindata = map[string]func() ([]byte, error){
	"config/hiveadmission/apiservice.yaml":      config_hiveadmission_apiservice_yaml,
	"config/hiveadmission/daemonset.yaml":       config_hiveadmission_daemonset_yaml,
	"config/hiveadmission/service-account.yaml": config_hiveadmission_service_account_yaml,
	"config/hiveadmission/service.yaml":         config_hiveadmission_service_yaml,
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
	for name := range node.Children {
		rv = append(rv, name)
	}
	return rv, nil
}

type _bintree_t struct {
	Func     func() ([]byte, error)
	Children map[string]*_bintree_t
}

var _bintree = &_bintree_t{nil, map[string]*_bintree_t{
	"config": {nil, map[string]*_bintree_t{
		"hiveadmission": {nil, map[string]*_bintree_t{
			"apiservice.yaml":      {config_hiveadmission_apiservice_yaml, map[string]*_bintree_t{}},
			"daemonset.yaml":       {config_hiveadmission_daemonset_yaml, map[string]*_bintree_t{}},
			"service-account.yaml": {config_hiveadmission_service_account_yaml, map[string]*_bintree_t{}},
			"service.yaml":         {config_hiveadmission_service_yaml, map[string]*_bintree_t{}},
		}},
	}},
}}
