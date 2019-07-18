# healthagent

healthagent is a tool which continously monitors an endpoint and informs another endpoint regarding status changes.

Usage:
```
./healthagent -logtostderr                          \
              -monitor http://192.168.5.1:1234      \
              -monitor http://192.168.5.2:1234      \
              -monitor http://192.168.5.3:1234      \
              -upstream https://monitoring.service.az1.com \
              -upstream https://monitoring.service.az2.com \
              -upstream https://monitoring.service.az3.com \
              -ca ./certs/ca.crt                    \
              -crt ./certs/client.crt               \
              -key ./certs/client.key
```

## License

Licensed under [MIT](./LICENSE).