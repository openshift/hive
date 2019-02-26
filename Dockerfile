FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 as builder
RUN mkdir -p /go/src/github.com/openshift/hive
WORKDIR /go/src/github.com/openshift/hive
COPY . .
RUN GOPATH=/go go get github.com/golang/mock/gomock; go install github.com/golang/mock/mockgen
RUN PATH=$PATH:/go/bin make generate
RUN go build -o bin/manager github.com/openshift/hive/cmd/manager
RUN go build -o bin/hiveutil github.com/openshift/hive/contrib/cmd/hiveutil
RUN go build -o bin/hiveadmission github.com/openshift/hive/cmd/hiveadmission

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
COPY --from=builder /go/src/github.com/openshift/hive/bin/manager /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveadmission /opt/services/
COPY --from=builder /go/src/github.com/openshift/hive/bin/hiveutil /usr/bin
ENTRYPOINT ["/opt/services/manager"]
