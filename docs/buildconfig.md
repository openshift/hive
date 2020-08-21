# Using a BuildConfig to build Hive

When deployed to an OpenShift server, Hive can be automatically built and deployed using a BuildConfig and a GitHub
Webhook. 

## To setup the BuildConfig:

1. Modify the `config/builconfig/hive-buildconfig.yaml` manifest by adding a secret string in the placeholder for a 
   webhook secret. If setting up for your own fork, replace the git repository URL with your own fork's URL.
1. Apply the manifests in `config/buildconfig/` to the namespace where the Hive operator is installed (typically the `hive` namespace).
1. Setup a deployment trigger using the `oc set triggers` command:
   ```
   oc project hive
   oc set triggers deployment/hive-operator --from-image=hive:latest --containers hive-operator
   ```

## To setup the Webhook:

1. Obtain the webhook URL for the BuildConfig by using `oc describe`
   ```
   oc project hive
   oc describe buildconfig/hive
   ```
   The description will contain a `Webhook GitHub URL`, copy that URL and replace the 
   `<secret>` placeholder with the secret string you entered in the buildconfig manifest.

1. Go to the GitHub repository, click on `Settings` and navigate to `Webhooks`.
1. Click on `Add Webhook`
1. Enter the URL from the webconfig in the `Payload URL` field
1. Select `application/json` in Content Type
1. Leave Secret blank
1. If not using valid certificates for your cluster, select to Disable SSL Verification
1. Click on `Add webhook`
