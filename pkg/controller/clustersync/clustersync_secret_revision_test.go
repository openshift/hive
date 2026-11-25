package clustersync

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testlogger "github.com/openshift/hive/pkg/test/logger"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testselectorsyncset "github.com/openshift/hive/pkg/test/selectorsyncset"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	"github.com/openshift/hive/pkg/util/scheme"
)

// Messages of the source-revision log events, repeated here so that a change to any of them is a
// deliberate change to the contract these tests pin rather than a silent rename.
const (
	msgReadSourceRevision    = "read source secret revision"
	msgAppliedSourceRevision = "applied secret for observed source revision"
	msgFailedSourceRevision  = "failed to apply secret for observed source revision"
	msgCannotReadSecret      = "cannot read secret"
)

// testSourceSecretUID is the UID the fixtures give their source Secret, asserted literally so
// that the revision assertions cannot pass by comparing two empty strings.
const testSourceSecretUID = "source-secret-uid"

// newSecretRevisionLogger returns a debug-level logger whose entries are captured by the returned
// hook and whose output is discarded, so a failing assertion is not buried in test logs.
func newSecretRevisionLogger() (log.FieldLogger, *logtest.Hook) {
	logger, hook := testlogger.NewLoggerWithHook()
	logger.Out = io.Discard
	return logger.WithField("controller", "clustersync"), hook
}

// entriesWithMessage returns the captured entries carrying the given message.
func entriesWithMessage(hook *logtest.Hook, message string) []*log.Entry {
	var matched []*log.Entry
	for _, entry := range hook.AllEntries() {
		if entry.Message == message {
			matched = append(matched, entry)
		}
	}
	return matched
}

// onlyEntryWithMessage asserts that exactly one entry carries the message and returns it.
func onlyEntryWithMessage(t *testing.T, hook *logtest.Hook, message string) *log.Entry {
	t.Helper()
	matched := entriesWithMessage(hook, message)
	require.Len(t, matched, 1, "expected exactly one %q entry", message)
	return matched[0]
}

// renderEntries flattens every captured entry into strings so that a test can assert that a
// value never appears anywhere in the emitted logs, in a message or in a field.
func renderEntries(hook *logtest.Hook) []string {
	var rendered []string
	for _, entry := range hook.AllEntries() {
		rendered = append(rendered, entry.Message)
		for key, value := range entry.Data {
			rendered = append(rendered, fmt.Sprintf("%s=%v", key, value))
		}
	}
	return rendered
}

// secretRevisionSyncSet builds a SyncSet in the test namespace with the given generation and
// source secret names mapped into the target cluster. The mappings leave the source namespace
// empty, so they inherit the SyncSet's namespace.
func secretRevisionSyncSet(typer runtime.ObjectTyper, generation int64, srcNames ...string) *hivev1.SyncSet {
	mappings := make([]hivev1.SecretMapping, len(srcNames))
	for i, srcName := range srcNames {
		mappings[i] = testSecretMapping(srcName, fmt.Sprintf("dest-namespace-%d", i), fmt.Sprintf("dest-name-%d", i))
	}
	return testsyncset.FullBuilder(testNamespace, "test-syncset", typer).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(generation),
		testsyncset.WithSecrets(mappings...),
	)
}

// sourceSecret builds a source Secret in the test namespace carrying a fixed UID.
func sourceSecret(typer runtime.ObjectTyper, name string, opts ...testsecret.Option) *corev1.Secret {
	return testsecret.FullBuilder(testNamespace, name, typer).
		GenericOptions(testgeneric.WithUID(testSourceSecretUID + "-" + name)).
		Build(append([]testsecret.Option{testsecret.WithDataKeyValue("test-key", []byte("test-data"))}, opts...)...)
}

// applySecretForMapping drives applySecret for one mapping of a syncset, the way applySyncSet
// drives it.
func applySecretForMapping(
	r *ReconcileClusterSync,
	syncSet CommonSyncSet,
	index int,
	applyFn func(obj []byte) (resource.ApplyResult, error),
	logger log.FieldLogger,
) (error, bool) {
	mapping := syncSet.GetSpec().Secrets[index]
	reference := testSecretRef(mapping.TargetRef.Namespace, mapping.TargetRef.Name)
	return r.applySecret(syncSet, index, mapping, reference, applyFn, labelApply, logger)
}

