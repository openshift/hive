FROM openshift/origin-release:golang-1.13 as builder
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .
RUN make build

FROM centos:7

# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then yum install -y openssh-clients && yum clean all && rm -rf /var/cache/yum/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-devel; then yum install -y libvirt-devel && yum clean all && rm -rf /var/cache/yum/*; fi

COPY --from=builder /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin
COPY --from=builder /go/src/github.com/openshift/hive/bin/operator /opt/services/hive-operator
COPY --from=builder /go/src/github.com/openshift/hive/bin/hive-apiserver /opt/services/

# Hacks to allow writing known_hosts, homedir is / by default in OpenShift.
# Bare metal installs need to write to $HOME/.cache, and $HOME/.ssh for as long as
# we're hitting libvirt over ssh. OpenShift will not let you write these directories
# by default so we must setup some permissions here.
ENV HOME /home/hive
RUN mkdir -p /home/hive && \
    chgrp -R 0 /home/hive && \
    chmod -R g=u /home/hive


# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]
