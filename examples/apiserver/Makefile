.PHONY: etcd
etcd:
	@docker run -d --name etcd --env ALLOW_NONE_AUTHENTICATION=yes -p 2379:2379 bitnamilegacy/etcd:latest

.PHONY: generate
generate:
	@go run ../../cmd/grafana-app-sdk/*.go generate -g ./apis --grouping=group --defencoding=none

.PHONY: run
run:
	@go run . --etcd-servers=http://127.0.0.1:2379 --secure-port 6443
