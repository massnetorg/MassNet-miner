# Makefile for MASS Miner

# consts
TEST_REPORT=test.report

# make commands
build:
	@echo "make build: begin"
	@echo "building massminer to ./bin for current platform..."
	@env GO111MODULE=on go build -o bin/massminer
	@echo "building massminercli to ./bin for current platform..."
	@env GO111MODULE=on go build -o bin/massminercli cmd/massminercli/main.go
	@echo "make build: end"

test:
	@echo "make test: begin"
	@env GO111MODULE=on go test ./... 2>&1 | tee $(TEST_REPORT)
	@echo "make test: end"