// storedResourceVersion reads a source Secret back from the client, so that a test pins the
// logged resourceVersion against what the client actually holds rather than against a value the
// fake client is free to rewrite.
func storedResourceVersion(t *testing.T, c client.Client, name string) string {
	t.Helper()
	secret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: name}, secret))
	require.NotEmpty(t, secret.ResourceVersion, "the client should hold a resourceVersion to report")
	return secret.ResourceVersion
}

func successfulApplyFn(_ []byte) (resource.ApplyResult, error) {
	return resource.CreatedApplyResult, nil
}

// TestApplySecret_ReadApplyCorrelation pins that a successful mapping reports the revision it
// read and then reports the apply outcome against that same revision.
func TestApplySecret_ReadApplyCorrelation(t *testing.T) {
	scheme := scheme.GetScheme()
	srcSecret := sourceSecret(scheme, "test-secret")
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(srcSecret).Build()
	r := &ReconcileClusterSync{Client: c}
	syncSet := secretRevisionSyncSet(scheme, 7, "test-secret")
	logger, hook := newSecretRevisionLogger()

	returnErr, requeue := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 0, successfulApplyFn, logger)

	assert.NoError(t, returnErr, "expected the apply to succeed")
	assert.False(t, requeue, "expected no requeue")

	resourceVersion := storedResourceVersion(t, c, "test-secret")
	read := onlyEntryWithMessage(t, hook, msgReadSourceRevision)
	applied := onlyEntryWithMessage(t, hook, msgAppliedSourceRevision)
	for _, entry := range []*log.Entry{read, applied} {
		assert.Equal(t, testSourceSecretUID+"-test-secret", entry.Data["sourceSecretUID"])
		assert.Equal(t, resourceVersion, entry.Data["sourceSecretResourceVersion"])
		assert.Equal(t, testNamespace, entry.Data["sourceSecretNamespace"])
		assert.Equal(t, "test-secret", entry.Data["sourceSecretName"])
		assert.Equal(t, int64(7), entry.Data["syncSetGeneration"])
	}
	// The read is diagnostic detail at the level of the other per-object lines on this path; the
	// outcome, which carries the same captured revision, is the one event reported at info.
	assert.Equal(t, log.DebugLevel, read.Level)
	assert.Equal(t, log.InfoLevel, applied.Level)
	assert.Empty(t, entriesWithMessage(hook, msgFailedSourceRevision),
		"a successful apply must not report a failure")
}

// TestApplySecret_SourceNamespaceResolution pins that the reported source namespace is the one
// actually read from, on both syncset kinds.
func TestApplySecret_SourceNamespaceResolution(t *testing.T) {
	scheme := scheme.GetScheme()
	cases := []struct {
		name              string
		buildSyncSet      func(runtime.ObjectTyper, hivev1.SecretMapping) CommonSyncSet
		srcNamespace      string
		expectedNamespace string
		expectedGen       int64
	}{
		{
			name: "SyncSet defaulting the source namespace",
			buildSyncSet: func(typer runtime.ObjectTyper, mapping hivev1.SecretMapping) CommonSyncSet {
				return (*SyncSetAsCommon)(testsyncset.FullBuilder(testNamespace, "test-syncset", typer).Build(
					testsyncset.ForClusterDeployments(testCDName),
					testsyncset.WithGeneration(4),
					testsyncset.WithSecrets(mapping),
				))
			},
			srcNamespace:      "",
			expectedNamespace: testNamespace,
			expectedGen:       4,
		},
		{
			name: "SyncSet with an explicit source namespace",
			buildSyncSet: func(typer runtime.ObjectTyper, mapping hivev1.SecretMapping) CommonSyncSet {
				return (*SyncSetAsCommon)(testsyncset.FullBuilder(testNamespace, "test-syncset", typer).Build(
					testsyncset.ForClusterDeployments(testCDName),
					testsyncset.WithGeneration(9),
					testsyncset.WithSecrets(mapping),
				))
			},
			srcNamespace:      testNamespace,
			expectedNamespace: testNamespace,
			expectedGen:       9,
		},
		{
			name: "SelectorSyncSet, which requires an explicit source namespace",
			buildSyncSet: func(typer runtime.ObjectTyper, mapping hivev1.SecretMapping) CommonSyncSet {
				sss := testselectorsyncset.FullBuilder("test-selectorsyncset", typer).Build(
					testselectorsyncset.WithGeneration(12),
					testselectorsyncset.WithSecrets(mapping),
				)
				return (*SelectorSyncSetAsCommon)(sss)
			},
			srcNamespace:      testNamespace,
			expectedNamespace: testNamespace,
			expectedGen:       12,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcSecret := sourceSecret(scheme, "test-secret")
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(srcSecret).Build()
			r := &ReconcileClusterSync{Client: c}
			mapping := testSecretMapping("test-secret", "dest-namespace-0", "dest-name-0")
			mapping.SourceRef.Namespace = tc.srcNamespace
			logger, hook := newSecretRevisionLogger()

			returnErr, _ := applySecretForMapping(r, tc.buildSyncSet(scheme, mapping), 0, successfulApplyFn, logger)

			require.NoError(t, returnErr)
			read := onlyEntryWithMessage(t, hook, msgReadSourceRevision)
			assert.Equal(t, tc.expectedNamespace, read.Data["sourceSecretNamespace"])
			assert.Equal(t, tc.expectedGen, read.Data["syncSetGeneration"])
			assert.Len(t, entriesWithMessage(hook, msgAppliedSourceRevision), 1)
		})
	}
}

