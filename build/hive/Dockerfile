FROM centos:7

COPY manager /opt/services/
COPY hiveadmission /opt/services/
COPY hiveutil /usr/bin
COPY hive-operator /opt/services

ENTRYPOINT ["/opt/services/manager"]
