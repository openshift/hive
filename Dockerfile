FROM registry.svc.ci.openshift.org/openshift/release:golang-1.15 as builder
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .
RUN make build

FROM quay.io/app-sre/centos:7

# CentOS images do not get updates as they are meant to mirror ISO content, and thus this update
# is strongly recommended for security updates.
RUN yum -y update && yum clean all

# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then yum install -y openssh-clients && yum clean all && rm -rf /var/cache/yum/*; fi

# libvirt libraries required for running bare metal installer.
RUN if ! rpm -q libvirt-devel; then yum install -y libvirt-devel && yum clean all && rm -rf /var/cache/yum/*; fi

COPY --from=builder /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin
COPY --from=builder /go/src/github.com/openshift/hive/bin/operator /opt/services/hive-operator

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
