# Customizing OpenStack Service Endpoints

CAPO supports overriding the OpenStack service endpoints used for each resource
type.  This is useful in multi-region deployments, air-gapped environments, or
when internal/private endpoints should be used for a specific service while the
rest of the cloud configuration remains unchanged.

## How it works

Endpoint overrides are stored as a YAML document in the same Kubernetes Secret
that holds the `clouds.yaml` credentials.  The key must be named
`endpoints.yaml`.

CAPO reads this key when it resolves the Secret, and for every OpenStack service
client it creates it will use the custom URL instead of the URL returned by the
Keystone service catalog.

## Supported services

| Key | OpenStack service | Typical version path |
|---|---|---|
| `compute` | Nova (instances, flavors, server groups, availability zones) | `/v2.1/` |
| `network` | Neutron (networks, subnets, ports, routers, security groups, floating IPs, trunks) | `/v2.0/` |
| `volume` | Cinder / Block Storage v3 (persistent volumes) | `/v3/` |
| `image` | Glance / Image v2 (machine images) | `/v2/` |
| `loadbalancer` | Octavia / Load Balancer v2 | `/v2.0/` |

> **Note:** Service keys are **case-insensitive** — you can write `image`,
> `Image`, or `IMAGE`.  However, lowercase is recommended by convention.

## Format

```yaml
# endpoints.yaml
clouds:
  <cloudName>:
    <regionName>:
      <service>: <url>
```

* `<cloudName>` – the cloud name as it appears in `clouds.yaml` (and in
  `OpenStackIdentityReference.cloudName`).  This **is** case-sensitive.
* `<regionName>` – the OpenStack region name (case-sensitive).  Use an empty
  string (`""`) as a catch-all wildcard that matches any region.
* `<service>` – one of `compute`, `network`, `volume`, `image`, `loadbalancer`
  (case-insensitive).
* `<url>` – the full base URL of the service endpoint, including the version
  path.  A trailing slash is added automatically if missing.

Only the clouds, regions, and services that you need to override must be
listed.  Any service without an entry will continue to use the URL discovered
from the Keystone service catalog.

## Understanding endpoint version paths

Different OpenStack services use different version path prefixes.  When you look
at the Keystone service catalog (`openstack catalog list`) you may see URLs like:

| Service | Catalog URL example | Notes |
|---|---|---|
| compute (Nova) | `https://host:8774/v2.1/` | Ends with `/v2.1/` |
| image (Glance) | `https://host:9292/` | Often just the port, **no** version path |
| network (Neutron) | `https://host:9696/v2.0/` | Ends with `/v2.0/` |
| volume (Cinder) | `https://host:8776/v3/` | Ends with `/v3/` |

This is **normal** — each service exposes its own API version.  When overriding
endpoints you must use the **full URL that Glance expects**, for example:
`https://glance.example.com/v2/` (not just `https://glance.example.com/`).

## Full example

```yaml
# endpoints.yaml
clouds:
  mycloud:
    RegionOne:
      compute: https://nova-internal.example.com/v2.1/
      network: https://neutron-internal.example.com/v2.0/
      volume: https://cinder-internal.example.com/v3/
      image: https://glance-internal.example.com/v2/
      loadbalancer: https://octavia-internal.example.com/v2.0/
    RegionTwo:
      compute: https://nova-regiontwo.example.com/v2.1/
```

### Wildcard region (catch-all)

If your overrides apply to all regions, use an empty-string key:

```yaml
clouds:
  mycloud:
    "":
      image: https://glance-custom.example.com/v2/
```

## Adding the key to an existing Secret

If you already have a Secret with `clouds.yaml` (and optionally `cacert`), patch
it to add the `endpoints.yaml` key:

```bash
# Encode the file to base64
ENDPOINTS_B64=$(base64 -w 0 < endpoints.yaml)

kubectl patch secret my-openstack-credentials \
  --type=merge \
  -p "{\"data\":{\"endpoints.yaml\":\"${ENDPOINTS_B64}\"}}"
```

Or create a new Secret that contains all three keys at once:

```bash
kubectl create secret generic my-openstack-credentials \
  --from-file=clouds.yaml=clouds.yaml \
  --from-file=cacert=ca.crt \
  --from-file=endpoints.yaml=endpoints.yaml
```

## Interaction with region overrides

When CAPO looks up an endpoint override it uses:

1. The cloud name from `OpenStackIdentityReference.cloudName`.
2. The region name from `OpenStackIdentityReference.region` (if set), or
   `region_name` from `clouds.yaml` as the fallback.

Make sure the `<regionName>` key in `endpoints.yaml` matches exactly (it is
case-sensitive).

## Region fallback rules

When looking up a service endpoint, CAPO tries these in order:

1. **Exact match** on the region name.
2. **Wildcard**: an empty-string `""` region key matches any region.
3. **Single-region default**: if only one region is configured for the cloud,
   it is used regardless of the requested region name.

## Debugging

To see endpoint override activity, run CAPO with verbosity level 2 or higher
(`-v=2`).  This will log:

* Which endpoint overrides were loaded from the secret.
* Which override URL was selected for each service client.
* When image lookup fails: the list of available images from the server.

If you see "no images were found" errors, check that:

1. Your image name is spelled exactly right (names are case-sensitive and
   may contain spaces, e.g. `"Debian 13"`).
2. The `image` endpoint override URL includes the correct version path
   (e.g. `https://host:9292/v2/` — **not** just `https://host:9292/`).
3. Your credentials have permission to list images.
