package main

import (
    "bytes"
    "crypto/tls"
    "crypto/x509"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "strings"
    "time"

    "github.com/NectGmbH/health"
    "github.com/sirupsen/logrus"
)

// StringSlice is a typed slice of strings
type StringSlice []string

// String returns a string representation of the current string slice.
func (i *StringSlice) String() string {
    return strings.Join(*i, " ")
}

// Set appends a entry to the string slice (used for flags)
func (i *StringSlice) Set(value string) error {
    *i = append(*i, value)
    return nil
}

// Configuration represents the user specified configuration of the agent
type Configuration struct {
    Monitors  StringSlice
    Upstreams StringSlice
    Interval  int
    KeepAlive int
    Name      string
    CA        []byte
    Cert      []byte
    Key       []byte
}

// Agent represents an health monitoring agent
type Agent struct {
    config          Configuration
    status          []health.HealthCheckStatus // Same index as monitor in config
    checks          []*health.HealthCheck
    lastRequest     time.Time
    keepAliveStopCh chan struct{}
    stopChans       []chan struct{}
    httpClient      *http.Client
}

// NewAgent creates a new agent with the passed configuration.
func NewAgent(config Configuration) (*Agent, error) {
    pool := x509.NewCertPool()
    pool.AppendCertsFromPEM(config.CA)

    clientCert, err := tls.X509KeyPair(config.Cert, config.Key)
    if err != nil {
        return nil, fmt.Errorf("couldn't parse cert pair, see: %v", err)
    }

    agent := &Agent{
        config:          config,
        status:          make([]health.HealthCheckStatus, len(config.Monitors)),
        checks:          make([]*health.HealthCheck, len(config.Monitors)),
        stopChans:       make([]chan struct{}, len(config.Monitors)),
        keepAliveStopCh: make(chan struct{}, 0),
        httpClient: &http.Client{
            Timeout: time.Second * 3,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    RootCAs:      pool,
                    Certificates: []tls.Certificate{clientCert},
                },
            },
        },
        lastRequest: time.Now(),
    }

    for i, monitor := range config.Monitors {
        prot, endpoint, err := TryParseProtocolEndpoint(monitor)
        if err != nil {
            return nil, fmt.Errorf("couldn't parse monitor `%s`, see: %v", monitor, err)
        }

        provider, err := health.GetHealthCheckProvider(prot)
        if err != nil {
            return nil, fmt.Errorf("couldn't get health check provider for monitor `%s`, see: %v", monitor, err)
        }

        h := health.NewHealthCheck(
            endpoint.IP,
            int(endpoint.Port),
            provider,
            time.Duration(config.Interval)*time.Second,
            60*time.Second,
            1*time.Second)

        agent.checks[i] = h
        agent.stopChans[i] = make(chan struct{}, 0)
    }

    return agent, nil
}

func (a *Agent) informUpstream(index int) error {
    var err error

    logUpstreamFail := func(up string, details error, try int) {
        logrus.WithFields(logrus.Fields{
            "upstream": up,
            "detail":   details,
            "cur":      try,
            "max":      len(a.config.Upstreams),
        }).Warn("couldn't inform upstream")
    }

    for try, upstream := range a.config.Upstreams {
        a.lastRequest = time.Now()

        buf, err := json.Marshal(a.status)

        if err != nil {
            err = fmt.Errorf("couldn't serialize current status, see: %v", err)
            logUpstreamFail(upstream, err, try)
            continue
        }

        req, err := http.NewRequest("POST", upstream, bytes.NewReader(buf))
        if err != nil {
            err = fmt.Errorf("couldn't create request to upstream `%s`, see: %v", upstream, err)
            logUpstreamFail(upstream, err, try)
            continue
        }

        req.Header.Add("X-Agent-Name", a.config.Name)

        resp, err := a.httpClient.Do(req)
        if err != nil {
            err = fmt.Errorf("couldn't POST to upstream `%s`, see: %v", upstream, err)
            logUpstreamFail(upstream, err, try)
            continue
        }

        defer resp.Body.Close()

        if resp.StatusCode < 200 || resp.StatusCode > 299 {
            bodyBytes, _ := ioutil.ReadAll(resp.Body)
            bodyString := string(bodyBytes)

            err = fmt.Errorf("invalid status `%d` for post to upstream `%s`, see: %s", resp.StatusCode, upstream, bodyString)
            logUpstreamFail(upstream, err, try)
            continue
        }

        return nil
    }

    return err
}

func (a *Agent) monitorHealthCheck(index int, check *health.HealthCheck, feed chan health.HealthCheckStatus) {
    for status := range feed {
        a.status[index] = status

        if status.DidChange {
            logrus.Info(status.String())

            err := a.informUpstream(index)
            if err != nil {
                logrus.WithFields(logrus.Fields{
                    "event":   "change",
                    "detail":  err.Error(),
                    "healthy": status.Healthy,
                    "monitor": a.config.Monitors[index],
                }).Error("couldn't inform upstream")
            }
        } else {
            logrus.Debug(status.String())
        }
    }
}

// Start start the monitoring loop in the background (NON BLOCKING!)
func (a *Agent) Start() {
    for i, check := range a.checks {
        func(i int, check *health.HealthCheck) {
            go a.monitorHealthCheck(i, check, check.Monitor(a.stopChans[i]))
        }(i, check)
    }

    go (func() {
        for {
            select {
            case <-a.keepAliveStopCh:
                return

            default:
                timeSinceLastReq := time.Since(a.lastRequest)

                if timeSinceLastReq > time.Duration(a.config.KeepAlive)*time.Second {
                    logrus.WithFields(logrus.Fields{
                        "timeSinceLastRequest": timeSinceLastReq,
                    }).Info("enforcing keep alive")

                    err := a.informUpstream(-1)
                    if err != nil {
                        logrus.WithFields(logrus.Fields{
                            "event":  "keepalive",
                            "detail": err.Error(),
                        }).Error("couldn't inform upstream")
                    }
                }

                time.Sleep(1 * time.Second)
            }
        }
    })()
}

// Stop stops all monitorings
func (a *Agent) Stop() {
    for _, s := range a.stopChans {
        s <- struct{}{}
        close(s)
    }

    a.keepAliveStopCh <- struct{}{}
    close(a.keepAliveStopCh)
}
