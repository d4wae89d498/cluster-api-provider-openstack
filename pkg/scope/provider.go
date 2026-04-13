/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scope

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	osclient "github.com/gophercloud/utils/v2/client"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	infrav1alpha1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha1"
	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/clients"
	"sigs.k8s.io/cluster-api-provider-openstack/version"
)

const (
	CloudsSecretKey = "clouds.yaml"
	CASecretKey     = "cacert"
)

// IdentityAccessDeniedError is returned when a namespace is not permitted to use a ClusterIdentity.
type IdentityAccessDeniedError struct {
	IdentityName       string
	RequesterNamespace string
}

func (e *IdentityAccessDeniedError) Error() string {
	return fmt.Sprintf("namespace %s not allowed to use cluster identity %s", e.RequesterNamespace, e.IdentityName)
}

type providerScopeFactory struct {
	clientCache *cache.LRUExpireCache
}

func (f *providerScopeFactory) NewClientScopeFromObject(ctx context.Context, ctrlClient client.Client, defaultCACert []byte, logger logr.Logger, objects ...infrav1.IdentityRefProvider) (Scope, error) {
	var namespace *string
	var identityRef *infrav1.OpenStackIdentityReference

	for _, o := range objects {
		namespace, identityRef = o.GetIdentityRef()
		if namespace != nil || identityRef != nil {
			break
		}
	}

	if namespace == nil || identityRef == nil {
		return nil, fmt.Errorf("unable to get identityRef from provided objects")
	}

	var cloud clientconfig.Cloud
	var caCert []byte

	// Determine which secret to read based on identity type
	var secretNamespace string
	var secretName string

	switch identityRef.Type {
	case "", "Secret":
		secretNamespace = *namespace
		secretName = identityRef.Name
		logger.V(4).Info("Using Secret for OpenStack credentials", "namespace", secretNamespace, "name", secretName, "cloudName", identityRef.CloudName)
	case "ClusterIdentity":
		// Fetch cluster-scoped identity and validate namespace access
		identity := &infrav1alpha1.OpenStackClusterIdentity{}
		if err := ctrlClient.Get(ctx, types.NamespacedName{Name: identityRef.Name}, identity); err != nil {
			return nil, fmt.Errorf("failed to get OpenStackClusterIdentity %s: %w", identityRef.Name, err)
		}
		// Validate selector (if any) against the caller namespace
		if identity.Spec.NamespaceSelector != nil {
			ns := &corev1.Namespace{}
			if err := ctrlClient.Get(ctx, types.NamespacedName{Name: *namespace}, ns); err != nil {
				return nil, fmt.Errorf("failed to get namespace %s: %w", *namespace, err)
			}
			selector, err := metav1.LabelSelectorAsSelector(identity.Spec.NamespaceSelector)
			if err != nil {
				return nil, fmt.Errorf("invalid namespace selector on identity %s: %w", identity.Name, err)
			}
			if !selector.Matches(labels.Set(ns.Labels)) {
				logger.V(2).Info("Namespace not allowed to use ClusterIdentity", "identity", identity.Name, "namespace", *namespace)
				return nil, &IdentityAccessDeniedError{IdentityName: identity.Name, RequesterNamespace: *namespace}
			}
		}
		secretNamespace = identity.Spec.SecretRef.Namespace
		secretName = identity.Spec.SecretRef.Name
		logger.V(4).Info("Using ClusterIdentity for OpenStack credentials", "identity", identityRef.Name, "secretNamespace", secretNamespace, "secretName", secretName, "cloudName", identityRef.CloudName)
	default:
		return nil, fmt.Errorf("unsupported identity type: %s", identityRef.Type)
	}

	// Read cloud from the resolved secret using the provided cloudName
	cloud, caCert, endpointOverrides, err := getCloudFromSecret(ctx, ctrlClient, secretNamespace, secretName, identityRef.CloudName, logger)
	if err != nil {
		return nil, err
	}

	if caCert == nil {
		caCert = defaultCACert
	}

	if f.clientCache == nil {
		return NewProviderScope(cloud, identityRef.Region, caCert, endpointOverrides, logger)
	}

	return NewCachedProviderScope(f.clientCache, cloud, identityRef.Region, caCert, endpointOverrides, logger)
}

