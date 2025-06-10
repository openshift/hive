# Hermetic builds for hive

https://konflux.pages.redhat.com/docs/users/building/activation-keys-subscription.html#configuring-an-rpm-lockfile-for-hermetic-builds

* Basically just follow the instructions
* Step 1: I used the same base image from the hive root dockerfile: `podman run -v ${PWD}:/source/:z -it $BASE_IMAGE bash`
* Step 2: The activation key I used was named `hive`, with our internal org ID
* Steps 3, 5, and 6 can be skippped unless `redhat.repo` is out of date and needs to be updated
* Step 7:
  * I logged into skopeo outside of the base image running container, and saved the auth file in this directory (which is mounted inside the container)
  * Instead of logging in from within the container, `export REGISTRY_AUTH_FILE=<path to auth file mounted inside container>`
* Step 8 can be skipped unless `rpms.in.yaml` needs to be updated
