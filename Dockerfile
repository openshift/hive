FROM registry.ci.openshift.org/openshift/release:rhel-8-release-golang-1.21-openshift-4.16 as builder_rhel8
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .

## debugging
# https://redhat-internal.slack.com/archives/C04PZ7H0VA8/p1721932345729919?thread_ts=1717698403.965519&cid=C04PZ7H0VA8
RUN mount && \
  ls /etc/pki/entitlement && \ 
  ls /run/serets/rhsm && \ 
RUN rmdir /run/secrets/rhsm
##

RUN if [ -e "/activation-key/org" ]; then subscription-manager register --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi

RUN dnf -y install git python3-pip
RUN make build-hiveutil

FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.21-openshift-4.16 as builder_rhel9
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .

## debugging
# https://redhat-internal.slack.com/archives/C04PZ7H0VA8/p1721932345729919?thread_ts=1717698403.965519&cid=C04PZ7H0VA8
RUN mount && \
  ls /etc/pki/entitlement && \ 
  ls /run/serets/rhsm && \ 
RUN rmdir /run/secrets/rhsm
##

RUN if [ -e "/activation-key/org" ]; then subscription-manager register --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi
RUN dnf -y install git python3-pip
RUN make build-hiveadmission build-manager build-operator && \
  make build-hiveutil

FROM registry.redhat.io/rhel9-4-els/rhel:9.4

## debugging
# https://redhat-internal.slack.com/archives/C04PZ7H0VA8/p1721932345729919?thread_ts=1717698403.965519&cid=C04PZ7H0VA8
RUN mount && \
  ls /etc/pki/entitlement && \ 
  ls /run/serets/rhsm && \ 
RUN rmdir /run/secrets/rhsm
##

RUN if [ -e "/activation-key/org" ]; then subscription-manager register --org $(cat "/activation-key/org") --activationkey $(cat "/activation-key/activationkey"); fi


##
# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then dnf install -y openssh-clients && dnf clean all && rm -rf /var/cache/dnf/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-libs; then dnf install -y libvirt-libs && dnf clean all && rm -rf /var/cache/dnf/*; fi

# tar is needed to package must-gathers on install failure
RUN if ! which tar; then dnf install -y tar && dnf clean all && rm -rf /var/cache/dnf/*; fi

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

# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]
