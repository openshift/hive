FROM openshift/origin-release:golang-1.10 as build-env

COPY . /go/src/github.com/openshift/cluster-network-operator
WORKDIR /go/src/github.com/openshift/cluster-network-operator
RUN ./hack/build-go.sh

FROM openshift/origin-base

COPY --from=build-env /go/src/github.com/openshift/cluster-network-operator/_output/linux/amd64/cluster-network-operator /bin/cluster-network-operator
COPY --from=build-env /go/src/github.com/openshift/cluster-network-operator/_output/linux/amd64/cluster-network-renderer /bin/cluster-network-renderer
COPY manifests /manifests
COPY bindata /bindata

ENV OPERATOR_NAME=cluster-network-operator
CMD ["/bin/cluster-network-operator"]
LABEL io.openshift.release.operator true