// TestApplySecret_NoRevisionReportedWithoutASuccessfulRead pins that every path that returns
// before the source Secret has been read leaves the revision events unemitted, so a revision
// event is never a claim about a read that did not happen.
func TestApplySecret_NoRevisionReportedWithoutASuccessfulRead(t *testing.T) {
	scheme := scheme.GetScheme()
	cases := []struct {
		name            string
		srcNamespace    string
		seedSource      bool
		selectorSyncSet bool
		expectedErr     string
		expectedRequeue bool
		expectedMessage string
	}{
		{
			name:            "source Secret does not exist",
			seedSource:      false,
			expectedErr:     "failed to read secret 0",
			expectedRequeue: true,
			expectedMessage: msgCannotReadSecret,
		},
		{
			name:            "source namespace missing on a SelectorSyncSet",
			selectorSyncSet: true,
			seedSource:      true,
			expectedErr:     "source namespace missing for secret 0",
			expectedRequeue: false,
			expectedMessage: "namespace must be specified for source secret",
		},
		{
			name:            "source namespace does not match the SyncSet namespace",
			srcNamespace:    "some-other-namespace",
			seedSource:      true,
			expectedErr:     "source in wrong namespace for secret 0",
			expectedRequeue: false,
			expectedMessage: "source secret must be in same namespace as SyncSet",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seeded []runtime.Object
			if tc.seedSource {
				seeded = append(seeded, sourceSecret(scheme, "test-secret"))
			}
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(seeded...).Build()
			r := &ReconcileClusterSync{Client: c}
			mapping := testSecretMapping("test-secret", "dest-namespace-0", "dest-name-0")
			mapping.SourceRef.Namespace = tc.srcNamespace
			var syncSet CommonSyncSet
			if tc.selectorSyncSet {
				syncSet = (*SelectorSyncSetAsCommon)(testselectorsyncset.FullBuilder("test-selectorsyncset", scheme).Build(
					testselectorsyncset.WithGeneration(1),
					testselectorsyncset.WithSecrets(mapping),
				))
			} else {
				syncSet = (*SyncSetAsCommon)(testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					testsyncset.WithGeneration(1),
					testsyncset.WithSecrets(mapping),
				))
			}
			logger, hook := newSecretRevisionLogger()

			applyCalled := false
			returnErr, requeue := applySecretForMapping(r, syncSet, 0, func(_ []byte) (resource.ApplyResult, error) {
				applyCalled = true
				return resource.CreatedApplyResult, nil
			}, logger)

			assert.False(t, applyCalled, "nothing should be applied when the source was not read")
			require.Error(t, returnErr)
			assert.Contains(t, returnErr.Error(), tc.expectedErr, "the original error wrapping is unchanged")
			assert.Equal(t, tc.expectedRequeue, requeue)

			assert.Empty(t, entriesWithMessage(hook, msgReadSourceRevision),
				"a read that did not succeed must never be reported as an observed revision")
			assert.Empty(t, entriesWithMessage(hook, msgAppliedSourceRevision))
			assert.Empty(t, entriesWithMessage(hook, msgFailedSourceRevision))
			assert.Len(t, entriesWithMessage(hook, tc.expectedMessage), 1,
				"the pre-existing line for this path is unchanged")
		})
	}
}

