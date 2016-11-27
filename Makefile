build: check keycloak-operator

REPO = jimmidyson/keycloak-operator
TAG = latest

keycloak-operator: $(shell find -name *.go)
	go build -o keycloak-operator github.com/jimmidyson/keycloak-operator/cmd/operator

keycloak-operator-linux-static: $(shell find -name *.go)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -o keycloak-operator-linux-static \
		-ldflags "-s" -a -installsuffix cgo \
		github.com/jimmidyson/keycloak-operator/cmd/operator

check: .check_license

.check_license: $(shell find ! -path '*/vendor/*' -name *.go)
	./scripts/check_license.sh
	touch .check_license

image: check keycloak-operator-linux-static
	docker build -t $(REPO):$(TAG) .

e2e:
	go test -v ./test/e2e/ --kubeconfig "$(HOME)/.kube/config" --operator-image=jimmidyson/keycloak-operator

clean-e2e:
	kubectl delete namespace keycloak-operator-e2e-tests

.PHONY: build check container e2e clean-e2e
