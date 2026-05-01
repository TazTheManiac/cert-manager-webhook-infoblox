package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	ibclient "github.com/infobloxopen/infoblox-go-client/v2"
)

// ────────────────────────────────────────────────────────────────────────────
// mockIBConnector — minimal IBConnector implementation for testing.
// ────────────────────────────────────────────────────────────────────────────

// mockIBConnector implements ibclient.IBConnector in-memory so unit tests
// never need a real Infoblox server.
type mockIBConnector struct {
	// records stores created TXT records keyed by a "name|text|view" composite.
	records map[string]string // composite key → WAPI ref

	// errors allows individual operations to be forced to fail.
	getErr    error
	createErr error
	deleteErr error
}

func newMockIBConnector() *mockIBConnector {
	return &mockIBConnector{records: make(map[string]string)}
}

func recordKey(name, text, view string) string {
	return name + "|" + text + "|" + view
}

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetObject simulates querying for TXT records.  The search criteria are read
// directly from the RecordTXT object fields (Name, Text, View), which
// getTXTRecord sets on the obj alongside QueryParams so that both the real API
// and this mock can locate the right record.
func (m *mockIBConnector) GetObject(obj ibclient.IBObject, ref string, queryParams *ibclient.QueryParams, res interface{}) error {
	if m.getErr != nil {
		return m.getErr
	}

	out, _ := res.(*[]ibclient.RecordTXT)
	if out == nil {
		return nil
	}

	rec, ok := obj.(*ibclient.RecordTXT)
	if !ok {
		*out = []ibclient.RecordTXT{}
		return nil
	}

	key := recordKey(derefStr(rec.Name), derefStr(rec.Text), derefStr(rec.View))

	if wapiRef, found := m.records[key]; found {
		*out = []ibclient.RecordTXT{{Ref: wapiRef}}
	} else {
		*out = []ibclient.RecordTXT{}
		return ibclient.NewNotFoundError("not found")
	}

	return nil
}

// CreateObject stores a new TXT record and returns a synthetic WAPI ref.
func (m *mockIBConnector) CreateObject(obj ibclient.IBObject) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}

	rec, ok := obj.(*ibclient.RecordTXT)
	if !ok {
		return "", nil
	}

	key := recordKey(derefStr(rec.Name), derefStr(rec.Text), derefStr(rec.View))
	ref := "record:txt/" + key
	m.records[key] = ref
	return ref, nil
}

// DeleteObject removes the record identified by ref.
func (m *mockIBConnector) DeleteObject(ref string) (string, error) {
	if m.deleteErr != nil {
		return "", m.deleteErr
	}

	for k, v := range m.records {
		if v == ref {
			delete(m.records, k)
			return ref, nil
		}
	}

	return "", ibclient.NewNotFoundError("ref not found: " + ref)
}

