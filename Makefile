lint:
	golangci-lint run --fix

tidy:
	go mod tidy

test: tidy lint
	go test ./...  -coverprofile=coverage.out
	go tool cover -func=coverage.out

vhs-pod:
	vhs docs/pod.tape

vhs-node:
	vhs docs/node.tape
