// Package main implements a cert-manager ACME DNS01 webhook for Infoblox WAPI.
//
// This webhook enables cert-manager to use an Infoblox GRID as the DNS provider
// for ACME DNS01 challenge solving.  It communicates with Infoblox through the
// official infoblox-go-client library (WAPI v2).
//
// Configuration is provided via the ClusterIssuer/Issuer webhook config stanza.
// Credentials can be supplied either as a Kubernetes Secret reference or as a
// JSON file mounted into the pod.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	ibclient "github.com/infobloxopen/infoblox-go-client/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	webhook "github.com/cert-manager/cert-manager/pkg/acme/webhook"
	whapi "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
)

const (
	// solverName is the unique identifier for this DNS solver within the group.
	solverName = "infoblox-wapi"

	// default configuration values.
	defaultPort                = "443"
	defaultVersion             = "2.10"
	defaultHTTPRequestTimeout  = 60
	defaultHTTPPoolConnections = 10
	defaultTTL                 = uint32(300)
)

// GroupName is the API group advertised by this webhook.  It is injected at
// runtime via the GROUP_NAME environment variable so that each operator can
// use their own domain-scoped group name.
var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME environment variable must be set")
	}

	klog.InfoS("Starting cert-manager Infoblox webhook", "groupName", GroupName)

	// Register our solver and start the webhook API server.
	cmd.RunWebhookServer(GroupName, &infobloxDNSSolver{})
}

// Compile-time assertion that infobloxDNSSolver satisfies the Solver interface.
var _ webhook.Solver = (*infobloxDNSSolver)(nil)

// infobloxDNSSolver implements the cert-manager DNS01 webhook Solver interface
// backed by Infoblox WAPI.
type infobloxDNSSolver struct {
	// kubeClient is initialised by Initialize() and used to read Secret
	// resources that hold Infoblox credentials.
	kubeClient kubernetes.Interface
}

// infobloxConfig is decoded from the webhook's config JSON (the
// `issuer.spec.acme.dns01.providers.webhook.config` field).
type infobloxConfig struct {
	// Host is the FQDN or IP address of the Infoblox GRID member.
	Host string `json:"host"`
	// Port is the HTTPS port (default: 443).
	Port string `json:"port"`
	// Version is the WAPI version to target (default: "2.10").
	Version string `json:"version"`
	// View is the DNS view that contains the zone to be managed.
	View string `json:"view"`
	// SslVerify enables TLS certificate verification when calling the WAPI.
	SslVerify bool `json:"sslVerify"`
	// HTTPRequestTimeout is the per-request timeout in seconds (default: 60).
	HTTPRequestTimeout int `json:"httpRequestTimeout"`
	// HTTPPoolConnections is the maximum number of idle connections to the
	// Infoblox server (default: 10).
	HTTPPoolConnections int `json:"httpPoolConnections"`
	// UsernameSecretRef references the Secret key containing the Infoblox
	// username.
	UsernameSecretRef cmmeta.SecretKeySelector `json:"usernameSecretRef"`
	// PasswordSecretRef references the Secret key containing the Infoblox
	// password.
	PasswordSecretRef cmmeta.SecretKeySelector `json:"passwordSecretRef"`
	// TTL is the time-to-live value set on created TXT records (default: 300).
	TTL uint32 `json:"ttl"`
	// UseTTL controls whether the TTL field is set on TXT records.
	UseTTL bool `json:"useTtl"`
}

// ────────────────────────────────────────────────────────────────────────────
// Solver interface implementation
// ────────────────────────────────────────────────────────────────────────────

// Name returns the solver identifier referenced in Issuer / ClusterIssuer
// resources.
func (s *infobloxDNSSolver) Name() string {
	return solverName
}

// Initialize is called once when the webhook starts.  It builds a Kubernetes
// client from the provided in-cluster config so that the solver can read
// Secret resources later.
func (s *infobloxDNSSolver) Initialize(kubeClientConfig *rest.Config, _ <-chan struct{}) error {
	klog.V(4).InfoS("Initializing Infoblox webhook solver")

	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	s.kubeClient = cl
	klog.V(4).InfoS("Infoblox webhook solver initialized successfully")
	return nil
}