// UpdateObject is not used by the webhook but is required by the interface.
func (m *mockIBConnector) UpdateObject(_ ibclient.IBObject, _ string) (string, error) {
	return "", nil
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: Name
// ────────────────────────────────────────────────────────────────────────────

func TestName(t *testing.T) {
	s := &infobloxDNSSolver{}
	assert.Equal(t, "infoblox-wapi", s.Name())
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: trimTrailingDot
// ────────────────────────────────────────────────────────────────────────────

func TestTrimTrailingDot(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com.", "example.com"},
		{"example.com", "example.com"},
		{"_acme-challenge.example.com.", "_acme-challenge.example.com"},
		{"sub.domain.example.com.", "sub.domain.example.com"},
		{"", ""},
		{".", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, trimTrailingDot(tt.input))
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: loadConfig / applyDefaults
// ────────────────────────────────────────────────────────────────────────────

func TestLoadConfig_Nil(t *testing.T) {
	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	// All fields should receive defaults.
	assert.Equal(t, defaultPort, cfg.Port)
	assert.Equal(t, defaultVersion, cfg.Version)
	assert.Equal(t, defaultHTTPRequestTimeout, cfg.HTTPRequestTimeout)
	assert.Equal(t, defaultHTTPPoolConnections, cfg.HTTPPoolConnections)
	assert.Equal(t, defaultTTL, cfg.TTL)
}

func TestLoadConfig_Empty(t *testing.T) {
	raw := apiextensionsv1.JSON{Raw: []byte("{}")}
	cfg, err := loadConfig(&raw)
	require.NoError(t, err)
	assert.Equal(t, defaultPort, cfg.Port)
}

func TestLoadConfig_Valid(t *testing.T) {
	js := `{
		"host":                "infoblox.example.com",
		"port":                "8443",
		"version":             "2.11",
		"view":                "External",
		"sslVerify":           true,
		"httpRequestTimeout":  90,
		"httpPoolConnections": 20,
		"ttl":                 600,
		"useTtl":              true
	}`
	raw := apiextensionsv1.JSON{Raw: []byte(js)}
	cfg, err := loadConfig(&raw)
	require.NoError(t, err)

	assert.Equal(t, "infoblox.example.com", cfg.Host)
	assert.Equal(t, "8443", cfg.Port)
	assert.Equal(t, "2.11", cfg.Version)
	assert.Equal(t, "External", cfg.View)
	assert.True(t, cfg.SslVerify)
	assert.Equal(t, 90, cfg.HTTPRequestTimeout)
	assert.Equal(t, 20, cfg.HTTPPoolConnections)
	assert.Equal(t, uint32(600), cfg.TTL)
	assert.True(t, cfg.UseTTL)
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	raw := apiextensionsv1.JSON{Raw: []byte("not json")}
	_, err := loadConfig(&raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error decoding solver config")
}

func TestApplyDefaults_AllEmpty(t *testing.T) {
	cfg := infobloxConfig{}
	applyDefaults(&cfg)
	assert.Equal(t, defaultPort, cfg.Port)
	assert.Equal(t, defaultVersion, cfg.Version)
	assert.Equal(t, defaultHTTPRequestTimeout, cfg.HTTPRequestTimeout)
	assert.Equal(t, defaultHTTPPoolConnections, cfg.HTTPPoolConnections)
	assert.Equal(t, defaultTTL, cfg.TTL)
}

func TestApplyDefaults_PreservesValues(t *testing.T) {
	cfg := infobloxConfig{
		Port:                "8443",
		Version:             "2.12",
		HTTPRequestTimeout:  120,
		HTTPPoolConnections: 25,
		TTL:                 900,
	}
	applyDefaults(&cfg)
	assert.Equal(t, "8443", cfg.Port)
	assert.Equal(t, "2.12", cfg.Version)
	assert.Equal(t, 120, cfg.HTTPRequestTimeout)
	assert.Equal(t, 25, cfg.HTTPPoolConnections)
	assert.Equal(t, uint32(900), cfg.TTL)
}

func TestApplyDefaults_NegativeTimeoutGetsDefault(t *testing.T) {
	cfg := infobloxConfig{HTTPRequestTimeout: -5}
	applyDefaults(&cfg)
	assert.Equal(t, defaultHTTPRequestTimeout, cfg.HTTPRequestTimeout)
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: getSecretValue
// ────────────────────────────────────────────────────────────────────────────

func makeTestSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

func TestGetSecretValue_Success(t *testing.T) {
	secret := makeTestSecret("my-secret", "test-ns", map[string][]byte{
		"username": []byte("admin"),
		"password": []byte("s3cr3t\n"), // trailing newline should be stripped
	})

	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset(secret)}

	user, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "username",
	}, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, "admin", user)

	pass, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "password",
	}, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", pass) // newline stripped
}

func TestGetSecretValue_SecretNotFound(t *testing.T) {
	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset()}

	_, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "missing"},
		Key:                  "username",
	}, "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot get secret")
}

func TestGetSecretValue_KeyNotFound(t *testing.T) {
	secret := makeTestSecret("my-secret", "test-ns", map[string][]byte{
		"username": []byte("admin"),
	})
	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset(secret)}

	_, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "nonexistent",
	}, "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in secret")
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: resolveCredentials
// ────────────────────────────────────────────────────────────────────────────

func TestResolveCredentials_NoConfig(t *testing.T) {
	solver := &infobloxDNSSolver{}
	cfg := &infobloxConfig{}

	_, _, err := solver.resolveCredentials(cfg, "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no credentials configured")
}

func TestResolveCredentials_SecretRef(t *testing.T) {
	secret := makeTestSecret("ib-creds", "test-ns", map[string][]byte{
		"username": []byte("secretuser"),
		"password": []byte("secretpass"),
	})
	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset(secret)}

	cfg := &infobloxConfig{
		UsernameSecretRef: cmmeta.SecretKeySelector{
			LocalObjectReference: cmmeta.LocalObjectReference{Name: "ib-creds"},
			Key:                  "username",
		},
		PasswordSecretRef: cmmeta.SecretKeySelector{
			LocalObjectReference: cmmeta.LocalObjectReference{Name: "ib-creds"},
			Key:                  "password",
		},
	}

	user, pass, err := solver.resolveCredentials(cfg, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, "secretuser", user)
	assert.Equal(t, "secretpass", pass)
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: getTXTRecord / createTXTRecord / deleteTXTRecord  (using mock)
// ────────────────────────────────────────────────────────────────────────────

