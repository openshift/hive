FROM openshift/origin-release:golang-1.11

# setting Git username and email for workaround of
# https://github.com/jenkinsci/docker/issues/519
ENV GIT_COMMITTER_NAME hive-team
ENV GIT_COMMITTER_EMAIL hive-team@redhat.com

# Install the golint, use this to check our source for niceness
RUN go get -u golang.org/x/lint/golint

# Basic Debug Tools
RUN yum -y install strace tcping && yum clean all

# Install kubebuilder 1.0.8
RUN export version=1.0.8 && \
    cd /tmp && \
    curl -L -O https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_linux_amd64.tar.gz && \
    tar -zxvf /tmp/kubebuilder_${version}_linux_amd64.tar.gz && \
    mv kubebuilder_${version}_linux_amd64 /usr/local/kubebuilder && \
    rm /tmp/kubebuilder_${version}_linux_amd64.tar.gz

# Get rid of "go: disabling cache ..." errors.
RUN mkdir -p /go && chgrp -R root /go && chmod -R g+rwX /go