// TestApplySecret_ReadFailureIsClassifiedUnchanged pins that the pre-existing read failure still
// surfaces the API classification of the error it wraps.
func TestApplySecret_ReadFailureIsClassifiedUnchanged(t *testing.T) {
	scheme := scheme.GetScheme()
	c := testfake.NewFakeClientBuilder().Build()
	r := &ReconcileClusterSync{Client: c}
	syncSet := secretRevisionSyncSet(scheme, 3, "missing-secret")
	logger, _ := newSecretRevisionLogger()

	returnErr, requeue := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 0, successfulApplyFn, logger)

	require.Error(t, returnErr)
	assert.Contains(t, returnErr.Error(), "failed to read secret 0")
	assert.True(t, apierrors.IsNotFound(errors.Cause(returnErr)), "the wrapped NotFound is preserved")
	assert.True(t, requeue, "a read failure still requeues")
}

// TestApplySecret_ApplyFailureRetainsSourceRevision pins that a failed apply still says which
// revision failed, categorizes the error, and never reproduces the error text on the new line.
func TestApplySecret_ApplyFailureRetainsSourceRevision(t *testing.T) {
	cases := []struct {
		name             string
		applyErr         error
		expectedCategory string
		expectedLevel    log.Level
	}{
		{
			name: "conflict",
			applyErr: apierrors.NewConflict(
				schema.GroupResource{Resource: "secrets"}, "dest-name-0", errors.New("conflict-detail-sentinel")),
			expectedCategory: "Conflict",
			expectedLevel:    log.InfoLevel,
		},
		{
			name: "forbidden",
			applyErr: apierrors.NewForbidden(
				schema.GroupResource{Resource: "secrets"}, "dest-name-0", errors.New("forbidden-detail-sentinel")),
			expectedCategory: "Forbidden",
			expectedLevel:    log.ErrorLevel,
		},
		{
			name: "invalid, whose message quotes the rejected value",
			applyErr: apierrors.NewInvalid(
				schema.GroupKind{Kind: "Secret"}, "dest-name-0",
				field.ErrorList{field.Invalid(field.NewPath("data"), "invalid-value-sentinel", "bad")}),
			expectedCategory: "Invalid",
			expectedLevel:    log.ErrorLevel,
		},
		{
			name:             "transport timeout, which carries no API status",
			applyErr:         errors.Wrap(context.DeadlineExceeded, "timeout-detail-sentinel"),
			expectedCategory: "Timeout",
			expectedLevel:    log.ErrorLevel,
		},
		{
			name:             "unclassified",
			applyErr:         errors.New("unclassified-detail-sentinel"),
			expectedCategory: "Other",
			expectedLevel:    log.ErrorLevel,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			srcSecret := sourceSecret(scheme, "test-secret")
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(srcSecret).Build()
			r := &ReconcileClusterSync{Client: c}
			syncSet := secretRevisionSyncSet(scheme, 11, "test-secret")
			logger, hook := newSecretRevisionLogger()

			returnErr, requeue := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 0,
				func(_ []byte) (resource.ApplyResult, error) {
					return resource.ApplyResult(""), tc.applyErr
				}, logger)

			require.Error(t, returnErr)
			assert.Contains(t, returnErr.Error(), "failed to apply secret 0",
				"the original error wrapping is unchanged")
			assert.True(t, requeue, "an apply failure still requeues")

			failed := onlyEntryWithMessage(t, hook, msgFailedSourceRevision)
			assert.Equal(t, testSourceSecretUID+"-test-secret", failed.Data["sourceSecretUID"])
			assert.Equal(t, storedResourceVersion(t, c, "test-secret"), failed.Data["sourceSecretResourceVersion"])
			assert.Equal(t, int64(11), failed.Data["syncSetGeneration"])
			assert.Equal(t, tc.expectedCategory, failed.Data["applyErrorCategory"])
			assert.Equal(t, tc.expectedLevel, failed.Level)

			// The new line reports a category, never the error. The pre-existing
			// "error applying resource" line still carries the error itself, which this
			// change does not touch.
			for key, value := range failed.Data {
				assert.NotContains(t, fmt.Sprintf("%v", value), "sentinel",
					"field %q must not reproduce error text", key)
			}
			assert.Len(t, entriesWithMessage(hook, msgReadSourceRevision), 1,
				"the read was still observed")
			assert.Empty(t, entriesWithMessage(hook, msgAppliedSourceRevision),
				"a failed apply must never be reported as applied")
		})
	}
}

