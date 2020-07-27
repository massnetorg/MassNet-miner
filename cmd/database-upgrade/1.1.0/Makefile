build:
	@echo "make build: begin"
	@echo "building mass-db-upgrade to ./bin for current platform..."
	@env GO111MODULE=on go build -o ./bin/mass-db-upgrade-1.1.0
	@echo "make build: end"

clean:
	@echo "make clean: begin"
	@echo "cleaning .bin/ path..."
	@rm -rf ./bin/logs ./bin/mass-db-upgrade*
	@echo "make clean: end"