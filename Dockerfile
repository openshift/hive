FROM openshift/origin-release:golang-1.12 as builder
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .
RUN go build -o bin/manager github.com/openshift/hive/cmd/manager
RUN go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil
RUN go build -o bin/hiveadmission github.com/openshift/hive/cmd/hiveadmission
RUN go build -o bin/hive-operator github.com/openshift/hive/cmd/operator

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base as hive
COPY --from=builder /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin
COPY --from=builder /go/src/github.com/openshift/hive/bin/hive-operator /opt/services

# In the event of failures we run ssh commands to execute the script which gathers logs
# from all systems in the cluster, and scp's those from the bootstrap node into the
# /logs volume in the install pod. This requires the current user ID (randomly assigned by
# the cluster) to be in the passwd file. At runtime we execute with gid 0, so making the
# file group writable allows our install process to add our install user ID to the passwd
# file and unblock ssh.
RUN chmod g+rw /etc/passwd

# TODO: should this be the operator?
ENTRYPOINT ["/opt/services/manager"]
