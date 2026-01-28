package internal

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	strcase "github.com/stoewer/go-strcase"

	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
)

// CreateOptionsWithEmbeddedAPI embeds API structs directly in CreateOptions.
type CreateOptionsWithEmbeddedAPI struct {
	ClusterDeploymentName string
	Namespace             string
	PoolName              string
	Platform              string
	Replicas              int64
	InstanceType          string
	Zones                 []string

	AWS       *hivev1aws.MachinePoolPlatform `flag:"aws"`
	GCP       *hivev1gcp.MachinePool         `flag:"gcp"`
	Azure     *hivev1azure.MachinePool       `flag:"azure"`
	OpenStack *hivev1openstack.MachinePool   `flag:"openstack"`
	VSphere   *hivev1vsphere.MachinePool     `flag:"vsphere"`
	IBMCloud  *hivev1ibmcloud.MachinePool    `flag:"ibmcloud"`
	Nutanix   *hivev1nutanix.MachinePool     `flag:"nutanix"`

	Labels        map[string]string
	MachineLabels map[string]string
	Taints        []string
}

type genericStringPtrWrapper struct {
	val reflect.Value
	typ reflect.Type
}

type customStringWrapper struct {
	val reflect.Value
	typ reflect.Type
}

type structSliceWrapper struct {
	val       reflect.Value
	typ       reflect.Type
	fieldName string
}

type pointerStructWrapper struct {
	fieldValue reflect.Value
}

var (
	pointerWrappers    []*pointerStructWrapper
	platformList       = "aws|gcp|azure|openstack|vsphere|nutanix|ibmcloud"
	DefaultDescPrefix  = "Field: "
	fieldCommentsCache = make(map[string]string)
	fieldCommentsMu    sync.RWMutex
	workspaceRootCache string
	workspaceRootOnce  sync.Once
)

// AddFlags automatically binds all flags for CreateOptionsWithEmbeddedAPI.
func AddFlags(opts *CreateOptionsWithEmbeddedAPI, cmd *cobra.Command) error {
	pointerWrappers = nil
	bindBasicFlags(cmd.Flags(), opts)
	return bindPlatformFlags(cmd.Flags(), opts)
}

// PostParseFlags cleans up empty pointer structs after flag parsing.
func PostParseFlags() error {
	for _, w := range pointerWrappers {
		if err := w.cleanupIfEmpty(); err != nil {
			return err
		}
	}
	return nil
}

func GetPlatformStruct(opts *CreateOptionsWithEmbeddedAPI, platform string) (interface{}, error) {
	optsValue := reflect.ValueOf(opts).Elem()
	for i := 0; i < optsValue.Type().NumField(); i++ {
		field := optsValue.Type().Field(i)
		if field.Tag.Get("flag") == platform {
			val := optsValue.Field(i)
			if val.Kind() == reflect.Pointer && val.IsNil() {
				return nil, errors.Errorf("platform %s config is nil", platform)
			}
			return val.Interface(), nil
		}
	}
	return nil, errors.Errorf("unsupported platform: %s", platform)
}

func bindBasicFlags(flags *pflag.FlagSet, opts *CreateOptionsWithEmbeddedAPI) {
	flags.StringVarP(&opts.ClusterDeploymentName, "cluster-deployment", "c", "", "ClusterDeployment name (required)")
	flags.StringVarP(&opts.Namespace, "namespace", "n", "", "Namespace (defaults to current kubeconfig namespace)")
	flags.StringVar(&opts.PoolName, "pool", "", "MachinePool name (required)")
	flags.StringVar(&opts.Platform, "platform", "", "Platform: "+platformList+" (required)")
	flags.Int64Var(&opts.Replicas, "replicas", 1, "Number of replicas")
	flags.StringToStringVar(&opts.Labels, "labels", map[string]string{}, "Node labels (key=value)")
	flags.StringToStringVar(&opts.MachineLabels, "machine-labels", map[string]string{}, "Machine labels (key=value)")
	flags.StringArrayVar(&opts.Taints, "taints", []string{}, "Node taints (key=value:effect)")
}