// getScopeCacheKey returns a cache key that uniquely identifies a provider
// scope.  It hashes both the cloud configuration and the endpoint overrides so
// that changing endpoints.yaml always produces a new scope (and the cached
// scope is not returned with stale — or absent — endpoint overrides).
func getScopeCacheKey(cloud clientconfig.Cloud, endpointOverrides *EndpointOverrides) (string, error) {
	cloudKey, err := computeSpewHash(cloud)
	if err != nil {
		return "", err
	}

	// Hash the overrides separately; a nil pointer and an empty struct must
	// produce the same key, so normalise to a concrete value first.
	var overrideVal EndpointOverrides
	if endpointOverrides != nil {
		overrideVal = *endpointOverrides
	}
	overrideKey, err := computeSpewHash(overrideVal)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d-%d", cloudKey, overrideKey), nil
}

type providerScope struct {
	providerClient     *gophercloud.ProviderClient
	providerClientOpts *clientconfig.ClientOpts
	projectID          string
	endpointOverrides  *EndpointOverrides
	cloudName          string
	regionName         string
	logger             logr.Logger
}

func NewProviderScope(cloud clientconfig.Cloud, regionName string, caCert []byte, endpointOverrides *EndpointOverrides, logger logr.Logger) (Scope, error) {
	providerClient, clientOpts, projectID, err := NewProviderClient(cloud, regionName, caCert, logger)
	if err != nil {
		return nil, err
	}

	// Determine the effective region for endpoint override lookup.
	effectiveRegion := regionName
	if effectiveRegion == "" {
		effectiveRegion = cloud.RegionName
	}

	hasOverrides := endpointOverrides != nil && len(endpointOverrides.Clouds) > 0
	logger.V(4).Info("Creating provider scope",
		"cloudName", cloud.Cloud,
		"regionName", effectiveRegion,
		"endpointOverridesPresent", hasOverrides,
	)
	if hasOverrides {
		logger.V(4).Info("Endpoint overrides: available regions for cloud",
			"cloudName", cloud.Cloud,
			"availableRegions", endpointOverrides.AvailableRegions(cloud.Cloud),
		)
	}

	return &providerScope{
		providerClient:     providerClient,
		providerClientOpts: clientOpts,
		projectID:          projectID,
		endpointOverrides:  endpointOverrides,
		cloudName:          cloud.Cloud,
		regionName:         effectiveRegion,
		logger:             logger,
	}, nil
}

func NewCachedProviderScope(cache *cache.LRUExpireCache, cloud clientconfig.Cloud, regionName string, caCert []byte, endpointOverrides *EndpointOverrides, logger logr.Logger) (Scope, error) {
	// The cache key covers both the cloud credentials AND the endpoint overrides
	// so that adding/changing endpoints.yaml invalidates the cached scope.
	key, err := getScopeCacheKey(cloud, endpointOverrides)
	if err != nil {
		return nil, fmt.Errorf("compute cloud config cache key: %w", err)
	}

	if scope, found := cache.Get(key); found {
		logger.V(4).Info("Using scope from cache", "cacheKey", key)
		return scope.(Scope), nil
	}

	scope, err := NewProviderScope(cloud, regionName, caCert, endpointOverrides, logger)
	if err != nil {
		return nil, err
	}

	token, err := scope.ExtractToken()
	if err != nil {
		return nil, err
	}

	// compute the token expiration time
	expiry := time.Until(token.ExpiresAt) / 2

	cache.Add(key, scope, expiry)
	return scope, nil
}

func (s *providerScope) ProjectID() string {
	return s.projectID
}

func (s *providerScope) NewComputeClient() (clients.ComputeClient, error) {
	endpointURL := s.endpointOverrides.GetEndpoint("compute", s.cloudName, s.regionName)
	return clients.NewComputeClient(s.providerClient, s.providerClientOpts, endpointURL)
}

