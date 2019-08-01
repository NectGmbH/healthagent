# healthagent

healthagent is a tool which continously monitors an endpoint and informs another endpoint regarding status changes.

Usage:
```
./healthagent -logtostderr                                 \
              -upstream https://monitoring.service.az1.com \
              -upstream https://monitoring.service.az2.com \
              -upstream https://monitoring.service.az3.com \
              -ca ./certs/ca.crt                           \
              -crt ./certs/client.crt                      \
              -key ./certs/client.key
```

## License

Licensed under [MIT](./LICENSE).