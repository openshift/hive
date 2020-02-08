# hiveutil

The `hiveutil` CLI offers several commands to help manage clusters with Hive.

To build the `hiveutil` binary, run `make hiveutil`.

### Create Cluster

The `create-cluster` command generates a `ClusterDeployment` and submits it to the Hive cluster using your current kubeconfig.

To view what `create-cluster` generates, *without* submitting it to the API server, add `-o yaml` to the above command. If you need to make any changes not supported by `create-cluster` options, the output can be saved, edited, and then submitted with `oc apply`. This is also a useful way to generate sample yaml.

By default, `create-cluster` assumes the latest Hive master CI build and the latest OpenShift stable release. `--release-image` can be specified to control which OpenShift release image to use.

#### Create Cluster on AWS

Credentials will be read from your AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables. Alternatively you can specify an AWS credentials file with --creds-file.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=aws mycluster
```

#### Create Cluster on Azure

Credentials will be read from `~/.azure/osServicePrincipal.json` typically created via the `az login` command.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=azure --azure-base-domain-resource-group-name=myresourcegroup mycluster
```

#### Create Cluster on GCP

Credentials will be read from `~/.gcp/osServiceAccount.json`, this can be created by:

 1. Login to GCP console at https://console.cloud.google.com/
 1. Create a service account with the owner role.
 1. Create a key for the service account.
 1. Select JSON for the key type.
 1. Download resulting JSON file and save to `~/.gcp/osServiceAccount.json`.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=gcp mycluster
```

### Other Commands

To see other commands offered by `hiveutil`, run `hiveutil --help`.