func bindPlatformFlags(flags *pflag.FlagSet, opts *CreateOptionsWithEmbeddedAPI) error {
	optsValue := reflect.ValueOf(opts).Elem()
	for i := 0; i < optsValue.Type().NumField(); i++ {
		field := optsValue.Type().Field(i)
		if prefix := field.Tag.Get("flag"); prefix != "" {
			if err := bindNestedFlags(flags, prefix+"-", optsValue.Field(i), field.Type, ""); err != nil {
				return errors.Wrapf(err, "failed to bind flags for platform %s", prefix)
			}
		}
	}
	return nil
}

func getJSONTagName(tag reflect.StructTag) string {
	jsonTag := tag.Get("json")
	if jsonTag == "" {
		return ""
	}
	name := strings.Split(jsonTag, ",")[0]
	if name == "" || name == "-" {
		return ""
	}
	return name
}

func getFlagNameFromField(field reflect.StructField) string {
	if name := getJSONTagName(field.Tag); name != "" {
		return smartKebabCase(name)
	}
	return smartKebabCase(field.Name)
}

// smartKebabCase converts camelCase/PascalCase to kebab-case with smart handling of abbreviations.
// It normalizes common abbreviations before conversion to avoid issues like "IDs" -> "i-ds".
//
// Examples:
//   - additionalSecurityGroupIDs -> additional-security-group-ids (not i-ds)
//   - tagIDs -> tag-ids (not tag-i-ds)
//   - memoryMiB -> memory-mib (not memory-mi-b)
func smartKebabCase(s string) string {
	replacements := map[string]string{
		"IDs": "Ids", "UUIDs": "Uuids", "URLs": "Urls", "ARNs": "Arns",
		"MiB": "Mib", "GiB": "Gib", "TiB": "Tib", "PiB": "Pib",
		"MB": "Mb", "GB": "Gb", "TB": "Tb", "PB": "Pb",
	}
	for from, to := range replacements {
		s = strings.ReplaceAll(s, from, to)
	}
	return strcase.KebabCase(s)
}

func bindNestedFlags(flags *pflag.FlagSet, prefix string, val reflect.Value, typ reflect.Type, structTypeName string) error {
	if typ.Kind() == reflect.Pointer {
		if val.IsNil() {
			val.Set(reflect.New(typ.Elem()))
		}
		val, typ = val.Elem(), typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return errors.Errorf("expected struct, got %s", typ.Kind())
	}
	if structTypeName == "" {
		structTypeName = typ.Name()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		flagName := prefix + getFlagNameFromField(field)
		if !fieldVal.CanSet() || field.Tag.Get("json") == "-" || flags.Lookup(flagName) != nil {
			continue
		}

		desc := getDescription(field.Tag, field.Name, typ, structTypeName)

		switch field.Type.Kind() {
		case reflect.Struct:
			if err := bindNestedFlags(flags, flagName+"-", fieldVal, field.Type, field.Type.Name()); err != nil {
				return errors.Wrapf(err, "failed to bind nested field %s", field.Name)
			}
		case reflect.Pointer:
			if field.Type.Elem().Kind() == reflect.Struct {
				if fieldVal.IsNil() {
					fieldVal.Set(reflect.New(field.Type.Elem()))
				}
				if err := bindNestedFlags(flags, flagName+"-", fieldVal, field.Type, field.Type.Elem().Name()); err != nil {
					return errors.Wrapf(err, "failed to bind pointer field %s", field.Name)
				}
				pointerWrappers = append(pointerWrappers, &pointerStructWrapper{fieldValue: fieldVal})
			} else {
				if err := bindPrimitivePtrFlag(flags, flagName, fieldVal, field, desc); err != nil {
					return errors.Wrapf(err, "failed to bind pointer field %s", field.Name)
				}
			}
		default:
			if err := bindPrimitiveFlag(flags, flagName, fieldVal, field, desc); err != nil {
				continue
			}
		}
	}
	return nil
}

