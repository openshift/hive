# Running v1alpha1 Aggregated API Server

Create certificates for the API server. These certificates only need to be created once:
They can be re-used on subsequent deployments of the aggregated API server.


```shell script
./hack/make-apiserver-certs.sh
```

Deploy the aggregated API server.

```shell script
./hack/deploy-apiserver.sh
```
