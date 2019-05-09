# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