func bindPrimitiveFlag(flags *pflag.FlagSet, name string, val reflect.Value, field reflect.StructField, desc string) error {
	if !val.CanAddr() {
		return errors.Errorf("cannot get address of field %s", field.Name)
	}
	ptr := val.Addr().Interface()

	switch field.Type.Kind() {
	case reflect.String:
		if field.Type == reflect.TypeFor[string]() {
			flags.StringVar(ptr.(*string), name, "", desc)
		} else {
			flags.Var(&customStringWrapper{val: val, typ: field.Type}, name, desc)
		}
	case reflect.Int:
		flags.IntVar(ptr.(*int), name, 0, desc)
	case reflect.Int32:
		flags.Int32Var(ptr.(*int32), name, 0, desc)
	case reflect.Int64:
		flags.Int64Var(ptr.(*int64), name, 0, desc)
	case reflect.Slice:
		switch field.Type.Elem().Kind() {
		case reflect.String:
			flags.StringSliceVar(ptr.(*[]string), name, []string{}, desc)
		case reflect.Struct:
			flags.Var(&structSliceWrapper{val: val, typ: field.Type, fieldName: field.Name}, name, desc+" (JSON format: each element as JSON object)")
		default:
			return errors.Errorf("unsupported slice element type: %s", field.Type.Elem().Kind())
		}
	case reflect.Map:
		if field.Type.Key().Kind() != reflect.String || field.Type.Elem().Kind() != reflect.String {
			return errors.New("unsupported map type")
		}
		flags.StringToStringVar(ptr.(*map[string]string), name, map[string]string{}, desc)
	default:
		return errors.Errorf("unsupported field type: %s", field.Type.Kind())
	}
	return nil
}

func bindPrimitivePtrFlag(flags *pflag.FlagSet, name string, val reflect.Value, field reflect.StructField, desc string) error {
	if field.Type.Elem().Kind() != reflect.String {
		return errors.Errorf("unsupported pointer element type: %s", field.Type.Elem().Kind())
	}
	flags.Var(&genericStringPtrWrapper{val: val, typ: field.Type}, name, desc)
	return nil
}

func (w *genericStringPtrWrapper) String() string {
	if w.val.IsNil() || !w.val.Elem().IsValid() {
		return ""
	}
	return w.val.Elem().String()
}

func (w *genericStringPtrWrapper) Set(s string) error {
	if w.val.IsNil() {
		w.val.Set(reflect.New(w.typ.Elem()))
	}
	elem := w.val.Elem()
	if elem.Kind() == reflect.String {
		elem.SetString(s)
		return nil
	}
	strVal := reflect.ValueOf(s)
	if !strVal.Type().ConvertibleTo(elem.Type()) {
		return errors.Errorf("cannot convert string to %s", elem.Type())
	}
	elem.Set(strVal.Convert(elem.Type()))
	return nil
}

func (w *genericStringPtrWrapper) Type() string { return "string" }

func (w *customStringWrapper) String() string {
	if !w.val.IsValid() {
		return ""
	}
	return w.val.String()
}

func (w *customStringWrapper) Set(s string) error {
	if !w.val.IsValid() || !w.val.CanSet() {
		return errors.Errorf("cannot set value for type %s", w.typ)
	}
	strVal := reflect.ValueOf(s)
	if !strVal.Type().ConvertibleTo(w.typ) {
		return errors.Errorf("cannot convert string to %s", w.typ)
	}
	w.val.Set(strVal.Convert(w.typ))
	return nil
}

func (w *customStringWrapper) Type() string { return "string" }

func (w *structSliceWrapper) String() string {
	if !w.val.IsValid() || w.val.IsNil() || w.val.Len() == 0 {
		return "[]"
	}
	data, err := json.Marshal(w.val.Interface())
	if err != nil {
		return "<error: " + err.Error() + ">"
	}
	return string(data)
}

func (w *structSliceWrapper) Set(s string) error {
	if s == "" {
		return nil
	}
	var jsonArray []json.RawMessage
	if err := json.Unmarshal([]byte(s), &jsonArray); err != nil {
		var singleObj json.RawMessage
		if err := json.Unmarshal([]byte(s), &singleObj); err != nil {
			return errors.Wrapf(err, "invalid JSON format for %s (expected JSON array or object)", w.fieldName)
		}
		jsonArray = []json.RawMessage{singleObj}
	}
	elemType := w.typ.Elem()
	slice := reflect.MakeSlice(w.typ, 0, len(jsonArray))
	for _, jsonItem := range jsonArray {
		elem := reflect.New(elemType).Interface()
		if err := json.Unmarshal(jsonItem, elem); err != nil {
			return errors.Wrapf(err, "failed to unmarshal JSON item for %s", w.fieldName)
		}
		slice = reflect.Append(slice, reflect.ValueOf(elem).Elem())
	}
	w.val.Set(slice)
	return nil
}

