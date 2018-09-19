# Copyright 2018 The Kubernetes Authors.
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

# This image is meant to be used only for CI as a base for
# verification tests and unit tests.
# It combines the builder image (images/builder-image) and the
# hive source.

FROM    builder-base
RUN     mkdir -p /go/src/github.com/openshift/hive && \
        cd /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY    . .
RUN     chmod -R og+w /go
