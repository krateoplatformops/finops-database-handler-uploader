# finops-database-handler-uploader

A Kubernetes operator that uploads Python notebooks into the finops-database-handler via a `Notebook` Custom Resource, supporting both inline code and remote API endpoints as sources.

📖 **Full documentation**: [docs.krateo.io — finops-database-handler-uploader](https://docs.krateo.io/key-concepts/kcf/finops-components/finops-database-handler-uploader)

---

## Key features

- Uploads Python notebooks to the finops-database-handler using a declarative Custom Resource
- Supports both inline notebook code and remote file endpoints as notebook sources
- The notebook's compute endpoint name is automatically derived from the Custom Resource name

## Requirements

| Dependency | Minimum version |
|------------|----------------|
| Kubernetes | v1.26 |
| Krateo | v3.0.0 |
| finops-database-handler | v0.5.3 |

## Install

```bash
helm repo add krateo https://charts.krateo.io
helm repo update
helm install finops-database-handler-uploader krateo/finops-database-handler-uploader --namespace krateo-system --create-namespace
```

> For advanced installation options, custom values, and upgrade instructions, see the [installation guide](https://docs.krateo.io/key-concepts/kcf/finops-components/finops-database-handler-uploader).

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `WATCH_NAMESPACE` | Yes | — | Namespace to watch for Custom Resources; auto-set by the Helm chart |
| `POLLING_INTERVAL` | No | `300` | Time between reconciles in seconds |
| `MAX_RECONCILE_RATE` | No | `1` | Number of concurrent reconcile workers |
| `FINOPS_DATABASE_HANDLER_ENDPOINT_NAME` | No | `finops-database-handler-endpoint` | Name of the secret containing the finops-database-handler endpoint |
| `FINOPS_DATABASE_HANDLER_ENDPOINT_NAMESPACE` | No | `krateo-system` | Namespace of the secret containing the finops-database-handler endpoint |
| `FINOPS_DATABASE_HANDLER_URL_OVERRIDE` | No | — | Override URL for the finops-database-handler, bypassing the endpoint secret |