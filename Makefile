all: build

REPO = jimmidyson/keycloak-operator
TAG = latest

build:
	./scripts/check_license.sh
	go build -o keycloak-operator github.com/jimmidyson/keycloak-operator/cmd/operator

container:
	GOOS=linux $(MAKE) build
	docker build -t $(REPO):$(TAG) .

e2e:
	go test -v ./test/e2e/ --kubeconfig "$(HOME)/.kube/config" --operator-image=jimmidyson/keycloak-operator

clean-e2e:
	kubectl delete namespace keycloak-operator-e2e-tests

.PHONY: all build container e2e clean-e2e
