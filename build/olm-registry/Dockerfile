FROM quay.io/openshift/origin-operator-registry:latest

COPY bundle manifests/hive
RUN initializer

CMD ["registry-server", "-t", "/tmp/terminate.log"]
