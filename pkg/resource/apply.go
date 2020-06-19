package resource

import (
	"bytes"
	"context"
	"io"

	"github.com/jonboulle/clockwork"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	kresource "k8s.io/cli-runtime/pkg/resource"
	kcmdapply "k8s.io/kubectl/pkg/cmd/apply"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// ApplyResult indicates what type of change was performed
// by calling the Apply function
type ApplyResult string

const (
	// ConfiguredApplyResult is returned when a patch was submitted
	ConfiguredApplyResult ApplyResult = "configured"

	// UnchangedApplyResult is returned when no change occurred
	UnchangedApplyResult ApplyResult = "unchanged"

	// CreatedApplyResult is returned when a resource was created
	CreatedApplyResult ApplyResult = "created"

	// UnknownApplyResult is returned when the resulting action could not be determined
	UnknownApplyResult ApplyResult = "unknown"
)

const fieldTooLong metav1.CauseType = "FieldValueTooLong"

// Apply applies the given resource bytes to the target cluster specified by kubeconfig
func (r *Helper) Apply(obj []byte) (ApplyResult, error) {
	fileName, err := r.createTempFile("apply-", obj)
	if err != nil {
		r.logger.WithError(err).Error("failed to create temp file for apply")
		return "", err
	}
	defer r.deleteTempFile(fileName)
	factory, err := r.getFactory("")
	if err != nil {
		r.logger.WithError(err).Error("failed to obtain factory for apply")
		return "", err
	}
	ioStreams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	applyOptions, changeTracker, err := r.setupApplyCommand(factory, fileName, ioStreams)
	if err != nil {
		r.logger.WithError(err).Error("failed to setup apply command")
		return "", err
	}
	err = applyOptions.Run()
	if err != nil {
		r.logger.WithError(err).
			WithField("stdout", ioStreams.Out.(*bytes.Buffer).String()).
			WithField("stderr", ioStreams.ErrOut.(*bytes.Buffer).String()).Warn("running the apply command failed")
		return "", err
	}
	return changeTracker.GetResult(), nil
}

// ApplyRuntimeObject serializes an object and applies it to the target cluster specified by the kubeconfig.
func (r *Helper) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	data, err := r.Serialize(obj, scheme)
	if err != nil {
		r.logger.WithError(err).Warn("cannot serialize runtime object")
		return "", err
	}
	return r.Apply(data)
}

func (r *Helper) CreateOrUpdate(obj []byte) (ApplyResult, error) {
	factory, err := r.getFactory("")
	if err != nil {
		r.logger.WithError(err).Error("failed to obtain factory for apply")
		return "", err
	}

	errOut := &bytes.Buffer{}
	result, err := r.createOrUpdate(factory, obj, errOut)
	if err != nil {
		r.logger.WithError(err).
			WithField("stderr", errOut.String()).Warn("running the apply command failed")
		return "", err
	}
	return result, nil
}

func (r *Helper) CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	data, err := r.Serialize(obj, scheme)
	if err != nil {
		r.logger.WithError(err).Warn("cannot serialize runtime object")
		return "", err
	}
	return r.CreateOrUpdate(data)
}

func (r *Helper) Create(obj []byte) (ApplyResult, error) {
	factory, err := r.getFactory("")
	if err != nil {
		r.logger.WithError(err).Error("failed to obtain factory for apply")
		return "", err
	}
	result, err := r.createOnly(factory, obj)
	if err != nil {
		r.logger.WithError(err).Warn("running the create command failed")
		return "", err
	}
	return result, nil
}

func (r *Helper) CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	data, err := r.Serialize(obj, scheme)
	if err != nil {
		r.logger.WithError(err).Warn("cannot serialize runtime object")
		return "", err
	}
	return r.Create(data)
}

func (r *Helper) createOnly(f cmdutil.Factory, obj []byte) (ApplyResult, error) {
	info, err := r.getResourceInternalInfo(f, obj)
	if err != nil {
		return "", err
	}
	c, err := f.DynamicClient()
	if err != nil {
		return "", err
	}
	if err = info.Get(); err != nil {
		if !errors.IsNotFound(err) {
			return "", err
		}
		// Object doesn't exist yet, create it
		gvr := info.ResourceMapping().Resource
		_, err := c.Resource(gvr).Namespace(info.Namespace).Create(context.TODO(), info.Object.(*unstructured.Unstructured), metav1.CreateOptions{})
		if err != nil {
			return "", err
		}
		return CreatedApplyResult, nil
	}
	return UnchangedApplyResult, nil
}

