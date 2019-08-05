FROM alpine:3.10

COPY ./healthagent /usr/local/bin/healthagent

RUN addgroup -g 1337 -S healthagent && adduser -u 1337 -S healthagent -G healthagent

USER healthagent

ENTRYPOINT [ "/usr/local/bin/healthagent", "-logtostderr", "-ca", "/certs/ca.crt", "-crt", "/certs/client.crt", "-key", "/certs/client.key" ]