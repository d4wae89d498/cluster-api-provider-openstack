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

| Key | OpenStack service |
|---|---|
| `compute` | Nova (instances, flavors, server groups, availability zones) |
| `network` | Neutron (networks, subnets, ports, routers, security groups, floating IPs, trunks) |
| `volume` | Cinder / Block Storage v3 (persistent volumes) |
| `image` | Glance / Image v2 (machine images) |
| `loadbalancer` | Octavia / Load Balancer v2 |

## Format

```yaml
# endpoints.yaml
<service>:
  <cloudName>:
    <regionName>: <url>
```

* `<service>` – one of `compute`, `network`, `volume`, `image`, `loadbalancer`.
* `<cloudName>` – the cloud name as it appears in `clouds.yaml` (and in
  `OpenStackIdentityReference.cloudName`).
* `<regionName>` – the OpenStack region name.
* `<url>` – the full base URL of the service endpoint.  **Must end with a
  trailing slash (`/`).**

Only the services and cloud/region combinations that you need to override must
be listed.  Any service without an entry will continue to use the URL discovered
from the Keystone service catalog.

## Full example

```yaml
# endpoints.yaml
compute:
  mycloud:
    RegionOne: https://nova-internal.example.com/v2.1/
    RegionTwo: https://nova-regiontwo.example.com/v2.1/
network:
  mycloud:
    RegionOne: https://neutron-internal.example.com/v2.0/
volume:
  mycloud:
    RegionOne: https://cinder-internal.example.com/v3/
image:
  mycloud:
    RegionOne: https://glance-internal.example.com/v2/
loadbalancer:
  mycloud:
    RegionOne: https://octavia-internal.example.com/v2.0/
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