// Present creates (or re-creates) the DNS TXT record required by the ACME
// DNS01 challenge.  The method is idempotent: if a matching record already
// exists it is deleted before a fresh one is created to guarantee the correct
// challenge value is present.
func (s *infobloxDNSSolver) Present(ch *whapi.ChallengeRequest) error {
	klog.InfoS("Present called", "dnsName", ch.DNSName, "resolvedFQDN", ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("failed to load solver config: %w", err)
	}

	ib, err := s.newIBConnector(&cfg, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("failed to create Infoblox connector: %w", err)
	}

	recordName := trimTrailingDot(ch.ResolvedFQDN)

	// Idempotency: remove any pre-existing TXT record with this name/value
	// before creating the fresh one.
	ref, err := getTXTRecord(ib, recordName, ch.Key, cfg.View)
	if err != nil {
		return fmt.Errorf("error checking for existing TXT record %q: %w", recordName, err)
	}

	if ref != "" {
		klog.V(4).InfoS("Removing existing TXT record before re-creating", "ref", ref)
		if err := deleteTXTRecord(ib, ref); err != nil {
			return fmt.Errorf("failed to delete existing TXT record %q: %w", recordName, err)
		}
	}

	ref, err = createTXTRecord(ib, recordName, ch.Key, cfg.View, cfg.TTL, cfg.UseTTL)
	if err != nil {
		return fmt.Errorf("failed to create TXT record %q: %w", recordName, err)
	}

	klog.InfoS("TXT record created", "name", recordName, "ref", ref)
	return nil
}

// CleanUp deletes the TXT record that was created by Present.  If the record
// has already been removed (e.g. by a previous call) the method returns nil.
func (s *infobloxDNSSolver) CleanUp(ch *whapi.ChallengeRequest) error {
	klog.InfoS("CleanUp called", "dnsName", ch.DNSName, "resolvedFQDN", ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("failed to load solver config: %w", err)
	}

	ib, err := s.newIBConnector(&cfg, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("failed to create Infoblox connector: %w", err)
	}

	recordName := trimTrailingDot(ch.ResolvedFQDN)

	ref, err := getTXTRecord(ib, recordName, ch.Key, cfg.View)
	if err != nil {
		return fmt.Errorf("error looking up TXT record %q: %w", recordName, err)
	}

	if ref == "" {
		klog.V(4).InfoS("TXT record not found, nothing to clean up", "name", recordName)
		return nil
	}

	if err := deleteTXTRecord(ib, ref); err != nil {
		return fmt.Errorf("failed to delete TXT record %q (ref=%s): %w", recordName, ref, err)
	}

	klog.InfoS("TXT record deleted", "name", recordName, "ref", ref)
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// Configuration helpers
// ────────────────────────────────────────────────────────────────────────────

// loadConfig deserialises the raw JSON config blob from the ChallengeRequest
// and fills in default values for any omitted fields.
func loadConfig(cfgJSON *apiextensionsv1.JSON) (infobloxConfig, error) {
	cfg := infobloxConfig{}

	if cfgJSON == nil {
		applyDefaults(&cfg)
		return cfg, nil
	}

	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %w", err)
	}

	applyDefaults(&cfg)
	return cfg, nil
}

// applyDefaults fills in sensible defaults for any zero-value configuration
// fields.
func applyDefaults(cfg *infobloxConfig) {
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	if cfg.Version == "" {
		cfg.Version = defaultVersion
	}
	if cfg.HTTPRequestTimeout <= 0 {
		cfg.HTTPRequestTimeout = defaultHTTPRequestTimeout
	}
	if cfg.HTTPPoolConnections <= 0 {
		cfg.HTTPPoolConnections = defaultHTTPPoolConnections
	}
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Infoblox connector factory
// ────────────────────────────────────────────────────────────────────────────

// newIBConnector resolves credentials from either a Kubernetes Secret or a
// mounted JSON file, then returns an initialised Infoblox WAPI connector.
func (s *infobloxDNSSolver) newIBConnector(cfg *infobloxConfig, namespace string) (ibclient.IBConnector, error) {
	username, password, err := s.resolveCredentials(cfg, namespace)
	if err != nil {
		return nil, err
	}

	hostCfg := ibclient.HostConfig{
		Host:    cfg.Host,
		Version: cfg.Version,
		Port:    cfg.Port,
	}

	authCfg := ibclient.AuthConfig{
		Username: username,
		Password: password,
	}

	transportCfg := ibclient.NewTransportConfig(
		strconv.FormatBool(cfg.SslVerify),
		cfg.HTTPRequestTimeout,
		cfg.HTTPPoolConnections,
	)

	rb := &ibclient.WapiRequestBuilder{}
	rq := &ibclient.WapiHttpRequestor{}

	connector, err := ibclient.NewConnector(hostCfg, authCfg, transportCfg, rb, rq)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise Infoblox connector: %w", err)
	}

	return connector, nil
}