func TestGetTXTRecord_NotFound(t *testing.T) {
	mock := newMockIBConnector()

	ref, err := getTXTRecord(mock, "_acme-challenge.example.com", "tokenvalue", "default")
	require.NoError(t, err)
	assert.Empty(t, ref)
}

func TestCreateAndGetTXTRecord(t *testing.T) {
	mock := newMockIBConnector()

	name := "_acme-challenge.example.com"
	text := "challengetoken"
	view := "default"

	ref, err := createTXTRecord(mock, name, text, view, 300, true)
	require.NoError(t, err)
	assert.NotEmpty(t, ref)

	found, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.Equal(t, ref, found)
}

func TestDeleteTXTRecord_Success(t *testing.T) {
	mock := newMockIBConnector()

	name := "_acme-challenge.example.com"
	text := "tokenvalue"
	view := "default"

	ref, err := createTXTRecord(mock, name, text, view, 300, true)
	require.NoError(t, err)

	err = deleteTXTRecord(mock, ref)
	require.NoError(t, err)

	// Record should no longer exist.
	found, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestDeleteTXTRecord_RefNotFound(t *testing.T) {
	mock := newMockIBConnector()
	err := deleteTXTRecord(mock, "record:txt/nonexistent")
	require.Error(t, err)
}

func TestCreateTXTRecord_Error(t *testing.T) {
	mock := newMockIBConnector()
	mock.createErr = ibclient.NewNotFoundError("forced error")

	_, err := createTXTRecord(mock, "example.com", "val", "default", 300, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CreateObject failed")
}

func TestGetTXTRecord_Error(t *testing.T) {
	mock := newMockIBConnector()
	mock.getErr = assert.AnError // not a NotFoundError, so should propagate

	_, err := getTXTRecord(mock, "example.com", "val", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetObject failed")
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: Present / CleanUp end-to-end (using fake k8s client + mock IB)
// ────────────────────────────────────────────────────────────────────────────

// presentSolverWithMock creates a solver pre-wired with a fake k8s client and
// replaces newIBConnector with one that always returns the provided mock.
// Because newIBConnector reaches out to a real Infoblox host, we test Present/
// CleanUp by calling the lower-level functions directly and verify behaviour
// through the mock rather than calling Present/CleanUp end-to-end (which would
// require injecting the mock IB connector into the solver – a good future
// refactor target).
func TestPresent_EndToEnd_UsingLowLevelFunctions(t *testing.T) {
	mock := newMockIBConnector()

	name := "_acme-challenge.sub.example.com"
	text := "acme-token-xyz"
	view := "Internal"

	// Simulate what Present() does with the mock:
	// 1. Check no existing record.
	ref, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.Empty(t, ref)

	// 2. Create record.
	ref, err = createTXTRecord(mock, name, text, view, 300, true)
	require.NoError(t, err)
	assert.NotEmpty(t, ref)

	// 3. Re-calling Present would delete the existing record first.
	existingRef, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.Equal(t, ref, existingRef)

	err = deleteTXTRecord(mock, existingRef)
	require.NoError(t, err)

	newRef, err := createTXTRecord(mock, name, text, view, 300, true)
	require.NoError(t, err)
	assert.NotEmpty(t, newRef)
}

func TestCleanUp_EndToEnd_UsingLowLevelFunctions(t *testing.T) {
	mock := newMockIBConnector()

	name := "_acme-challenge.example.com"
	text := "acme-token-cleanup"
	view := "default"

	ref, err := createTXTRecord(mock, name, text, view, 300, true)
	require.NoError(t, err)
	assert.NotEmpty(t, ref)

	// Simulate CleanUp:
	found, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.NotEmpty(t, found)

	err = deleteTXTRecord(mock, found)
	require.NoError(t, err)

	// Record should be gone.
	gone, err := getTXTRecord(mock, name, text, view)
	require.NoError(t, err)
	assert.Empty(t, gone)
}

func TestCleanUp_RecordAlreadyGone(t *testing.T) {
	mock := newMockIBConnector()

	// Simulate CleanUp when no record exists (should be a no-op).
	found, err := getTXTRecord(mock, "_acme-challenge.example.com", "token", "default")
	require.NoError(t, err)
	assert.Empty(t, found)
	// Nothing to delete → no error expected.
}

// ────────────────────────────────────────────────────────────────────────────
// Benchmarks
// ────────────────────────────────────────────────────────────────────────────

func BenchmarkTrimTrailingDot(b *testing.B) {
	for i := 0; i < b.N; i++ {
		trimTrailingDot("_acme-challenge.sub.example.com.")
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	raw := apiextensionsv1.JSON{Raw: []byte(`{"host":"ib.local","view":"default"}`)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loadConfig(&raw)
	}
}

func BenchmarkApplyDefaults(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := infobloxConfig{}
		applyDefaults(&cfg)
	}
}
