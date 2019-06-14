package resource

import (
	"bytes"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdpatch "k8s.io/kubernetes/pkg/kubectl/cmd/patch"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	patchTypes = map[string]types.PatchType{
		"json":      types.JSONPatchType,
		"merge":     types.MergePatchType,
		"strategic": types.StrategicMergePatchType,
	}
)

// Patch invokes the kubectl patch command with the given resource, patch and patch type
func (r *Helper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error {

	ioStreams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	factory, err := r.getFactory(name.Namespace)
	if err != nil {
		return err
	}
	patchOptions, err := r.setupPatchCommand(name.Name, kind, apiVersion, patchType, factory, string(patch), ioStreams)
	if err != nil {
		r.logger.WithError(err).Error("failed to setup patch command")
		return err
	}
	err = patchOptions.RunPatch()
	if err != nil {
		r.logger.WithError(err).
			WithField("stdout", ioStreams.Out.(*bytes.Buffer).String()).
			WithField("stderr", ioStreams.ErrOut.(*bytes.Buffer).String()).Error("running the patch command failed")
		return err
	}
	return nil
}

func (r *Helper) setupPatchCommand(name, kind, apiVersion, patchType string, f cmdutil.Factory, patch string, ioStreams genericclioptions.IOStreams) (*kcmdpatch.PatchOptions, error) {

	cmd := kcmdpatch.NewCmdPatch(f, ioStreams)
	cmd.Flags().Parse([]string{})

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		r.logger.WithError(err).WithField("groupVersion", apiVersion).Error("cannot parse group version")
		return nil, err
	}
	args := []string{fmt.Sprintf("%s.%s.%s/%s", kind, gv.Version, gv.Group, name)}

	o := kcmdpatch.NewPatchOptions(ioStreams)
	o.Complete(f, cmd, args)
	if patchType == "" {
		patchType = "strategic"
	}
	_, ok := patchTypes[patchType]
	if !ok {
		return nil, fmt.Errorf("Invalid patch type: %s. Valid patch types are 'strategic', 'merge' or 'json'", patchType)
	}
	o.PatchType = patchType
	o.Patch = patch

	return o, nil
}
