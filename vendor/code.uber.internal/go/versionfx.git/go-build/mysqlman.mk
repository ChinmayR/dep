# mysqlman.mk sets up a docker mysqlman instance that is useful for mysql integration testing.
# Setting MYSQLMAN_DB_NAME with the required db name will spin up a mysql instance hosting that db
# and store the host/port config in MYSQLMAN_TEST_CONFIG json file.

# for more info:
# https://engdocs.uberinternal.com/mysqlman/integration_tests.html#go-integration-tests

DOCKERMYSQLMAN = $(MYSQLMAN_ENV)/bin/dockermysqlman
MYSQLMAN_ENV = $(CURDIR)/$(BUILD_DIR)/mysqlman_env
MYSQLMAN_MAKEFILE = $(dir $(lastword $(MAKEFILE_LIST)))/mysqlman.mk

# User-overridable variables:
MYSQLMAN_APP_ID ?= dockermysqlman
MYSQLMAN_DB_USER ?= uber
MYSQLMAN_DB_PASSWORD ?= uber
MYSQLMAN_INSTALL ?= mysqlman
export MYSQLMAN_TEST_CONFIG ?= $(CURDIR)/$(BUILD_DIR)/mysql_config.json

TEST_DEPS += $(MYSQLMAN_TEST_CONFIG)

# create mysqlman virtualenv
$(MYSQLMAN_ENV):
	virtualenv --setuptools $@
	$@/bin/pip install -U setuptools==27.3.0
	$@/bin/pip install -U pip==8.1.2

$(DOCKERMYSQLMAN): $(MYSQLMAN_MAKEFILE) | $(MYSQLMAN_ENV)
	$|/bin/pip install -U $(MYSQLMAN_INSTALL)

# setup mysql test database
$(MYSQLMAN_TEST_CONFIG): | $(DOCKERMYSQLMAN)
	@echo "$(COLOR_BOLD)Starting managed mysql instance$(COLOR_RESET)"
	$| --app-id=$(MYSQLMAN_APP_ID) --database=$(MYSQLMAN_DB_NAME) \
	--user=$(MYSQLMAN_DB_USER) --password=$(MYSQLMAN_DB_PASSWORD) \
	--json-file=$(MYSQLMAN_TEST_CONFIG)

# cleanup mysql
.PHONY: cleanup-mysqlman
cleanup-mysqlman: | $(DOCKERMYSQLMAN)
	@echo "$(COLOR_BOLD)Stopping managed mysql instance$(COLOR_RESET)"
	$| --app-id=$(MYSQLMAN_APP_ID) --stop
	rm -f $(MYSQLMAN_TEST_CONFIG)
