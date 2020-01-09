FROM openshift/origin-release:golang-1.12 as builder
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .
RUN go build -o bin/manager github.com/openshift/hive/cmd/manager
RUN go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil
RUN go build -o bin/hiveadmission github.com/openshift/hive/cmd/hiveadmission
RUN go build -o bin/hive-operator github.com/openshift/hive/cmd/operator
RUN go build -o bin/hive-apiserver github.com/openshift/hive/cmd/hive-apiserver

#FROM registry.access.redhat.com/ubi7/ubi
FROM registry.svc.ci.openshift.org/origin/4.1:base

# ssh-agent required for gathering logs in some situations:
RUN if ! rpm -q openssh-clients; then yum install -y openssh-clients && yum clean all && rm -rf /var/cache/yum/*; fi

# libvirt libraries required for running bare metal installer.
RUN yum install -y libvirt-devel && yum clean all && rm -rf /var/cache/yum/*

COPY --from=builder /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin
COPY --from=builder /go/src/github.com/openshift/hive/bin/hive-operator /opt/services
COPY --from=builder /go/src/github.com/openshift/hive/bin/hive-apiserver /opt/services/

# In the event of failures we run ssh commands to execute the script which gathers logs
# from all systems in the cluster, and scp's those from the bootstrap node into the
# /logs volume in the install pod. This requires the current user ID (randomly assigned by
# the cluster) to be in the passwd file. At runtime we execute with gid 0, so making the
# file group writable allows our install process to add our install user ID to the passwd
# file and unblock ssh.
RUN chmod g+rw /etc/passwd

# Hacks to allow writing known_hosts, homedir is / by default in OpenShift.
# Bare metal installs need to write to $HOME/.cache, and $HOME/.ssh for as long as
# we're hitting libvirt over ssh. OpenShift will not let you write these directories
# by default so we must setup some permissions here, and at runtime add our random UID
# to /etc/passwd. (in installmanager.go)
ENV HOME /home/hive
RUN mkdir -p /home/hive && \
    chgrp -R 0 /home/hive && \
    chmod -R g=u /home/hive
RUN chmod g=u /etc/passwd


# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]
