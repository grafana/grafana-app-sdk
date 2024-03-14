.DEFAULT_GOAL := run

.PHONY=generate
generate:
	@grafana-app-sdk generate -c . -g ./apis
	rm -rf definitions
	rm -rf plugin

.PHONY=run
run: generate
	@go run main.go

.PHONY=clean
clean:
	rm -rf .data