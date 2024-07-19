FROM registry.redhat.io/rhel8/go-toolset:1.21 as builder_rhel8
COPY . .
USER root
RUN dnf install -y git python3-pip
USER default
RUN python3 -m venv ~/python-hive-venv && \
  source ~/python-hive-venv/bin/activate && \
  pip3 install GitPython && \
  git config --global --add safe.directory "$PWD" && \
  make build-hiveutil

FROM registry.redhat.io/rhel9/go-toolset:1.21 as builder_rhel9
COPY . .
USER root
RUN dnf install -y git python3-pip
USER default
RUN python3 -m venv ~/python-hive-venv && \
  source ~/python-hive-venv/bin/activate && \
  pip3 install GitPython && \
  git config --global --add safe.directory "$PWD" && \
  make build-hiveadmission build-manager build-operator && \
  # Separate section for hiveutil to keep open files in check
  make build-hiveutil

FROM registry.redhat.io/rhel9-4-els/rhel:9.4

# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then dnf install -y openssh-clients && dnf clean all && rm -rf /var/cache/dnf/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-libs; then dnf install -y libvirt-libs && dnf clean all && rm -rf /var/cache/dnf/*; fi

# tar is needed to package must-gathers on install failure
RUN if ! which tar; then dnf install -y tar && dnf clean all && rm -rf /var/cache/dnf/*; fi

COPY --from=builder_rhel9 /opt/app-root/src//bin/manager /opt/services/
COPY --from=builder_rhel9 /opt/app-root/src//bin/hiveadmission /opt/services/
COPY --from=builder_rhel9 /opt/app-root/src//bin/operator /opt/services/hive-operator

COPY --from=builder_rhel8 /opt/app-root/src//bin/hiveutil /usr/bin/hiveutil.rhel8
COPY --from=builder_rhel9 /opt/app-root/src//bin/hiveutil /usr/bin

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

ENTRYPOINT ["/opt/services/manager"]