func (s *providerScope) NewNetworkClient() (clients.NetworkClient, error) {
	endpointURL := s.endpointOverrides.GetEndpoint("network", s.cloudName, s.regionName)
	return clients.NewNetworkClient(s.providerClient, s.providerClientOpts, endpointURL)
}

func (s *providerScope) NewVolumeClient() (clients.VolumeClient, error) {
	endpointURL := s.endpointOverrides.GetEndpoint("volume", s.cloudName, s.regionName)
	return clients.NewVolumeClient(s.providerClient, s.providerClientOpts, endpointURL)
}

func (s *providerScope) NewImageClient() (clients.ImageClient, error) {
	endpointURL := s.endpointOverrides.GetEndpoint("image", s.cloudName, s.regionName)
	return clients.NewImageClient(s.providerClient, s.providerClientOpts, endpointURL)
}

func (s *providerScope) NewLbClient() (clients.LbClient, error) {
	endpointURL := s.endpointOverrides.GetEndpoint("loadbalancer", s.cloudName, s.regionName)
	return clients.NewLbClient(s.providerClient, s.providerClientOpts, endpointURL)
}

func (s *providerScope) ExtractToken() (*tokens.Token, error) {
	client, err := openstack.NewIdentityV3(s.providerClient, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, fmt.Errorf("create new identity service client: %w", err)
	}
	return tokens.Get(context.TODO(), client, s.providerClient.Token()).ExtractToken()
}

func NewProviderClient(cloud clientconfig.Cloud, regionName string, caCert []byte, logger logr.Logger) (*gophercloud.ProviderClient, *clientconfig.ClientOpts, string, error) {
	clientOpts := new(clientconfig.ClientOpts)

	// We explicitly disable reading auth data from env variables by setting an invalid EnvPrefix.
	// By doing this, we make sure that the data from clouds.yaml is enough to authenticate.
	// For more information: https://github.com/gophercloud/utils/v2/blob/8677e053dcf1f05d0fa0a616094aace04690eb94/openstack/clientconfig/requests.go#L508
	clientOpts.EnvPrefix = "NO_ENV_VARIABLES_"
	if regionName == "" {
		regionName = cloud.RegionName
	}
	if cloud.AuthInfo != nil {
		clientOpts.AuthInfo = cloud.AuthInfo
		clientOpts.AuthType = cloud.AuthType
		clientOpts.RegionName = regionName
		clientOpts.EndpointType = cloud.EndpointType
	}

	opts, err := clientconfig.AuthOptions(clientOpts)
	if err != nil {
		return nil, nil, "", fmt.Errorf("auth option failed for cloud %v: %v", cloud.Cloud, err)
	}
	opts.AllowReauth = true

	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, nil, "", fmt.Errorf("create providerClient err: %v", err)
	}

	ua := gophercloud.UserAgent{}
	ua.Prepend(fmt.Sprintf("cluster-api-provider-openstack/%s", version.Get().String()))
	provider.UserAgent = ua

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if cloud.Verify != nil {
		config.InsecureSkipVerify = !*cloud.Verify
	}
	if caCert != nil {
		config.RootCAs = x509.NewCertPool()
		ok := config.RootCAs.AppendCertsFromPEM(caCert)
		if !ok {
			// If no certificates were successfully parsed, set RootCAs to nil to use the host's root CA
			config.RootCAs = nil
		}
	}

	provider.HTTPClient.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: config}
	if klog.V(5).Enabled() {
		provider.HTTPClient.Transport = &osclient.RoundTripper{
			Rt:     provider.HTTPClient.Transport,
			Logger: &gophercloudLogger{logger},
		}
	}
	err = openstack.Authenticate(context.TODO(), provider, *opts)
	if err != nil {
		return nil, nil, "", fmt.Errorf("providerClient authentication err: %v", err)
	}

	projectID, err := getProjectIDFromAuthResult(provider.GetAuthResult())
	if err != nil {
		return nil, nil, "", err
	}

	return provider, clientOpts, projectID, nil
}

