LESSMAN_APP_ID ?= lessman
export LESSMAN_TEST_CONFIG ?= $(CURDIR)/$(BUILD_DIR)/host_config.json
LESSMAN_ARGS ?=

TEST_DEPS += $(LESSMAN_TEST_CONFIG)

# start lessman for your schemaless instance and write the output to a json file.
$(LESSMAN_TEST_CONFIG):
	lessman start $(SCHEMALESS_INSTANCE) -fv --app-id=$(LESSMAN_APP_ID) --json-file=$(LESSMAN_TEST_CONFIG) $(LESSMAN_ARGS)

# cleanup lessman
.PHONY: cleanup-lessman
cleanup-lessman: ## Stops all lessman containers for LESSMAN_APP_ID and forces fresh containers to be created on next test run
	rm -f $(LESSMAN_TEST_CONFIG)
	-lessman stop-all --app-id=$(LESSMAN_APP_ID)
