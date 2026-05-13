# Hermetic builds for hive

https://konflux.pages.redhat.com/docs/users/building/activation-keys-subscription.html#configuring-an-rpm-lockfile-for-hermetic-builds

* Basically just follow the instructions
* Files referenced in the instructions live in the directory containing this README. Path accordingly.
* Step 1: As of this writing, the ubi-minimal base image from the hive root dockerfile currently does *not* have subscription-manager in it. You can copy it from your system into your container's mount point *or* use a different image (of the same RHEL version) e.g.: `podman run -v ${PWD}:/source/:z -it registry.access.redhat.com/ubi9/ubi:latest bash`
* Step 2: A viable activation key can be found in our bitwarden account. (I think you can instead create a fresh one following the instructions, but I have not tried it.)
* Steps 3, 5, and 6 can be skippped unless `redhat.repo` is out of date and needs to be updated
* Step 7:
  * I logged into skopeo outside of the base image running container, and saved the auth file in this directory (which is mounted inside the container)
  * Instead of logging in from within the container, `export REGISTRY_AUTH_FILE=<path to auth file mounted inside container>`
* Step 8 can be skipped unless `rpms.in.yaml` needs to be updated
* In step 9, I ran into the SSL errors and had to fix the path under `/etc/pki/entitlement` in the `redhat.repo` file. YMMV. (The directory name may or may not be unique to the container image from which you're running this procedure.)
