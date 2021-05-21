# Release Image Verification

## Overview

Currently during upgrades, OpenShift 4 checks that the release image
that is being requested in upgrade is trusted. By default it only trusts
release images that are published by Red Hat. This is very important
since release image is the definition of what runs in the cluster and
therefore must be verified before it is used.

Similarly, currently Hive uses the release image specified for the
cluster to extract binaries like oc, openshift-install and then executes
them in internal environment (Hive clusters), it is important that Hive
enforce that the content is trusted before running it.
The mechanism that is used by OpenShift during upgrades should be the
best way to establish and enforce that trust.

## How verification works?

for details see https://github.com/openshift/cluster-update-keys

The definition of trust is defined in ConfigMap data keys:

store-*:
These define the ways in which signature(s) for specific release image
digests can be fetched.
Multiple stores can be used to provide resilient source of signatures

verifier-public-key-*:
This is the public key that will be used to verify the signature and
release image are indeed trusted.
Multiple keys imply that the digest and signatures must be trusted by
every key.

OpenShift publishes this config map by default in each cluster at
openshift-config-managed/release-verification . If Hive is running on
OpenShift 4 and it intends to allow only release images published by Red
Hat, it can use this config map as definitions of trust.

The verification requires digest of the release image, the
repository or source of registry is not used for verification. Therefore
release images with tag cannot be verified using this method.

OpenShift publishes [library][release-image-verify-lib] for working with trust
definitions, stores and verification process. Hive uses this library to perform
the verification.

## How Hive uses verification?

When verification is enabled (it is disabled by default),

Hive validation will reject ClusterImageSets that use release images
with tags.

Hive controllers will verify the release image before
extracting any information.
In cases when the verification fails, Hive will set the
InstallImagesNotResolved condition to True with
ReleaseImageVerificationFailed reason.

How to configure Hive for verifying release images?

By default no trust for release images is defined and therefore no
verification occurs.

Verification is turned on by defining a trust using HiveConfig.

```yaml
spec:
  releaseImageVerificationConfigMapRef:
    namespace: <> # namespace of the config map that defines the trust
    name: <> # name of the config map that defines the trust
```

[release-image-verify-lib]: https://pkg.go.dev/github.com/openshift/library-go/pkg/verify
