# healthagent

healthagent is a tool which continously monitors an endpoint and informs another endpoint regarding status changes.

## Usage

### Bare metal

```
./healthagent -logtostderr                                 \
              -upstream https://monitoring.service.az1.com \
              -upstream https://monitoring.service.az2.com \
              -upstream https://monitoring.service.az3.com \
              -ca ./certs/ca.crt                           \
              -crt ./certs/client.crt                      \
              -key ./certs/client.key
```

### Docker

```
docker run -v ./certs:/certs --restart=always kavatech/healthagent -name foo -upstream https://healthd.1.org -upstream https://healthd.2.org -upstream https://healthd.3.org
```

## License

Licensed under [MIT](./LICENSE).