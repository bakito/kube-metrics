lint:
	golangci-lint run --fix

tidy:
	go mod tidy

test: tidy lint
	go test ./...  -coverprofile=coverage.out
	go tool cover -func=coverage.out

vhs:
	vhs docs/pod.tape
