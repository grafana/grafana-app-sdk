.PHONY: etcd
etcd:
	@docker run -d --name etcd --env ALLOW_NONE_AUTHENTICATION=yes bitnami/etcd:latest

.kubeconfig:
	@touch .kubeconfig
	@kind create cluster --kubeconfig .kubeconfig

.PHONY: run
run: .kubeconfig
	@go run . --etcd-servers 127.0.0.1:2739 --kubeconfig .kubeconfig --secure-port 6443 --authentication-kubeconfig .kubeconfig --authorization-kubeconfig .kubeconfig