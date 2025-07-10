# FinOps Database Handler Uploader 

This service allows to upload python code notebookes into the [finops-database-handler](https://github.com/krateoplatformops/finops-database-handler) through the use of a custom resource. The custom resource can either contain code inline or a API endpoint to a python file.

## Summary

1. [Overview](#overview)
2. [Configuration](#configuration)
3. [Examples](#examples)
4. [Installation](#installation)

## Overview
To upload a new notebook into the [finops-database-handler](https://github.com/krateoplatformops/finops-database-handler) you need to create a new `Notebook` custom resource (see [sample data](./sample_data/)). The field `type` can be either `inline` or `api`. If you use inline, you need to put the notebook code in the `inline` field. Otherwise, if you use the api type, you need to compile the `api` field. See the [examples](#examples) section for more details. The name of the endpoint of the notebook (see [endpoints for the finops-database-handler](https://github.com/krateoplatformops/finops-database-handler?tab=readme-ov-file#api)) is the name of the custom resource.

## Configuration
The finops-database-handler-uploader accepts six environment variables (or parameters):
- WATCH_NAMESPACE: automatically set to the install namespace if you use the [Helm chart](https://github.com/krateoplatformops/finops-database-handler-uploader-chart)
- POLLING_INTERVAL: time between reconciles in seconds, defaults to 300
- MAX_RECONCILE_RATE: number of concurrent reconciles (i.e., number of workers)
- FINOPS_DATABASE_HANDLER_ENDPOINT_NAME: name of the secret that contains the endpoint for the finops-database-handler, defaults to `finops-database-handler-endpoint`
- FINOPS_DATABASE_HANDLER_ENDPOINT_NAMESPACE: namespace of the secret that contains the endpoint for the finops-database-handler, defaults to `krateo-system`
- FINOPS_DATABASE_HANDLER_URL_OVERRIDE: if you do not want to use the `server-url` from the endpoint secret, you can put the url here (used for testing purposes) 

## Examples
Inline example:
```yaml
apiVersion: finops.krateo.io/v1
kind: Notebook
metadata:
  name: query
  namespace: uploader-test
spec: 
  type: inline
  inline: |
    import pip._internal as pip
    def install(package):
        pip.main(['install', package])
    def main():   
        table_name_arg = sys.argv[5]
        table_name_key_value = str.split(table_name_arg, '=')
        if len(table_name_key_value) == 2:
            if table_name_key_value[0] == 'table_name':
                table_name = table_name_key_value[1]
        try:
            resource_query = f"SELECT * FROM {table_name}"
            cursor.execute(resource_query)
            raw_data = cursor.fetchall()
            print(pd.DataFrame(raw_data))
        finally:
            cursor.close()
            connection.close()
    if __name__ == "__main__":
        try:
            import pandas as pd
        except ImportError:
            install('pandas')
            import pandas as pd
        main()
```

API example:
```yaml
apiVersion: finops.krateo.io/v1
kind: Notebook
metadata:
  name: query2
  namespace: uploader-test
spec: 
  type: api
  api: 
    path: "/krateoplatformops/finops-database-handler/refs/heads/main/notebook_samples/query.py"
    verb: GET
    endpointRef:
      name: raw-github
      namespace: uploader-test
---
apiVersion: v1
kind: Secret
metadata:
  name: raw-github
  namespace: uploader-test
stringData:
  server-url: https://raw.githubusercontent.com
```

## Installation
The webservice can be installed through the [Helm chart](https://github.com/krateoplatformops/finops-database-handler-uploader-chart):
```sh
helm repo add krateo https://charts.krateo.io
helm repo update krateo
helm install finops-database-handler-uploader krateo/finops-database-handler-uploader
```



