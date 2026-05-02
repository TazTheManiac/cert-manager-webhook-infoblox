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
)

// ────────────────────────────────────────────────────────────────────────────
// Tests: Name
// ────────────────────────────────────────────────────────────────────────────

func TestName(t *testing.T) {
	s := &infobloxDNSSolver{}
	got := s.Name()
	t.Logf("solver name = %q", got)
	assert.Equal(t, "infoblox", got)
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
			got := trimTrailingDot(tt.input)
			t.Logf("trimTrailingDot(%q) = %q (want %q)", tt.input, got, tt.expected)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: loadConfig / applyDefaults
// ────────────────────────────────────────────────────────────────────────────

func TestLoadConfig_Nil(t *testing.T) {
	t.Log("loading config from nil, expecting all defaults to be applied")
	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	t.Logf("port=%s version=%s httpRequestTimeout=%d httpPoolConnections=%d ttl=%d",
		cfg.Port, cfg.Version, cfg.HTTPRequestTimeout, cfg.HTTPPoolConnections, cfg.TTL)
	assert.Equal(t, defaultPort, cfg.Port)
	assert.Equal(t, defaultVersion, cfg.Version)
	assert.Equal(t, defaultHTTPRequestTimeout, cfg.HTTPRequestTimeout)
	assert.Equal(t, defaultHTTPPoolConnections, cfg.HTTPPoolConnections)
	assert.Equal(t, defaultTTL, cfg.TTL)
}

func TestLoadConfig_Empty(t *testing.T) {
	t.Log("loading config from empty JSON object, expecting defaults to be applied")
	raw := apiextensionsv1.JSON{Raw: []byte("{}")}
	cfg, err := loadConfig(&raw)
	require.NoError(t, err)
	t.Logf("port=%s (default=%s)", cfg.Port, defaultPort)
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
	t.Log("loading config from fully-populated JSON")
	cfg, err := loadConfig(&raw)
	require.NoError(t, err)
	t.Logf("host=%s port=%s version=%s view=%s sslVerify=%v httpRequestTimeout=%d httpPoolConnections=%d ttl=%d useTtl=%v",
		cfg.Host, cfg.Port, cfg.Version, cfg.View, cfg.SslVerify, cfg.HTTPRequestTimeout, cfg.HTTPPoolConnections, cfg.TTL, cfg.UseTTL)

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
	t.Log("loading config from invalid JSON, expecting a decode error")
	raw := apiextensionsv1.JSON{Raw: []byte("not json")}
	_, err := loadConfig(&raw)
	require.Error(t, err)
	t.Logf("error received: %v", err)
	assert.Contains(t, err.Error(), "error decoding solver config")
}

func TestApplyDefaults_AllEmpty(t *testing.T) {
	t.Log("applying defaults to zero-value config, all fields should be filled in")
	cfg := infobloxConfig{}
	applyDefaults(&cfg)
	t.Logf("port=%s version=%s httpRequestTimeout=%d httpPoolConnections=%d ttl=%d",
		cfg.Port, cfg.Version, cfg.HTTPRequestTimeout, cfg.HTTPPoolConnections, cfg.TTL)
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
	t.Logf("input: port=%s version=%s httpRequestTimeout=%d httpPoolConnections=%d ttl=%d",
		cfg.Port, cfg.Version, cfg.HTTPRequestTimeout, cfg.HTTPPoolConnections, cfg.TTL)
	applyDefaults(&cfg)
	t.Logf("after applyDefaults: port=%s version=%s httpRequestTimeout=%d httpPoolConnections=%d ttl=%d",
		cfg.Port, cfg.Version, cfg.HTTPRequestTimeout, cfg.HTTPPoolConnections, cfg.TTL)
	assert.Equal(t, "8443", cfg.Port)
	assert.Equal(t, "2.12", cfg.Version)
	assert.Equal(t, 120, cfg.HTTPRequestTimeout)
	assert.Equal(t, 25, cfg.HTTPPoolConnections)
	assert.Equal(t, uint32(900), cfg.TTL)
}

func TestApplyDefaults_NegativeTimeoutGetsDefault(t *testing.T) {
	cfg := infobloxConfig{HTTPRequestTimeout: -5}
	t.Logf("input: httpRequestTimeout=%d (negative, should be replaced with default)", cfg.HTTPRequestTimeout)
	applyDefaults(&cfg)
	t.Logf("after applyDefaults: httpRequestTimeout=%d (default=%d)", cfg.HTTPRequestTimeout, defaultHTTPRequestTimeout)
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

	t.Logf("looking up key %q from secret %q in namespace %q", "username", "my-secret", "test-ns")
	user, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "username",
	}, "test-ns")
	require.NoError(t, err)
	t.Logf("username = %q", user)
	assert.Equal(t, "admin", user)

	t.Logf("looking up key %q, raw value has trailing newline, should be stripped", "password")
	pass, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "password",
	}, "test-ns")
	require.NoError(t, err)
	t.Logf("password = %q (newline stripped)", pass)
	assert.Equal(t, "s3cr3t", pass)
}

func TestGetSecretValue_SecretNotFound(t *testing.T) {
	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset()}

	t.Logf("looking up key %q from non-existent secret %q, expecting error", "username", "missing")
	_, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "missing"},
		Key:                  "username",
	}, "test-ns")
	require.Error(t, err)
	t.Logf("error received: %v", err)
	assert.Contains(t, err.Error(), "cannot get secret")
}

func TestGetSecretValue_KeyNotFound(t *testing.T) {
	secret := makeTestSecret("my-secret", "test-ns", map[string][]byte{
		"username": []byte("admin"),
	})
	solver := &infobloxDNSSolver{kubeClient: fake.NewSimpleClientset(secret)}

	t.Logf("looking up non-existent key %q from secret %q, expecting error", "nonexistent", "my-secret")
	_, err := solver.getSecretValue(cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{Name: "my-secret"},
		Key:                  "nonexistent",
	}, "test-ns")
	require.Error(t, err)
	t.Logf("error received: %v", err)
	assert.Contains(t, err.Error(), "not found in secret")
}

// ────────────────────────────────────────────────────────────────────────────
// Tests: resolveCredentials
// ────────────────────────────────────────────────────────────────────────────

func TestResolveCredentials_NoConfig(t *testing.T) {
	solver := &infobloxDNSSolver{}
	cfg := &infobloxConfig{}

	t.Log("resolving credentials with empty config, expecting 'no credentials configured' error")
	_, _, err := solver.resolveCredentials(cfg, "test-ns")
	require.Error(t, err)
	t.Logf("error received: %v", err)
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

	t.Logf("resolving credentials from secret %q in namespace %q", "ib-creds", "test-ns")
	user, pass, err := solver.resolveCredentials(cfg, "test-ns")
	require.NoError(t, err)
	t.Logf("username = %q  password = %q", user, "***")
	assert.Equal(t, "secretuser", user)
	assert.Equal(t, "secretpass", pass)
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
