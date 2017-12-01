PROJECT_ROOT = code.uber.internal/engsec/wonka-go.git

LINT_EXCLUDES = mock_.*_test

SERVICES = wonkamaster/wonkamaster

THRIFT_SRCS = wonkamaster/thrift/pullo.thrift

# list all executables
PROGS = wonkacli/wonkacli	\
	wonkad/wonkad \
	wonkamaster/wonkamaster

WONKA_FILES: $(wildcard *.go)

wonkacli/wonkacli: $(wildcard *.go) $(wildcard wonkacli/*.go) $(wildcard wonkacli/cmd/*.go)
wonkad/wonkad: $(wildcard *.go) $(wildcard wonkad/*.go)
wonkamaster/wonkamaster: $(wildcard wonkamaster/*.go) \
	$(wildcard wonkamaster/keys/*.go) \
	$(wildcard wonkamaster/handlers/*.go) \
	$(wildcard wonkamaster/wonkassh/*.go) \
	$(wildcard wonkamaster/claims/*.go) \
	$(wildcard wonkamaster/wonkadb/*.go)

BUILD_LDFLAGS := -X $(PROJECT_ROOT).BuildVersion=$(shell git describe --long --always --dirty 2>/dev/null)

-include go-build/rules.mk

go-build/rules.mk:
	git submodule update --init

.PHONY: enroll

enroll: $(PROGS)
	(cd wonkacli && ./enroll_service)

# See README.md for release process and instructions.
release: jenkins-with-db
	./scripts/release

run: $(PROGS)
	trap 'docker-compose stop && exit' SIGINT && \
	cd wonkamaster && \
		docker-compose up -d && sleep 1 && \
		UBER_ENVIRONMENT=test UBER_CONFIG_DIR=./config ./wonkamaster

HOMEBREW_TARGETS = wonkacli/wonkacli
BOTTLE_NAME = wonkacli

# We build a bunch o' bottles here
-include go-build/bottles.mk

# cassandra config
IS_JENKINS = $(if $(BUILD_TAG),1,)
# give the container a unique name if we're on jenkins to ensure isolation
DOCKER_CASSANDRA_NAME = cassandra-$(if $(IS_JENKINS),$(BUILD_TAG),local-wonka)
CSTAR_DEFAULT_PORT = 9042
# use 9042 for local development, random port otherwise
PORT_FLAG = $(if $(IS_JENKINS),-P,-p $(CSTAR_DEFAULT_PORT):$(CSTAR_DEFAULT_PORT))
DOCKER_CASSANDRA_TAG = 2.2.9
DOCKER_CASSANDRA_HOST = 127.0.0.1
CQL_VERSION = 3.3.1

.PHONY: jenkins-with-db
jenkins-with-db: __eval_docker_cassandra_port
	# $(MAKE) db-install
	DOCKER_CASSANDRA_PORT="$(DOCKER_CASSANDRA_PORT)" $(MAKE) jenkins
	# $(MAKE) db-stop
	# $(MAKE) db-tear-down

# discover the randomly-chosen docker port if we're on jenkins, otherwise use the default
.PHONY: __eval_docker_cassandra_port
__eval_docker_cassandra_port:
	$(eval DOCKER_CASSANDRA_PORT := $(if $(IS_JENKINS),$(shell docker inspect --format '{{index .NetworkSettings.Ports "9042/tcp" 0 "HostPort"}}' $(DOCKER_CASSANDRA_NAME) 2>/dev/null),$(CSTAR_DEFAULT_PORT)))

.PHONY: wait-on-db
wait-on-db: __eval_docker_cassandra_port
	./wonkamaster/scripts/wait-with-cqlsh.sh --cqlversion $(CQL_VERSION) --host $(DOCKER_CASSANDRA_HOST) --port $(DOCKER_CASSANDRA_PORT) --name Cassandra --interval 1 --tries 100

.PHONY: __eval_cql_cmd
__eval_cql_cmd: __eval_docker_cassandra_port
	$(eval CQL_CMD := cqlsh --cqlversion=$(CQL_VERSION) $(DOCKER_CASSANDRA_HOST) $(DOCKER_CASSANDRA_PORT))

.PHONY: db-install
db-install:
	docker run --name $(DOCKER_CASSANDRA_NAME) $(PORT_FLAG) -d cassandra:$(DOCKER_CASSANDRA_TAG)
	docker start ${DOCKER_CASSANDRA_NAME}
	$(MAKE) wait-on-db
	$(MAKE) db-table-setup

.PHONY: db-tear-down
db-tear-down:
	docker rm -f ${DOCKER_CASSANDRA_NAME}

.PHONY: db-start
db-start:
	docker start ${DOCKER_CASSANDRA_NAME}

.PHONY: db-stop
db-stop:
	docker stop ${DOCKER_CASSANDRA_NAME}

.PHONY: db-table-setup
db-table-setup: __eval_cql_cmd
	$(CQL_CMD) -f wonkamaster/schema/development.cql