func (r *Helper) createOrUpdate(f cmdutil.Factory, obj []byte, errOut io.Writer) (ApplyResult, error) {
	info, err := r.getResourceInternalInfo(f, obj)
	if err != nil {
		return "", err
	}
	c, err := f.DynamicClient()
	if err != nil {
		return "", err
	}
	sourceObj := info.Object.DeepCopyObject()
	if err = info.Get(); err != nil {
		if !errors.IsNotFound(err) {
			return "", err
		}
		// Object doesn't exist yet, create it
		gvr := info.ResourceMapping().Resource
		_, err := c.Resource(gvr).Namespace(info.Namespace).Create(context.TODO(), info.Object.(*unstructured.Unstructured), metav1.CreateOptions{})
		if err != nil {
			return "", err
		}
		return CreatedApplyResult, nil
	}
	openAPISchema, _ := f.OpenAPISchema()
	patcher := kcmdapply.Patcher{
		Mapping:       info.Mapping,
		Helper:        kresource.NewHelper(info.Client, info.Mapping),
		DynamicClient: c,
		Overwrite:     true,
		BackOff:       clockwork.NewRealClock(),
		OpenapiSchema: openAPISchema,
	}
	sourceBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, sourceObj)
	if err != nil {
		return "", err
	}
	patch, _, err := patcher.Patch(info.Object, sourceBytes, info.Source, info.Namespace, info.Name, errOut)
	if err != nil {
		return "", err
	}
	result := ConfiguredApplyResult
	if string(patch) == "{}" {
		result = UnchangedApplyResult
	}
	return result, nil
}

func (r *Helper) setupApplyCommand(f cmdutil.Factory, fileName string, ioStreams genericclioptions.IOStreams) (*kcmdapply.ApplyOptions, *changeTracker, error) {
	r.logger.Debug("setting up apply command")
	o := kcmdapply.NewApplyOptions(ioStreams)
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain dynamic client from factory")
		return nil, nil, err
	}
	o.DeleteOptions = o.DeleteFlags.ToOptions(dynamicClient, o.IOStreams)
	o.OpenAPISchema, _ = f.OpenAPISchema()
	o.Validator, err = f.Validator(false)
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain schema to validate objects from factory")
		return nil, nil, err
	}
	o.Builder = f.NewBuilder()
	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain RESTMapper from factory")
		return nil, nil, err
	}

	o.DynamicClient = dynamicClient
	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain namespace from factory")
		return nil, nil, err
	}
	tracker := &changeTracker{
		internalToPrinter: func(string) (printers.ResourcePrinter, error) { return o.PrintFlags.ToPrinter() },
	}
	o.ToPrinter = tracker.ToPrinter
	o.DeleteOptions.FilenameOptions.Filenames = []string{fileName}
	return o, tracker, nil
}

type trackerPrinter struct {
	setResult       func()
	internalPrinter printers.ResourcePrinter
}

func (p *trackerPrinter) PrintObj(o runtime.Object, w io.Writer) error {
	if p.setResult != nil {
		p.setResult()
	}
	return p.internalPrinter.PrintObj(o, w)
}

type changeTracker struct {
	result            []ApplyResult
	internalToPrinter func(string) (printers.ResourcePrinter, error)
}

func (t *changeTracker) GetResult() ApplyResult {
	if len(t.result) == 1 {
		return t.result[0]
	}
	return UnknownApplyResult
}

func (t *changeTracker) ToPrinter(name string) (printers.ResourcePrinter, error) {
	var f func()
	switch name {
	case "created":
		f = func() { t.result = append(t.result, CreatedApplyResult) }
	case "configured":
		f = func() { t.result = append(t.result, ConfiguredApplyResult) }
	case "unchanged":
		f = func() { t.result = append(t.result, UnchangedApplyResult) }
	}
	p, err := t.internalToPrinter(name)
	if err != nil {
		return nil, err
	}
	return &trackerPrinter{
		internalPrinter: p,
		setResult:       f,
	}, nil
}
