FROM registry.access.redhat.com/ubi9/go-toolset:1.21 as builder
COPY . .
RUN python3 -m venv ~/python-hive-venv && \
  source ~/python-hive-venv/bin/activate && \
  pip install GitPython && \
  git config --global --add safe.directory "$PWD" && \
  make build

FROM registry.redhat.io/rhel9-4-els/rhel:9.4

ARG DNF=dnf

RUN $DNF -y update && $DNF clean all

# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then $DNF install -y openssh-clients && $DNF clean all && rm -rf /var/cache/dnf/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-libs; then $DNF install -y libvirt-libs && $DNF clean all && rm -rf /var/cache/dnf/*; fi

COPY --from=builder /opt/app-root/src//bin/manager /opt/services/
COPY --from=builder /opt/app-root/src//bin/hiveadmission /opt/services/
COPY --from=builder /opt/app-root/src//bin/hiveutil /usr/bin
COPY --from=builder /opt/app-root/src//bin/operator /opt/services/hive-operator

# Hacks to allow writing known_hosts, homedir is / by default in OpenShift.
# Bare metal installs need to write to $HOME/.cache, and $HOME/.ssh for as long as
# we're hitting libvirt over ssh. OpenShift will not let you write these directories
# by default so we must setup some permissions here.
ENV HOME /home/hive
RUN mkdir -p /home/hive && \
  chgrp -R 0 /home/hive && \
  chmod -R g=u /home/hive

# This is so that we can write source certificate anchors during container start up.
RUN mkdir -p /etc/pki/ca-trust/source/anchors && \
  chgrp -R 0 /etc/pki/ca-trust/source/anchors && \
  chmod -R g=u /etc/pki/ca-trust/source/anchors

# This is so that we can run update-ca-trust during container start up.
RUN mkdir -p /etc/pki/ca-trust/extracted/openssl && \
  mkdir -p /etc/pki/ca-trust/extracted/pem && \
  mkdir -p /etc/pki/ca-trust/extracted/java && \
  chgrp -R 0 /etc/pki/ca-trust/extracted && \
  chmod -R g=u /etc/pki/ca-trust/extracted

# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]
