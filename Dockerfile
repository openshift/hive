ARG CONTAINER_SUB_MANAGER_OFF=0
ARG EL8_BUILD_IMAGE=${EL8_BUILD_IMAGE:-registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.24-openshift-4.20}
ARG EL9_BUILD_IMAGE=${EL9_BUILD_IMAGE:-registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.20}
ARG BASE_IMAGE=${BASE_IMAGE:-registry.access.redhat.com/ubi9/ubi-minimal:latest}

FROM ${EL8_BUILD_IMAGE} as builder_rhel8
ARG GO=${GO:-go}
ARG BUILD_IMAGE_CUSTOMIZATION
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .

RUN if [ -f "${BUILD_IMAGE_CUSTOMIZATION}" ]; then "${BUILD_IMAGE_CUSTOMIZATION}"; fi

RUN if [ -e "/activation-key/org" ]; then dnf install -y subscription-manager && dnf clean all && rm -rf /var/cache/dnf/*; unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi
RUN python3 -m ensurepip
ENV GO=${GO}
RUN make build-hiveutil

FROM ${EL9_BUILD_IMAGE} as builder_rhel9
ARG GO=${GO:-go}
ARG CONTAINER_SUB_MANAGER_OFF
ARG BUILD_IMAGE_CUSTOMIZATION
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .

RUN if [ -f "${BUILD_IMAGE_CUSTOMIZATION}" ]; then "${BUILD_IMAGE_CUSTOMIZATION}"; fi

ENV SMDEV_CONTAINER_OFF=${CONTAINER_SUB_MANAGER_OFF}
RUN if [ -e "/activation-key/org" ]; then dnf install -y subscription-manager && dnf clean all && rm -rf /var/cache/dnf/*; unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi
RUN python3 -m ensurepip
ENV GO=${GO}
RUN make build-hiveadmission build-manager build-operator && \
  make build-hiveutil

FROM ${BASE_IMAGE}
ARG CONTAINER_SUB_MANAGER_OFF
ENV SMDEV_CONTAINER_OFF=${CONTAINER_SUB_MANAGER_OFF}
ARG DNF=${DNF:-microdnf}

RUN if [ -e "/activation-key/org" ]; then ${DNF} install -y subscription-manager && ${DNF} clean all && rm -rf /var/cache/dnf/*; unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi


##
# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then ${DNF} install -y openssh-clients && ${DNF} clean all && rm -rf /var/cache/dnf/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-libs; then ${DNF} install -y libvirt-libs && ${DNF} clean all && rm -rf /var/cache/dnf/*; fi

# tar is needed to package must-gathers on install failure
RUN if ! command -v tar; then ${DNF} install -y tar && ${DNF} clean all && rm -rf /var/cache/dnf/*; fi

COPY --from=builder_rhel9 /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder_rhel9 /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder_rhel9 /go/src/github.com/openshift/hive/bin/operator /opt/services/hive-operator

COPY --from=builder_rhel8 /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin/hiveutil.rhel8
COPY --from=builder_rhel9 /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin/hiveutil

# Hacks to allow writing known_hosts, homedir is / by default in OpenShift.
# Bare metal installs need to write to $HOME/.cache, and $HOME/.ssh for as long as
# we're hitting libvirt over ssh. OpenShift will not let you write these directories
# by default so we must setup some permissions here.
ENV HOME /home/hive
RUN mkdir -p /home/hive && \
  chgrp -R 0 /home/hive && \
  chmod -R g=u /home/hive

RUN mkdir -p /output/hive-trusted-cabundle && \
  chgrp -R 0 /output/hive-trusted-cabundle && \
  chmod -R g=u /output/hive-trusted-cabundle

# replace removed symlink when using activation-key
RUN if [ -e "/activation-key/org" ]; then ln -s /etc/rhsm-host /run/secrets/rhsm ; fi

# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]

LABEL name="hive"
LABEL summary="API driven OpenShift 4 cluster provisioning and management"
LABEL description="Hive is an operator which runs as a service on top of Kubernetes/OpenShift. The Hive service can be used to provision and perform initial configuration of OpenShift clusters"
LABEL distribution-scope="public"
LABEL release="1"
LABEL url="https://github.com/openshift/hive"
LABEL vendor="Red Hat, Inc."
LABEL version="1"
LABEL io.k8s.description="Hive is an operator which runs as a service on top of Kubernetes/OpenShift. The Hive service can be used to provision and perform initial configuration of OpenShift clusters"
LABEL io.k8s.display-name="hive-operator"
LABEL io.openshift.tags="cluster,management,provision"
LABEL com.redhat.component="hive-rhel9"