// TestApplySecret_SuccessPathDoesNotDiscloseSecretContent pins that nothing derived from the
// Secret's payload reaches the logs when the mapping succeeds. The failure path's pre-existing
// "error applying resource" line, which logs the error in full, is out of scope here.
func TestApplySecret_SuccessPathDoesNotDiscloseSecretContent(t *testing.T) {
	scheme := scheme.GetScheme()
	const (
		dataKeySentinel         = "data-key-sentinel-7f3a"
		dataValueSentinel       = "data-value-sentinel-9c1b"
		stringDataKeySentinel   = "string-data-key-sentinel-2e8d"
		stringDataValueSentinel = "string-data-value-sentinel-4b6f"
		annotationValueSentinel = "annotation-value-sentinel-1d5e"
		labelValueSentinel      = "label-value-sentinel-8a2c"
	)
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).
		GenericOptions(
			testgeneric.WithUID(testSourceSecretUID),
			testgeneric.WithAnnotation("test-annotation", annotationValueSentinel),
			testgeneric.WithLabel("test-label", labelValueSentinel),
		).
		Build(
			testsecret.WithDataKeyValue(dataKeySentinel, []byte(dataValueSentinel)),
			// StringData is write-only and a real API server never returns it, but a source
			// object that still carries it must not disclose it either.
			func(s *corev1.Secret) {
				s.StringData = map[string]string{stringDataKeySentinel: stringDataValueSentinel}
			},
		)
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(srcSecret).Build()
	r := &ReconcileClusterSync{Client: c}
	syncSet := secretRevisionSyncSet(scheme, 2, "test-secret")
	logger, hook := newSecretRevisionLogger()

	returnErr, _ := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 0, successfulApplyFn, logger)
	require.NoError(t, returnErr)
	require.NotEmpty(t, entriesWithMessage(hook, msgReadSourceRevision), "the revision was reported")
	require.NotEmpty(t, entriesWithMessage(hook, msgAppliedSourceRevision), "the outcome was reported")

	var sentinels []string
	for _, sentinel := range []string{
		dataKeySentinel, dataValueSentinel,
		stringDataKeySentinel, stringDataValueSentinel,
		annotationValueSentinel, labelValueSentinel,
	} {
		// Secret values are base64-encoded wherever the object is serialized as JSON, so a
		// plain substring scan would miss a leak of exactly the thing that matters most.
		sentinels = append(sentinels, sentinel, base64.StdEncoding.EncodeToString([]byte(sentinel)))
	}
	for _, rendered := range renderEntries(hook) {
		for _, sentinel := range sentinels {
			assert.False(t, strings.Contains(rendered, sentinel),
				"emitted log %q discloses %q", rendered, sentinel)
		}
	}
}

