FROM alpine:3.10

COPY ./healthagent /usr/local/bin/healthagent

RUN addgroup -g 1337 -S healthagent && adduser -u 1337 -S healthagent -G healthagent

USER healthagent

# Usage: docker run -v ./certs:/certs --restart=always kavatech/healthagent -name foo -upstream https://healthd.1.org -upstream https://healthd.2.org -upstream https://healthd.3.org
ENTRYPOINT [ "/usr/local/bin/healthagent", "-logtostderr", "-ca", "/certs/ca.crt", "-crt", "/certs/client.crt", "-key", "/certs/client.key" ]