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

FROM openshift/origin-release:golang-1.10

# Install the golint, use this to check our source for niceness
RUN go get -u github.com/golang/lint/golint

# Install gomock and mockgen for the mocks used in unit tests
RUN go get -u github.com/golang/mock/gomock
RUN go get -u github.com/golang/mock/mockgen

# Install kubebuilder 1.0.1    TODO: ask cewong if there's a way to cache this. It takes a long time to d/l
RUN export version=1.0.1 && \
    cd /tmp && \
    curl -L -O https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${version}/kubebuilder_${version}_linux_amd64.tar.gz && \
    tar -zxvf /tmp/kubebuilder_${version}_linux_amd64.tar.gz && \
    mv kubebuilder_${version}_linux_amd64 /usr/local/kubebuilder && \
    rm /tmp/kubebuilder_${version}_linux_amd64.tar.gz
