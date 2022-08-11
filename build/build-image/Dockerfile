FROM registry.ci.openshift.org/openshift/release:golang-1.18

# setting Git username and email for workaround of
# https://github.com/jenkinsci/docker/issues/519
ENV GIT_COMMITTER_NAME hive-team
ENV GIT_COMMITTER_EMAIL hive-team@redhat.com

# Basic Debug Tools
RUN yum -y install strace tcping && yum clean all

# Install kubebuilder tools for controller-runtime testenv (make test-integration)
RUN K8S_VERSION=1.19.2 && \
    cd /tmp && \
    curl -sSLo envtest-bins.tar.gz "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-${K8S_VERSION}-$(go env GOOS)-$(go env GOARCH).tar.gz" && \
    mkdir /usr/local/kubebuilder && \
    tar -C /usr/local/kubebuilder --strip-components=1 -zvxf envtest-bins.tar.gz && \
    rm envtest-bins.tar.gz

# Get rid of "go: disabling cache ..." errors.
RUN mkdir -p /go && chgrp -R root /go && chmod -R g+rwX /go