type gophercloudLogger struct {
	logger logr.Logger
}

// Printf is a default Printf method.
func (g gophercloudLogger) Printf(format string, args ...interface{}) {
	g.logger.Info(fmt.Sprintf(format, args...))
}

// getCloudFromSecret extract a Cloud from the given namespace:secretName.
// It also reads the optional "endpoints.yaml" key for per-service endpoint overrides.
func getCloudFromSecret(ctx context.Context, ctrlClient client.Client, secretNamespace string, secretName string, cloudName string, logger logr.Logger) (clientconfig.Cloud, []byte, *EndpointOverrides, error) {
	emptyCloud := clientconfig.Cloud{}

	if secretName == "" {
		return emptyCloud, nil, nil, nil
	}

	if cloudName == "" {
		return emptyCloud, nil, nil, fmt.Errorf("secret name set to %v but no cloud was specified. Please set cloud_name in your machine spec", secretName)
	}

	secret := &corev1.Secret{}
	err := ctrlClient.Get(ctx, types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}, secret)
	if err != nil {
		return emptyCloud, nil, nil, err
	}

	content, ok := secret.Data[CloudsSecretKey]
	if !ok {
		return emptyCloud, nil, nil, fmt.Errorf("OpenStack credentials secret %v did not contain key %v",
			secretName, CloudsSecretKey)
	}
	var clouds clientconfig.Clouds
	if err = yaml.Unmarshal(content, &clouds); err != nil {
		return emptyCloud, nil, nil, fmt.Errorf("failed to unmarshal clouds credentials stored in secret %v: %v", secretName, err)
	}

	// Parse optional endpoint overrides.
	var endpointOverrides *EndpointOverrides
	if endpointsData, hasEndpoints := secret.Data[EndpointsSecretKey]; hasEndpoints {
		logger.V(4).Info("Loading endpoint overrides from secret", "secret", secretName, "namespace", secretNamespace, "key", EndpointsSecretKey)
		var overrides EndpointOverrides
		if err = yaml.Unmarshal(endpointsData, &overrides); err != nil {
			return emptyCloud, nil, nil, fmt.Errorf("failed to unmarshal endpoint overrides stored in secret %v (key %v): %v", secretName, EndpointsSecretKey, err)
		}
		for cloud, byRegion := range overrides.Clouds {
			for region, byService := range byRegion {
				for service, url := range byService {
					logger.V(4).Info("Endpoint override loaded", "cloud", cloud, "region", region, "service", service, "url", url)
				}
			}
		}
		logger.V(4).Info("Endpoint overrides secret loaded", "secret", secretName, "namespace", secretNamespace)
		endpointOverrides = &overrides
	}

	// get caCert
	caCert, ok := secret.Data[CASecretKey]

	// clouds.Clouds[cloudName] returns a Cloud struct whose Cloud field is not
	// automatically populated from the YAML map key.  Populate it explicitly so
	// that endpoint-override lookups (which key on cloud.Cloud) work correctly
	// with standard clouds.yaml files that do not include an explicit
	// "cloud: <name>" line inside each cloud entry.
	cloud := clouds.Clouds[cloudName]
	cloud.Cloud = cloudName

	if !ok {
		return cloud, nil, endpointOverrides, nil
	}

	return cloud, caCert, endpointOverrides, nil
}

// getProjectIDFromAuthResult handles different auth mechanisms to retrieve the
// current project id. Usually we use the Identity v3 Token mechanism that
// returns the project id in the response to the initial auth request.
func getProjectIDFromAuthResult(authResult gophercloud.AuthResult) (string, error) {
	switch authResult := authResult.(type) {
	case tokens.CreateResult:
		project, err := authResult.ExtractProject()
		if err != nil {
			return "", fmt.Errorf("unable to extract project from CreateResult: %v", err)
		}

		return project.ID, nil

	default:
		return "", fmt.Errorf("unable to get the project id from auth response with type %T", authResult)
	}
}
