package main

// Render all created objects to disk, rather than directly to the apiserver.
// This is used for debugging

import (
	"fmt"
	"os"

	netv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/cluster-network-operator/pkg/network"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

func main() {
	err := render()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render() error {
	var configPath string
	var outPath string
	var manifestPath string

	pflag.StringVar(&configPath, "config", "", "json or yaml representation of NetworkConfig object")
	pflag.StringVar(&outPath, "out", "", "file to put rendered manifests")
	pflag.StringVar(&manifestPath, "bindata", "./bindata", "directory containing network manifests")
	pflag.Parse()

	if configPath == "" {
		return fmt.Errorf("--config must be specified")
	}
	if outPath == "" {
		return fmt.Errorf("--out must be specified")
	}

	conf, err := readConfigObject(configPath)
	if err != nil {
		return err
	}

	objs, err := network.Render(conf, manifestPath)
	if err != nil {
		return err
	}

	err = writeObjects(outPath, objs)
	return err
}

// readConfigObject reads a NetworkConfig object from disk
func readConfigObject(path string) (*netv1.NetworkConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open NetworkConfig file %s", path)
	}

	decoder := k8syaml.NewYAMLOrJSONDecoder(f, 4096)
	conf := netv1.NetworkConfig{}
	if err := decoder.Decode(&conf); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal NetworkConfig")
	}

	return &conf, nil
}

// writeObjects serializes the list of objects as a single yaml file
func writeObjects(path string, objs []*uns.Unstructured) error {
	fp, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.Wrapf(err, "could not open output file %s", path)
	}
	defer fp.Close()

	for _, obj := range objs {
		b, err := yaml.Marshal(obj)
		if err != nil {
			return errors.Wrapf(err, "could not marshal object %s %s %s",
				obj.GroupVersionKind().String(),
				obj.GetNamespace(),
				obj.GetName())
		}

		if _, err := fmt.Fprintln(fp, "\n---"); err != nil {
			return errors.Wrap(err, "write failed")
		}
		if _, err := fp.Write(b); err != nil {
			return errors.Wrap(err, "write failed")
		}
	}

	if err := fp.Close(); err != nil {
		return errors.Wrapf(err, "close failed")
	}
	return nil
}