func (w *structSliceWrapper) Type() string { return "json" }

func (w *pointerStructWrapper) cleanupIfEmpty() error {
	if w.fieldValue.IsNil() {
		return nil
	}
	v := w.fieldValue.Elem()
	if !hasNonZeroValue(v) {
		w.fieldValue.Set(reflect.Zero(w.fieldValue.Type()))
	}
	return nil
}

func getDescription(tag reflect.StructTag, fieldName string, structType reflect.Type, structTypeName string) string {
	prefix := DefaultDescPrefix
	if customPrefix := tag.Get("descPrefix"); customPrefix != "" {
		prefix = customPrefix
	}
	baseDesc := fieldName
	if name := getJSONTagName(tag); name != "" {
		baseDesc = name
	}
	baseDesc = prefix + baseDesc
	if structType != nil && structTypeName != "" {
		if comment := getFieldComment(structType, structTypeName, fieldName); comment != "" {
			baseDesc += " " + comment
		}
	}
	return baseDesc
}

func getFieldComment(fieldType reflect.Type, structTypeName, fieldName string) string {
	cacheKey := fieldType.PkgPath() + "." + structTypeName + "." + fieldName

	fieldCommentsMu.RLock()
	comment, cached := fieldCommentsCache[cacheKey]
	fieldCommentsMu.RUnlock()
	if cached {
		return comment
	}

	comment = extractFieldComment(fieldType, structTypeName, fieldName)
	fieldCommentsMu.Lock()
	fieldCommentsCache[cacheKey] = comment
	fieldCommentsMu.Unlock()
	return comment
}

func extractFieldComment(fieldType reflect.Type, structTypeName, fieldName string) string {
	pkgPath := fieldType.PkgPath()
	if pkgPath == "" {
		return ""
	}
	sourceFile := findSourceFile(pkgPath, structTypeName)
	if sourceFile == "" {
		return ""
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, sourceFile, nil, parser.ParseComments)
	if err != nil {
		return ""
	}
	var comment string
	ast.Inspect(f, func(n ast.Node) bool {
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			return true
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != structTypeName {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 || field.Names[0].Name != fieldName {
					continue
				}
				if field.Doc != nil {
					comment = fmtRawDoc(field.Doc.Text())
					return false
				}
			}
		}
		return true
	})
	return comment
}

func fmtRawDoc(rawDoc string) string {
	if rawDoc == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(rawDoc, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "//"))
		if line == "" || strings.HasPrefix(line, "+") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(line)
	}
	return strings.TrimSpace(b.String())
}

func findSourceFile(pkgPath, typeName string) string {
	workspaceRoot := findWorkspaceRoot()
	if workspaceRoot == "" {
		return ""
	}
	pathParts := strings.Split(pkgPath, "/")
	if len(pathParts) < 2 {
		return ""
	}
	for i, part := range pathParts {
		if part == "apis" && i < len(pathParts) {
			if file := findTypeFile(filepath.Join(workspaceRoot, filepath.Join(pathParts[i:]...)), typeName); file != "" {
				return file
			}
			break
		}
	}
	return findTypeFile(filepath.Join(workspaceRoot, "vendor", pkgPath), typeName)
}

func findWorkspaceRoot() string {
	workspaceRootOnce.Do(func() {
		for _, dir := range []string{
			func() string { wd, _ := os.Getwd(); return wd }(),
			func() string { exec, _ := os.Executable(); return filepath.Dir(exec) }(),
		} {
			if root := findRoot(dir); root != "" {
				workspaceRootCache = root
				return
			}
		}
	})
	return workspaceRootCache
}

func findRoot(dir string) string {
	markers := []string{"go.mod", "apis"}
	for dir != "" {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func findTypeFile(dir, typeName string) string {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return ""
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		var found bool
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != typeName {
				return true
			}
			found = true
			return false
		})
		if found {
			return file
		}
	}
	return ""
}

func hasNonZeroValue(v reflect.Value) bool {
	if v.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < v.NumField(); i++ {
		if !isEmpty(v.Field(i)) {
			return true
		}
	}
	return false
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
