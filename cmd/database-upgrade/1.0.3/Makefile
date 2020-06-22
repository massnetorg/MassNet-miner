# consts
SCRIPT_BUILD="./build.sh"

build:
	@echo "make build: begin"
	@echo "building mass-db-upgrade to ./bin for current platform..."
	@echo "(To build for all platforms, run 'make buildall')"
	@$(SCRIPT_BUILD) "1.0.3" "CURRENT"
	@echo "make build: end"

buildall:
	@echo "make buildall: begin"
	@echo "building mass-db-upgrade to ./bin for all platforms..."
	@echo "(To build only for current platform, run 'make build')"
	@$(SCRIPT_BUILD) "1.0.3"
	@echo "make buildall: end"

clean:
	@echo "make clean: begin"
	@echo "cleaning .bin/ path..."
	@rm -rf ./bin/logs ./bin/mass-db-upgrade*
	@echo "make clean: end"