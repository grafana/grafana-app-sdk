.PHONY: etcd
etcd:
	@docker run -d --name etcd --env ALLOW_NONE_AUTHENTICATION=yes bitnami/etcd:latest

.PHONY: run
run:
	@go run . --etcd-servers 127.0.0.1:2739 --secure-port 6443