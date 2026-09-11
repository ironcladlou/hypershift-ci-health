FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y git-core && microdnf clean all
COPY bin/ci-health /usr/local/bin/ci-health
ENTRYPOINT ["ci-health"]