// resolveCredentials returns the Infoblox username and password from the
// Kubernetes Secrets referenced in cfg.
func (s *infobloxDNSSolver) resolveCredentials(cfg *infobloxConfig, namespace string) (string, string, error) {
	if cfg.UsernameSecretRef.Key == "" || cfg.PasswordSecretRef.Key == "" {
		return "", "", errors.New(
			"no credentials configured: set usernameSecretRef and passwordSecretRef",
		)
	}

	return s.credentialsFromSecret(cfg, namespace)
}

// credentialsFromSecret reads the username and password from the Kubernetes
// Secrets referenced in cfg.
func (s *infobloxDNSSolver) credentialsFromSecret(cfg *infobloxConfig, namespace string) (string, string, error) {
	username, err := s.getSecretValue(cfg.UsernameSecretRef, namespace)
	if err != nil {
		return "", "", fmt.Errorf("failed to read username from secret: %w", err)
	}

	password, err := s.getSecretValue(cfg.PasswordSecretRef, namespace)
	if err != nil {
		return "", "", fmt.Errorf("failed to read password from secret: %w", err)
	}

	return username, password, nil
}

// getSecretValue fetches a single key from a Kubernetes Secret.
func (s *infobloxDNSSolver) getSecretValue(sel cmmeta.SecretKeySelector, namespace string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	secret, err := s.kubeClient.CoreV1().Secrets(namespace).Get(ctx, sel.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot get secret %s/%s: %w", namespace, sel.Name, err)
	}

	raw, ok := secret.Data[sel.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", sel.Key, namespace, sel.Name)
	}

	// Trim a trailing newline that is commonly present when secrets are created
	// from files (e.g. `kubectl create secret generic ... --from-file=...`).
	return strings.TrimSuffix(string(raw), "\n"), nil
}

// ────────────────────────────────────────────────────────────────────────────
// Infoblox WAPI record operations
// ────────────────────────────────────────────────────────────────────────────

// getTXTRecord searches Infoblox for a TXT record with the given name, text
// content and DNS view.  It returns the WAPI object reference if found, or an
// empty string if the record does not exist.
func getTXTRecord(ib ibclient.IBConnector, name, text, view string) (string, error) {
	var records []ibclient.RecordTXT

	// Set fields on the obj so that both the real API (via QueryParams) and
	// test mocks (which read obj fields directly) can match the record.
	obj := ibclient.NewEmptyRecordTXT()
	obj.Name = &name
	obj.Text = &text
	if view != "" {
		obj.View = &view
	}

	// QueryParams carry the search criteria for the real Infoblox WAPI.
	params := ibclient.NewQueryParams(false, map[string]string{
		"name": name,
		"text": text,
		"view": view,
	})

	err := ib.GetObject(obj, "", params, &records)

	if len(records) > 0 {
		klog.V(4).InfoS("Found existing TXT record", "name", name, "ref", records[0].Ref)
		return records[0].Ref, nil
	}

	// A NotFoundError is expected when the record does not yet exist; treat it
	// as a successful "not found" result rather than a hard error.
	var notFound *ibclient.NotFoundError
	if errors.As(err, &notFound) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("GetObject failed for TXT record %q: %w", name, err)
	}

	return "", nil
}

// createTXTRecord creates a new TXT record in Infoblox and returns its
// WAPI object reference.
func createTXTRecord(ib ibclient.IBConnector, name, text, view string, ttl uint32, useTTL bool) (string, error) {
	obj := ibclient.NewRecordTXT(view, "", name, text, ttl, useTTL, "", nil)

	ref, err := ib.CreateObject(obj)
	if err != nil {
		return "", fmt.Errorf("CreateObject failed for TXT record %q: %w", name, err)
	}

	return ref, nil
}

// deleteTXTRecord deletes the Infoblox record identified by ref.
func deleteTXTRecord(ib ibclient.IBConnector, ref string) error {
	if _, err := ib.DeleteObject(ref); err != nil {
		return fmt.Errorf("DeleteObject failed for ref %q: %w", ref, err)
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// Utility
// ────────────────────────────────────────────────────────────────────────────

// trimTrailingDot strips the trailing dot from an FQDN as required by the
// Infoblox WAPI (which expects bare hostnames, not RFC 1035 absolute names).
func trimTrailingDot(fqdn string) string {
	return strings.TrimSuffix(fqdn, ".")
}
