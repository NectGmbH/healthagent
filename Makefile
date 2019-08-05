VERSION=0.1.1
SERVER=server
CLIENT=client
CERTS_DIR=./certs/
ORG=NectGmbH
IMAGE=kavatech/healthagent:$(VERSION)

build:
	go build -ldflags "-X main.APPVERSION=$(VERSION)"

docker-build:
	GOOS=linux go build -ldflags "-X main.APPVERSION=$(VERSION)" 
	docker build -t $(IMAGE) .

docker-push: docker-build
	docker push $(IMAGE)

certs-create-ca:
	openssl genrsa -out $(CERTS_DIR)ca.key 4096
	openssl req -x509 -new -nodes -key $(CERTS_DIR)ca.key -subj "/C=DE/ST=HH/O=$(ORG), Inc./CN=ca.nect.com" -sha256 -days 1024 -out $(CERTS_DIR)ca.crt

certs-create-server:
	openssl genrsa -out $(CERTS_DIR)$(SERVER).key 2048
	openssl req -new -sha256 -key $(CERTS_DIR)$(SERVER).key -config server-cert.conf -out $(CERTS_DIR)$(SERVER).csr
	openssl x509 -req -in $(CERTS_DIR)$(SERVER).csr -CA $(CERTS_DIR)ca.crt -CAkey $(CERTS_DIR)ca.key -CAcreateserial -extfile server-cert.conf -extensions v3_req -out $(CERTS_DIR)$(SERVER).crt -days 500 -sha256

certs-create-client:
	openssl ecparam -genkey -name secp256r1 | openssl ec -out $(CERTS_DIR)$(CLIENT).key
	openssl req -new -sha256 -key $(CERTS_DIR)$(CLIENT).key -subj "/C=DE/ST=HH/O=$(ORG), Inc./CN=$(CLIENT)" -out $(CERTS_DIR)$(CLIENT).csr
	openssl x509 -req -in $(CERTS_DIR)$(CLIENT).csr -CA $(CERTS_DIR)ca.crt -CAkey $(CERTS_DIR)ca.key -CAcreateserial -out $(CERTS_DIR)$(CLIENT).crt -days 500 -sha256

certs-create-client-san:
	openssl ecparam -genkey -name secp256r1 | openssl ec -out $(CERTS_DIR)$(CLIENT)-san.key
	openssl req -new -sha256 -key $(CERTS_DIR)$(CLIENT)-san.key -config client-cert.conf  -out $(CERTS_DIR)$(CLIENT)-san.csr
	openssl x509 -req -in $(CERTS_DIR)$(CLIENT)-san.csr -CA $(CERTS_DIR)ca.crt -CAkey $(CERTS_DIR)ca.key -CAcreateserial -extfile client-cert.conf -extensions v3_req -out $(CERTS_DIR)$(CLIENT)-san.crt -days 500 -sha256