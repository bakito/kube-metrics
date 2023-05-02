# Run go fmt against code
fmt:
	go fmt ./...
	gofmt -s -w .

# Run go vet against code
vet:
	go vet ./...

# Run go mod tidy
tidy:
	go mod tidy

# Run tests
test:  tidy fmt vet
	go test ./...  -coverprofile=coverage.out
	go tool cover -func=coverage.out

build:
	env GOOS=linux go build -o pod-metrics main.go
	env GOOS=windows go build -o pod-metrics.exe main.go