// TestApplySecret_SequentialMappingsDoNotShareContext pins that the revision captured for one
// mapping is not carried into the next one. The detector is a mapping that fails before it can
// capture anything: its line must show no revision fields at all. Asserting only on two
// successful mappings would be a tautology, because logrus snapshots an entry's fields when it
// is emitted.
func TestApplySecret_SequentialMappingsDoNotShareContext(t *testing.T) {
	scheme := scheme.GetScheme()
	first := sourceSecret(scheme, "test-secret-0")
	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(first).Build()
	r := &ReconcileClusterSync{Client: c}
	// Only the first mapping's source exists; the second cannot be read.
	syncSet := secretRevisionSyncSet(scheme, 5, "test-secret-0", "test-secret-1")
	logger, hook := newSecretRevisionLogger()

	// The caller's logger is reused across mappings, exactly as applySyncSet reuses it.
	firstErr, _ := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 0, successfulApplyFn, logger)
	require.NoError(t, firstErr)
	secondErr, _ := applySecretForMapping(r, (*SyncSetAsCommon)(syncSet), 1, successfulApplyFn, logger)
	require.Error(t, secondErr)

	read := onlyEntryWithMessage(t, hook, msgReadSourceRevision)
	assert.Equal(t, "test-secret-0", read.Data["sourceSecretName"])
	assert.Equal(t, testSourceSecretUID+"-test-secret-0", read.Data["sourceSecretUID"])

	cannotRead := onlyEntryWithMessage(t, hook, msgCannotReadSecret)
	for _, leaked := range []string{
		"sourceSecretUID", "sourceSecretResourceVersion",
		"sourceSecretNamespace", "sourceSecretName", "syncSetGeneration",
	} {
		assert.NotContains(t, cannotRead.Data, leaked,
			"the first mapping's context must not reach the second mapping's events")
	}
}

func TestApplyErrorCategory(t *testing.T) {
	groupResource := schema.GroupResource{Resource: "secrets"}
	cases := []struct {
		name     string
		err      error
		expected string
	}{
		{name: "nil", err: nil, expected: "None"},
		{name: "not found", err: apierrors.NewNotFound(groupResource, "x"), expected: "NotFound"},
		{name: "already exists", err: apierrors.NewAlreadyExists(groupResource, "x"), expected: "AlreadyExists"},
		{name: "conflict", err: apierrors.NewConflict(groupResource, "x", errors.New("boom")), expected: "Conflict"},
		{name: "invalid", err: apierrors.NewInvalid(schema.GroupKind{Kind: "Secret"}, "x", nil), expected: "Invalid"},
		{name: "forbidden", err: apierrors.NewForbidden(groupResource, "x", errors.New("boom")), expected: "Forbidden"},
		{name: "unauthorized", err: apierrors.NewUnauthorized("nope"), expected: "Unauthorized"},
		{name: "too many requests", err: apierrors.NewTooManyRequests("slow down", 1), expected: "TooManyRequests"},
		{name: "server timeout", err: apierrors.NewServerTimeout(groupResource, "apply", 1), expected: "Timeout"},
		{name: "api timeout", err: apierrors.NewTimeoutError("too slow", 1), expected: "Timeout"},
		{name: "context deadline", err: errors.Wrap(context.DeadlineExceeded, "wrapped"), expected: "Timeout"},
		{
			name:     "network timeout",
			err:      errors.Wrap(&net.DNSError{Err: "i/o timeout", IsTimeout: true}, "wrapped"),
			expected: "Timeout",
		},
		{name: "service unavailable", err: apierrors.NewServiceUnavailable("down"), expected: "ServerError"},
		{name: "internal error", err: apierrors.NewInternalError(errors.New("boom")), expected: "ServerError"},
		{name: "unclassified", err: errors.New("boom"), expected: "Other"},
		{
			name:     "non-timeout network error",
			err:      &net.DNSError{Err: "no such host"},
			expected: "Other",
		},
		{
			name:     "wrapped conflict",
			err:      errors.Wrap(apierrors.NewConflict(groupResource, "x", errors.New("boom")), "wrapped"),
			expected: "Conflict",
		},
		{
			name:     "cancellation is not a timeout",
			err:      errors.Wrap(context.Canceled, "wrapped"),
			expected: "Other",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, applyErrorCategory(tc.err))
		})
	}
	// Guard against the table drifting away from the deadline the timeout branch is written for.
	require.Equal(t, "Timeout", applyErrorCategory(&net.OpError{Err: timeoutError{}}))
}

// timeoutError is a minimal net.Error that reports a timeout, used to pin the branch that
// recognizes a transport timeout with no Kubernetes status attached.
type timeoutError struct{}

func (timeoutError) Error() string   { return "timed out" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
