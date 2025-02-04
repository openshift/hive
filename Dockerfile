ARG CONTAINER_SUB_MANAGER_OFF=0
ARG EL8_BUILD_IMAGE=${EL8_BUILD_IMAGE:-registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.22-openshift-4.17}
ARG EL9_BUILD_IMAGE=${EL9_BUILD_IMAGE:-registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.22-openshift-4.17}
ARG BASE_IMAGE=${BASE_IMAGE:-registry.ci.openshift.org/ocp/4.16:base-rhel9}

FROM ${EL8_BUILD_IMAGE} as builder_rhel8
ARG GO=${GO:-go}
ARG BUILD_IMAGE_CUSTOMIZATION
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .

RUN if [ -f "${BUILD_IMAGE_CUSTOMIZATION}" ]; then "${BUILD_IMAGE_CUSTOMIZATION}"; fi

RUN if [ -e "/activation-key/org" ]; then unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi
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
RUN if [ -e "/activation-key/org" ]; then unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi
RUN python3 -m ensurepip
ENV GO=${GO}
RUN make build-hiveadmission build-manager build-operator && \
  make build-hiveutil

FROM ${BASE_IMAGE}
ARG CONTAINER_SUB_MANAGER_OFF
ENV SMDEV_CONTAINER_OFF=${CONTAINER_SUB_MANAGER_OFF}

RUN if [ -e "/activation-key/org" ]; then unlink /etc/rhsm-host; subscription-manager register --force --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi


##
# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then dnf install -y openssh-clients && dnf clean all && rm -rf /var/cache/dnf/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-libs; then dnf install -y libvirt-libs && dnf clean all && rm -rf /var/cache/dnf/*; fi

# tar is needed to package must-gathers on install failure
RUN if ! command -v tar; then dnf install -y tar && dnf clean all && rm -rf /var/cache/dnf/*; fi

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
