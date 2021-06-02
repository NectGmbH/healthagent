package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"
)

// APPVERSION contains the version of the tool, injected by make
var APPVERSION string

func main() {
	logrus.Infof("healthagent v%s", APPVERSION)

	config := Configuration{}
	var caPath string
	var crtPath string
	var keyPath string

	hostname, _ := os.Hostname()

	flag.IntVar(&config.KeepAlive, "keep-alive", 30, "amount of seconds where no requests are made to the upstream till a 'forcefull' keep alive request happens")
	flag.IntVar(&config.Interval, "interval", 1, "interval of health monitors")
	flag.Var(&config.Upstreams, "upstream", "upstream endpoint which should be notified on health changes, e.g. https://schnitzel.de/. Multiple can be given (it'll try all till the first works), e.g.: -upstream https://192.168.1.1 -upstream https://192.168.1.2")
	flag.StringVar(&config.Name, "name", hostname, "name to use to identify the current agent, defaults to the hostname")
	flag.StringVar(&caPath, "ca", "", "path to the ca.crt")
	flag.StringVar(&crtPath, "crt", "", "path to the client.crt")
	flag.StringVar(&keyPath, "key", "", "path to the client.key")
	flag.BoolVar(&config.JsonLogging, "json-logging", false, "Always use JSON logging")
	flag.Parse()

	if config.JsonLogging == true {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
	logrus.Infof("healthagent v%s", APPVERSION)

	if config.KeepAlive <= 0 {
		logrus.Fatalf("keep-alive is set to `%d` expected it to be at least 1", config.KeepAlive)
	}

	if len(config.Upstreams) == 0 {
		logrus.Fatal("no upstream configured, pass it using -upstream http://schnitzel.de")
	}

	if config.Name == "" {
		logrus.Fatal("no name given, pass it using -name or keep it unspecified for using the hostname")
	}

	if caPath == "" {
		logrus.Fatal("no ca certificate given, pass it using -ca")
	}

	if crtPath == "" {
		logrus.Fatal("no client certificate given, pass it using -crt")
	}

	if keyPath == "" {
		logrus.Fatal("no client key given, pass it using -key")
	}

	var err error
	config.CA, err = ioutil.ReadFile(caPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"path":   caPath,
			"detail": err.Error(),
		}).Fatalf("couldn't read ca certificate")
	}

	config.Cert, err = ioutil.ReadFile(crtPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"path":   crtPath,
			"detail": err.Error(),
		}).Fatalf("couldn't read client certificate")
	}

	config.Key, err = ioutil.ReadFile(keyPath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"path":   keyPath,
			"detail": err.Error(),
		}).Fatalf("couldn't read client key")
	}

	agent, err := NewAgent(config)
	if err != nil {
		logrus.Fatalf("couldn't create agent `%s`, see: %v", config.Name, err)
	}

	agent.Start()
	logrus.Info("healthagent started")

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	for range signalCh {
		logrus.Infof("Received ^C, shutting down...")
		agent.Stop()
		break
	}

	logrus.Info("Stopped.")
}
