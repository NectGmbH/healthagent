package main

import (
    "bytes"
    "crypto/tls"
    "crypto/x509"
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "net/http"
    "strings"
    "time"

    "github.com/NectGmbH/health"
    "github.com/OneOfOne/xxhash"
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
    syncStopCh      chan struct{}
    lastRequest     time.Time
    keepAliveStopCh chan struct{}
    stopChans       []chan struct{}
    httpClient      *http.Client
    lastMonitorHash uint64
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
        status:          make([]health.HealthCheckStatus, 0),
        checks:          make([]*health.HealthCheck, 0),
        stopChans:       make([]chan struct{}, 0),
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

    err = agent.syncMonitors(true)
    if err != nil {
        return nil, fmt.Errorf("couldn't do initial sync of monitors, see: %v", err)
    }

    go agent.loopSyncMonitors()

    return agent, nil
}

func (a *Agent) loopSyncMonitors() {
    a.syncStopCh = make(chan struct{}, 0)

    for {
        select {
        case <-a.syncStopCh:
            logrus.Info("stopped sync monitors loop")
            return

        default:
            err := a.syncMonitors(false)
            if err != nil {
                logrus.Panicf("couldn't sync monitors, aborting execution since state may be fucked, see: %v", err)
            }

            time.Sleep(30 * time.Second)
        }
    }
}

func (a *Agent) syncMonitors(init bool) error {
    monitors, hash, err := a.retrieveMonitors()
    if err != nil {
        return fmt.Errorf("couldn't retrieve monitors from upstream, see: %v", err)
    }

    logrus.Debugf("received monitors %#v with hash %d", monitors, hash)

    if hash != 0 && hash == a.lastMonitorHash {
        logrus.Info("SYNC MONITORS - skipping, since upstream has same hash")
        return nil
    }

    logrus.Info("start syncing monitors")

    if !init {
        logrus.Info("SYNC MONITORS - stopping health checking")
        a.Stop()
        logrus.Info("SYNC MONITORS - stopped health checking")
    }

    a.checks = make([]*health.HealthCheck, len(monitors))
    a.stopChans = make([]chan struct{}, len(monitors))
    a.status = make([]health.HealthCheckStatus, len(monitors))

    logrus.Info("SYNC MONITORS - setting up monitors")
    err = a.setupMonitors(monitors)
    if err != nil {
        return fmt.Errorf("couldn't setup monitors, see: %v", err)
    }
    logrus.Info("SYNC MONITORS - set up monitors")

    if !init {
        logrus.Info("SYNC MONITORS - restarting health checking")
        a.Start()
        logrus.Info("SYNC MONITORS - restarted health checking")
    }

    a.lastMonitorHash = hash

    logrus.Info("finished syncing monitors")

    return nil
}

func (a *Agent) setupMonitors(monitors StringSlice) error {
    for i, monitor := range monitors {
        prot, endpoint, err := TryParseProtocolEndpoint(monitor)
        if err != nil {
            return fmt.Errorf("couldn't parse monitor `%s`, see: %v", monitor, err)
        }

        provider, err := health.GetHealthCheckProvider(prot)
        if err != nil {
            return fmt.Errorf("couldn't get health check provider for monitor `%s`, see: %v", monitor, err)
        }

        h := health.NewHealthCheck(
            endpoint.IP,
            int(endpoint.Port),
            provider,
            time.Duration(a.config.Interval)*time.Second,
            60*time.Second,
            1*time.Second)

        logrus.Infof("setup health check for %v:%d", endpoint.IP, endpoint.Port)

        a.checks[i] = h
        a.stopChans[i] = make(chan struct{}, 0)
    }

    return nil
}

func (a *Agent) logUpstreamFail(msg string, up string, details error, try int) {
    logrus.WithFields(logrus.Fields{
        "upstream": up,
        "detail":   details,
        "cur":      try,
        "max":      len(a.config.Upstreams),
    }).Warn(msg)
}

func (a *Agent) retrieveMonitors() (StringSlice, uint64, error) {
    var err error

    for try, upstream := range a.config.Upstreams {
        var req *http.Request
        req, err = http.NewRequest("GET", upstream, nil)
        if err != nil {
            err = fmt.Errorf("couldn't create GET request to upstream `%s`, see: %v", upstream, err)
            a.logUpstreamFail("couldn't receive monitors from upstream", upstream, err, try)
            continue
        }

        req.Header.Add("X-Agent-Name", a.config.Name)

        var resp *http.Response
        resp, err = a.httpClient.Do(req)
        if err != nil {
            err = fmt.Errorf("couldn't GET to upstream `%s`, see: %v", upstream, err)
            a.logUpstreamFail("couldn't receive monitors from upstream", upstream, err, try)
            continue
        }

        defer resp.Body.Close()
        bodyBytes, _ := ioutil.ReadAll(resp.Body)
        bodyString := string(bodyBytes)

        if resp.StatusCode < 200 || resp.StatusCode > 299 {
            err = fmt.Errorf("invalid status `%d` for GET to upstream `%s`, see: %s", resp.StatusCode, upstream, bodyString)
            a.logUpstreamFail("couldn't receive monitors from upstream", upstream, err, try)
            continue
        }

        var monitors []string
        err = json.Unmarshal(bodyBytes, &monitors)
        if err != nil {
            err = fmt.Errorf("couldn't deserialize monitors from upstream `%s`, see: %s", upstream, err)
            a.logUpstreamFail("couldn't receive monitors from upstream", upstream, err, try)
            continue
        }

        h := xxhash.New64()
        reader := strings.NewReader(bodyString)
        io.Copy(h, reader)
        hash := h.Sum64()

        return monitors, hash, nil
    }

    return nil, 0, err
}

func (a *Agent) informUpstream(index int) error {
    var err error

    for try, upstream := range a.config.Upstreams {
        a.lastRequest = time.Now()

        buf, err := json.Marshal(a.status)

        if err != nil {
            err = fmt.Errorf("couldn't serialize current status, see: %v", err)
            a.logUpstreamFail("couldn't inform upstream about new status", upstream, err, try)
            continue
        }

        req, err := http.NewRequest("POST", upstream, bytes.NewReader(buf))
        if err != nil {
            err = fmt.Errorf("couldn't create POST request to upstream `%s`, see: %v", upstream, err)
            a.logUpstreamFail("couldn't inform upstream about new status", upstream, err, try)
            continue
        }

        req.Header.Add("X-Agent-Name", a.config.Name)

        resp, err := a.httpClient.Do(req)
        if err != nil {
            err = fmt.Errorf("couldn't POST to upstream `%s`, see: %v", upstream, err)
            a.logUpstreamFail("couldn't inform upstream about new status", upstream, err, try)
            continue
        }

        defer resp.Body.Close()

        if resp.StatusCode < 200 || resp.StatusCode > 299 {
            bodyBytes, _ := ioutil.ReadAll(resp.Body)
            bodyString := string(bodyBytes)

            err = fmt.Errorf("invalid status `%d` for post to upstream `%s`, see: %s", resp.StatusCode, upstream, bodyString)
            a.logUpstreamFail("couldn't inform upstream about new status", upstream, err, try)
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
